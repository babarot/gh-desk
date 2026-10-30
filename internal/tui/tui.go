// Package tui is gh-triage's terminal UI. It reads keys through the keymap
// by action name, never by key, and learns about GitHub only through the
// Source the app package hands it.
package tui

import (
	"context"
	"log/slog"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/babarot/gh-triage/internal/keymap"
	"github.com/babarot/gh-triage/internal/logging"
	"github.com/babarot/gh-triage/internal/model"
)

// Source is what the TUI needs from GitHub. The app package passes the
// GitHub client, adapted to it.
type Source interface {
	Viewer(ctx context.Context) (string, error)
	// SearchPage returns first pull requests matching query after the
	// cursor after (empty for the first page), how many match in all, the
	// next page's cursor (empty after the last), and warnings for the parts
	// GitHub could not read.
	SearchPage(ctx context.Context, query string, first int, after string) (prs []model.PR, total int, next string, warnings []string, err error)
	Notifications(ctx context.Context, limit int) ([]model.Notification, error)
	PullRequests(ctx context.Context, keys []model.PRKey) (map[model.PRKey]model.PR, error)
	Detail(ctx context.Context, key model.PRKey) (model.Detail, error)
	// Diff is a pull request's diff at its HeadOID.
	Diff(ctx context.Context, pr model.PR) ([]byte, error)
	Comment(ctx context.Context, apiURL string) (model.Comment, error)
	Review(ctx context.Context, pr model.PR, event model.ReviewEvent, body string) error
	MarkDone(ctx context.Context, n model.Notification) error
	Browse(url string) error
}

// Section is a tab of pull requests found by a search.
type Section struct {
	Name    string
	Query   string
	Grouped bool
	Folded  []string
	// DraftsLast sorts drafts after the rest.
	DraftsLast bool
	Limit      int
}

// Settings are the config values the TUI uses, as plain values.
type Settings struct {
	Sections        []Section
	InboxLimit      int
	HideClosed      bool
	DoneAfterReview bool
	HomeOwners      []string
	StaleDays       int
	PreviewHeight   int
	Collapse        []string
	// Theme is "auto", "mocha" or "latte".
	Theme string
	// Editor is the command ctrl+e runs to edit a review, as $EDITOR
	// gives it; empty turns it off.
	Editor string
	// Now is the clock, for tests; nil is time.Now.
	Now func() time.Time
}

type view int

const (
	viewList view = iota
	viewPR
	viewCompose
)

// Model is the TUI's state.
type Model struct {
	src  Source
	keys keymap.Map
	log  *slog.Logger
	set  Settings
	ctx  context.Context
	st   styles

	viewer string
	tabs   []*tab
	cur    int
	unread map[model.PRKey]model.Notification

	width, height int
	view          view
	help          bool
	status        string
	statusErr     bool

	filtering bool
	filter    string

	pane    pane
	cache   cache
	prv     prView
	compose *compose
	confirm *confirm
}

// New returns the TUI reading from src.
func New(src Source, km keymap.Map, log *slog.Logger, set Settings) *Model {
	if set.Now == nil {
		set.Now = time.Now
	}
	if log == nil {
		log = logging.Discard()
	}
	m := &Model{src: src, keys: km, log: log, set: set, ctx: context.Background(), unread: map[model.PRKey]model.Notification{}}
	m.st = newStyles(mocha)
	if set.Theme == "latte" {
		m.st = newStyles(latte)
	}
	for _, s := range set.Sections {
		m.tabs = append(m.tabs, newSectionTab(s))
	}
	m.tabs = append(m.tabs, newInboxTab(set.HideClosed))
	m.pane = pane{on: set.PreviewHeight > 0, mode: paneDiff, unfold: map[model.PRKey]bool{}}
	m.cache = newCache()
	return m
}

// Run shows the TUI until it quits.
func Run(src Source, km keymap.Map, log *slog.Logger, set Settings) error {
	_, err := tea.NewProgram(New(src, km, log, set)).Run()
	return err
}

// Init asks for the terminal's background, the viewer, and every tab.
func (m *Model) Init() tea.Cmd {
	cmds := []tea.Cmd{m.loadViewer()}
	if m.set.Theme == "auto" {
		cmds = append(cmds, tea.RequestBackgroundColor)
	}
	return tea.Batch(cmds...)
}

// Update handles a message.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		if m.compose != nil {
			m.compose.resize(m.width, m.height)
		}
		return m, nil
	case tea.BackgroundColorMsg:
		if m.set.Theme == "auto" && !msg.IsDark() {
			m.st = newStyles(latte)
		}
		return m, nil
	case tea.KeyPressMsg:
		return m, m.key(msg)
	case tea.PasteMsg:
		return m, m.paste(msg)
	}
	return m, m.handleData(msg)
}

// key routes a key press to what has the focus: a popup, the help, the
// filter, the review being written, or the screen.
func (m *Model) key(msg tea.KeyPressMsg) tea.Cmd {
	k := msg.String()
	if k == keymap.Interrupt {
		return tea.Quit
	}
	switch {
	case m.confirm != nil:
		return m.confirmKey(k)
	case m.help:
		if k == keymap.Back || m.keys.Matches("help", k) || m.keys.Matches("quit", k) {
			m.help = false
		}
		return nil
	case m.filtering:
		return m.filterKey(msg)
	case m.view == viewCompose:
		return m.composeKey(msg)
	}
	if m.keys.Matches("help", k) {
		m.help = true
		return nil
	}
	switch m.view {
	case viewPR:
		return m.prKey(k)
	default:
		return m.listKey(k)
	}
}

func (m *Model) paste(msg tea.PasteMsg) tea.Cmd {
	switch {
	case m.filtering:
		m.filter += msg.Content
		m.tab().sel, m.tab().offset = 0, 0
	case m.view == viewCompose && m.compose != nil:
		return m.compose.update(msg)
	}
	return nil
}

// setStatus shows a line under the list until the next one.
func (m *Model) setStatus(s string, isErr bool) {
	m.status, m.statusErr = s, isErr
}

// tab is the tab shown.
func (m *Model) tab() *tab { return m.tabs[m.cur] }
