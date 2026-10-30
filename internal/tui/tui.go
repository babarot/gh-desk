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
	"github.com/babarot/gh-triage/internal/plan"
	"github.com/babarot/gh-triage/internal/runner"
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
	// MergeInfo reads what deciding a merge needs, by node ID; with
	// details, each check (and whether it is required) and the files.
	MergeInfo(ctx context.Context, ids []string, details bool) (map[string]model.MergeInfo, error)
	// Bodies reads descriptions by node ID, for rules that read them.
	Bodies(ctx context.Context, ids []string) (map[string]string, error)
}

// Runner approves and merges batches of pull requests.
type Runner interface {
	Start(ctx context.Context, kind runner.Kind, items []runner.Item) (string, <-chan runner.Event)
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
	// Plan is what deciding a merge takes from the config file.
	Plan plan.Settings
	// Rules are what S offers besides "approved by me".
	Rules []plan.Rule
	// Refresh is how often the shown tab is read again; zero turns it off.
	Refresh time.Duration
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
	run  Runner
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

	pane  pane
	cache cache
	// sending is the review on its way to GitHub; no other is sent until
	// it is back.
	sending *model.PRKey

	selected map[model.PRKey]bool
	bulk     *bulk
	picker   *pick
	// progress is each pull request's step or result in a batch, shown on
	// its row until R.
	progress map[model.PRKey]progress
	batches  map[string]*batchState
	notes    []*note
	notesAt  *notesView
	toast    *note
	merged   map[model.PRKey]bool
	// refreshing is the reading started by R or the clock, and refreshed
	// when each tab was last read.
	refreshed map[int]time.Time
	prv       prView
	compose   *compose
	confirm   *confirm
}

// New returns the TUI reading from src and acting through run.
func New(src Source, run Runner, km keymap.Map, log *slog.Logger, set Settings) *Model {
	if set.Now == nil {
		set.Now = time.Now
	}
	if log == nil {
		log = logging.Discard()
	}
	m := &Model{src: src, run: run, keys: km, log: log, set: set, ctx: context.Background(), unread: map[model.PRKey]model.Notification{},
		selected: map[model.PRKey]bool{}, progress: map[model.PRKey]progress{}, batches: map[string]*batchState{},
		merged: map[model.PRKey]bool{}, refreshed: map[int]time.Time{}}
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

// Run shows the TUI until it quits. Quitting stops every batch.
func Run(ctx context.Context, src Source, run Runner, km keymap.Map, log *slog.Logger, set Settings) error {
	m := New(src, run, km, log, set)
	m.ctx = ctx
	_, err := tea.NewProgram(m, tea.WithContext(ctx)).Run()
	return err
}

// Init asks for the terminal's background, the viewer, and every tab.
func (m *Model) Init() tea.Cmd {
	cmds := []tea.Cmd{m.loadViewer(), m.tick()}
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
	case tickMsg:
		return m, m.autoRefresh()
	case eventMsg:
		return m, m.event(msg)
	case toastMsg:
		if m.toast == msg.n {
			m.toast = nil
		}
		return m, nil
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
	case m.bulk != nil:
		return m.bulkKey(k)
	case m.notesAt != nil:
		return m.notesKey(k)
	case m.picker != nil:
		return m.pickKey(k)
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
