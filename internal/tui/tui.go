// Package tui is gh-desk's terminal UI. It reads keys through the keymap
// by action name, never by key, and learns about GitHub only through the
// Source the app package hands it.
package tui

import (
	"cmp"
	"context"
	"log/slog"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/babarot/gh-desk/internal/logging"
	"github.com/babarot/gh-desk/internal/model"
	"github.com/babarot/gh-desk/internal/plan"
	"github.com/babarot/gh-desk/internal/runner"
	"github.com/babarot/gh-desk/internal/tui/chat"
	"github.com/babarot/gh-desk/internal/tui/keymap"
	"github.com/babarot/gh-desk/internal/tui/statusline"
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
	PostComment(ctx context.Context, pr model.PR, body string) error
	MarkDone(ctx context.Context, n model.Notification) error
	Browse(url string) error
	// MergeInfo reads what deciding a merge needs, by node ID; with
	// details, each check (and whether it is required) and the files.
	MergeInfo(ctx context.Context, ids []string, details bool) (map[string]model.MergeInfo, error)
	// Bodies reads descriptions by node ID, for rules that read them.
	Bodies(ctx context.Context, ids []string) (map[string]string, error)
	// Heavy reads the fields a list's search leaves out, by node ID, each
	// for the head it has now.
	Heavy(ctx context.Context, ids []string) (map[string]model.Heavy, error)
}

// Layout keeps how the screen was left, for the next launch. The app
// package passes the state package's Layout.
type Layout interface {
	SavePreviewHeight(h int) error
}

// Runner approves and merges batches of pull requests.
type Runner interface {
	Start(ctx context.Context, kind runner.Kind, items []runner.Item) (string, <-chan runner.Event)
	StartReview(ctx context.Context, rv runner.ReviewText, items []runner.Item) (string, <-chan runner.Event)
	Resume(ctx context.Context, ps []model.Pending) (string, <-chan runner.Event)
}

// Tab is a tab: pull requests found by a search, or the inbox.
type Tab struct {
	Name  string
	Inbox bool
	Query string
	// Groups are the groups a search's rows are split into, in the order
	// shown; none leaves them unsplit.
	Groups []plan.Group
	Folded []plan.Group
	// DraftsLast sorts drafts after the rest.
	DraftsLast bool
	// Limit is how many pull requests, or notifications, to read.
	Limit int
	// ShowSettled has the inbox show the notifications whose pull request
	// is merged or closed, rather than leave them out with the excluded.
	ShowSettled bool
}

// Settings are the config values the TUI uses, as plain values.
type Settings struct {
	// Tabs are the tabs in the order shown; exactly one is the inbox.
	Tabs []Tab
	// MarkDone marks a pull request's notification done after a review
	// is sent here.
	MarkDone bool
	// Columns are the list's columns in their order, by id; nil for all of
	// them in the default order.
	Columns []string
	// Menu are the actions the menu of enter offers, in their order; nil
	// for all of them in the default order.
	Menu       []string
	HomeOwners []string
	StaleDays  int
	// PreviewHeight is the preview pane's lines from the config file; 0
	// hides it until it is toggled.
	PreviewHeight int
	// PaneHeight is the pane's lines last set with the mouse or keys; 0
	// when none was, and PreviewHeight is taken.
	PaneHeight int
	// DefaultPaneHeight is the pane's lines when neither PreviewHeight nor
	// PaneHeight gives them: opened after starting hidden.
	DefaultPaneHeight int
	// OpenOnDiff opens the pane on the diff instead of the overview.
	OpenOnDiff bool
	// PlainDescription shows the Markdown of a description and its comments
	// as written instead of drawing it.
	PlainDescription bool
	// Layout keeps the pane's height when it is changed; nil keeps it
	// for this run only.
	Layout   Layout
	Collapse []string
	// ThemeDark and ThemeLight are the themes on a dark terminal and on a
	// light one. ThemeDark is used until the terminal tells its
	// background, and on one that does not.
	ThemeDark, ThemeLight string
	// Spinner is the name of the spinner shown while something is read.
	Spinner string
	// Shimmer runs a light over the text that says what is going on.
	Shimmer bool
	// Version is gh-desk's version, told at the foot of the ? popup.
	Version string
	// Editor is the command ctrl+e runs to edit a review, with its
	// arguments; empty turns it off.
	Editor string
	// Plan is what deciding a merge takes from the config file.
	Plan plan.Settings
	// Language is what Claude answers in.
	Language string
	// ReleaseNotes has Claude read the release notes of the bot pull
	// requests in a merge popup.
	ReleaseNotes bool
	// Rules are what S offers besides "approved by me".
	Rules []plan.Rule
	// Exclude are the rules that leave pull requests out of every tab
	// until they are shown with a key; a pull request one of them says Yes
	// of is left out.
	Exclude []plan.Rule
	// Resume are the merges an earlier run was waiting for.
	Resume []model.Pending
	// CanWait is whether this gh-desk holds the lock on the record of
	// pending merges. Without it, merges that wait are left out, since
	// nothing would go on with them after quitting.
	CanWait bool
	// ConfigWarnings are what the config file likely did not mean, one
	// line each with where it is; they are shown as a notification at
	// start.
	ConfigWarnings []string
	// StoreProblem says why the record could not be opened, when it was
	// not another gh-desk holding it.
	StoreProblem string
	// Lock is tried again while CanWait is false, to take the record
	// over from a gh-desk that quit; nil never tries.
	Lock Lock
	// Host is the GitHub host the profiles and pull requests are on, for
	// their web pages; empty is github.com.
	Host string
	// Viewer is the login the app read before starting, which the cached
	// lists are of; empty has the TUI read it.
	Viewer string
	// Lists are the lists last read; nil keeps none.
	Lists Lists
	// Chats keep the conversations with Claude; nil keeps them only while
	// gh-desk runs.
	Chats chat.Store
	// Refresh is how often the shown tab is read again; zero turns it off.
	Refresh time.Duration
	// Now is the clock, for tests; nil is time.Now.
	Now func() time.Time
	// Tick asks for msg after d, for tests; nil is tea.Tick.
	Tick func(d time.Duration, msg tea.Msg) tea.Cmd
	// LogRecords are the records written to the log while gh-desk runs;
	// nil has none.
	LogRecords LogRecords
}

// LogRecords are the latest records of the log, the same the log file
// has.
type LogRecords interface {
	// All is the records kept, oldest first.
	All() []logging.Record
}

// Model is the TUI's state.
type Model struct {
	src    Source
	run    Runner
	claude Claude
	keys   keymap.Map
	log    *slog.Logger
	set    Settings
	ctx    context.Context
	st     styles

	viewer string
	tabs   []*tab
	cur    int
	// inbox is the inbox's place among the tabs.
	inbox int
	// tabFrom is the first tab the header shows when the tabs do not fit.
	tabFrom int
	unread  map[model.PRKey]model.Notification

	width, height int
	help          bool
	helpTop       int             // the first line of the ? popup shown
	helpFolded    map[string]bool // the ? popup's sections folded, by title
	helpBox       helpBox
	// popupAt is where the popup shown was last drawn, for the mouse.
	popupAt popupBox
	// line is what the status line says (ADR-025), and statusTickAt when
	// the redraw asked for it is due.
	line         statusline.Line
	statusTickAt time.Time
	// holder is the gh-desk holding the record of pending merges, while
	// this one runs without it.
	holder holder
	// taking is set while a try to take the lock is on its way.
	taking bool

	filtering bool
	filter    string

	// dragging is set while the pane's top edge is held with the mouse,
	// lastClick is the row clicked last, to tell a double click, and
	// armed is the link clicked once, waiting for the click that opens
	// it; nil when none is.
	dragging  bool
	lastClick click
	armed     *armed

	pane  pane
	cache cache
	// sending names the pull requests a comment is on its way to; none
	// other is sent until it is back. Empty when none is.
	sending string
	// posting is the comment on its way, and unsent the text of one that
	// failed on some, which the next comment starts with.
	posting *posting
	unsent  string
	// unsentReview is the review that was not sent to some of its pull
	// requests, which the next review starts with.
	unsentReview *runner.ReviewText

	selected map[model.PRKey]selection
	bulk     *bulk
	picker   *pick
	menu     *actionMenu
	// progress is each pull request's step or result in a batch, shown on
	// its row until a refresh.
	progress map[model.PRKey]progress
	batches  map[string]*batchState
	// batchOrder is the batches as they started.
	batchOrder []string
	notes      []*note
	notesAt    *notesView
	toast      *note
	merged     map[model.PRKey]bool
	// held is where each row showing a result sorts while it shows it.
	held map[model.PRKey]heldSort
	// refreshed is when each tab was last read.
	refreshed map[int]time.Time
	// refreshing is the reading r started, while it lasts.
	refreshing *refreshRun
	// rereading is a reading of the lists again, by r or the clock, until
	// it is logged.
	rereading *rereading
	// logv is the log viewer while it is open; logGen counts its openings.
	logv   *logView
	logGen int
	// telling is r's reading of the shown tab, while it lasts, so that
	// its end is said.
	telling *telling
	compose *compose
	confirm *confirm
	// chats are the conversations with Claude, by pull request; chatState
	// keeps them and takes their locks. claudeOpen puts the claude pane in
	// place of the preview.
	chats      map[model.PRKey]*chatScreen
	chatState  *chat.State
	claudeOpen bool
	// chatKey is the pull request the claude pane was opened on; it stays
	// on it while the pane is open, as the list takes no keys meanwhile.
	chatKey model.PRKey
	// triage is the conversation with Claude about the lists, once begun.
	triage *triage
	// cautions are what Claude said of release notes, by notesID; empty
	// when nothing calls for care.
	cautions map[string]string

	// heavyQueue are the reads of heavy fields not sent yet, and heavyBusy
	// how many are on their way.
	heavyQueue []heavyJob
	heavyBusy  int
	// spinning is set while a spinner tick is on its way.
	spinning bool
	// spinStyle is the spinner of Settings.Spinner.
	spinStyle spinnerStyle
	// fresh are the rows a reading again added or changed, marked until
	// the time given; freshGen tells the last mark's end from earlier ones.
	fresh    map[model.PRKey]time.Time
	freshGen int
}

// New returns the TUI reading from src, acting through run, and asking cl;
// a nil cl turns Claude off.
func New(src Source, run Runner, cl Claude, km keymap.Map, log *slog.Logger, set Settings) *Model {
	if set.Now == nil {
		set.Now = time.Now
	}
	if log == nil {
		log = logging.Discard()
	}
	m := &Model{src: src, run: run, claude: cl, cautions: map[string]string{}, fresh: map[model.PRKey]time.Time{}, keys: km, log: log, set: set, ctx: context.Background(), unread: map[model.PRKey]model.Notification{},
		selected: map[model.PRKey]selection{}, progress: map[model.PRKey]progress{}, batches: map[string]*batchState{},
		merged: map[model.PRKey]bool{}, held: map[model.PRKey]heldSort{}, refreshed: map[int]time.Time{}}
	m.chats = map[model.PRKey]*chatScreen{}
	m.chatState = chat.NewState(set.Chats, log, set.Now, chatStatus{m})
	m.st = newStyles(theme(set.ThemeDark))
	m.spinStyle = spinner(set.Spinner)
	for i, t := range set.Tabs {
		if t.Inbox {
			m.inbox = i
		}
		m.tabs = append(m.tabs, newTab(t))
	}
	m.pane = pane{on: set.PreviewHeight > 0, height: cmp.Or(set.PaneHeight, set.PreviewHeight, set.DefaultPaneHeight), files: map[model.PRKey]map[string]bool{}, file: map[model.PRKey]string{}, tabRow: -1, folded: map[string]bool{}}
	if set.OpenOnDiff {
		m.pane.mode = paneDiff
	}
	m.cache = newCache()
	m.viewer = set.Viewer
	m.loadCached()
	m.chatState.LoadKeys()
	return m
}

// quitWait is how long quitting waits for Claude Code to stop, after
// interrupting it. A constant: it only keeps quitting from hanging.
const quitWait = 2 * time.Second

// Run shows the TUI until it quits. Quitting stops every batch, and
// interrupts Claude, waiting for it a moment.
func Run(ctx context.Context, src Source, run Runner, cl Claude, km keymap.Map, log *slog.Logger, set Settings) error {
	ctx, cancel := context.WithCancel(ctx)
	m := New(src, run, cl, km, log, set)
	m.ctx = ctx
	_, err := tea.NewProgram(m, tea.WithContext(ctx)).Run()
	cancel()
	done := make(chan struct{})
	go func() {
		m.chatState.Asking().Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(quitWait):
	}
	return err
}

// Init asks for the terminal's background, the viewer, and every tab.
func (m *Model) Init() tea.Cmd {
	cmds := []tea.Cmd{m.tick(), m.resume(), m.configNote(), m.takeLock()}
	if m.viewer == "" {
		cmds = append(cmds, m.loadViewer())
	} else {
		cmds = append(cmds, m.loadAll())
	}
	if m.set.ThemeLight != m.set.ThemeDark {
		cmds = append(cmds, tea.RequestBackgroundColor)
	}
	return tea.Batch(cmds...)
}

// Update handles a message, and keeps the spinner turning while anything
// is read.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg.(type) {
	case tea.KeyPressMsg, tea.MouseMsg:
		// Only the user moves the cursor onto a row.
		before, _, had := m.currentPR()
		if _, ok := msg.(tea.KeyPressMsg); ok {
			m.line.KeyPressed()
			m.armed = nil
		}
		_, cmd := m.update(msg)
		m.followCursor(before.Key, had)
		m.endTabReading()
		m.endRefresh()
		return m, tea.Batch(cmd, m.spin(), m.statusTick())
	}
	// What is read may make rows come and go under an exclusion rule;
	// the cursor stays on its pull request, so that the next key does not
	// act on another one it was moved onto unseen.
	var back func()
	switch msg.(type) {
	case heavyMsg, checksMsg, bodiesMsg, notifsMsg, inboxFirstMsg, inboxPRsMsg:
		back = m.holdCursor(m.tab())
	}
	_, cmd := m.update(msg)
	if back != nil {
		back()
	}
	m.endTabReading()
	m.endRefresh()
	return m, tea.Batch(cmd, m.spin(), m.statusTick())
}

func (m *Model) update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		if m.logInPane() && m.layout().pane == 0 {
			// No room left for the log viewer; it would take the keys
			// unseen.
			m.closeLog()
		}
		if m.compose != nil {
			m.compose.resize(m.width, m.height)
		}
		for _, c := range m.chats {
			c.input.SetWidth(chatInputWidth(m.paneInner()))
		}
		return m, nil
	case tea.BackgroundColorMsg:
		if !msg.IsDark() {
			m.st = newStyles(theme(m.set.ThemeLight))
		}
		return m, nil
	case tea.KeyPressMsg:
		return m, m.key(msg)
	case tickMsg:
		return m, m.autoRefresh()
	case logTickMsg:
		return m, m.logTick(msg)
	case eventMsg:
		return m, m.event(msg)
	case lockMsg:
		return m, m.tookLock(msg)
	case toastMsg:
		if m.toast == msg.n {
			m.toast = nil
		}
		return m, nil
	case spinMsg:
		m.spinning = false
		m.watchChat()
		return m, nil
	case statusTickMsg:
		m.statusTickAt = time.Time{}
		return m, nil
	case disarmMsg:
		m.disarm()
		return m, nil
	case freshMsg:
		if msg.gen == m.freshGen {
			clear(m.fresh)
		}
		return m, nil
	case heavyMsg:
		return m, m.heavyRead(msg)
	case triageMsg:
		m.triageAnswered(msg)
		return m, nil
	case tea.PasteMsg:
		return m, m.paste(msg)
	case tea.MouseMsg:
		return m, m.mouse(msg)
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
	case m.menu != nil:
		return m.menuKey(k)
	case m.confirm != nil:
		return m.confirmKey(k)
	case m.triage != nil && m.triage.open:
		return m.triageKey(msg)
	case m.help:
		m.helpKey(k)
		return nil
	case m.filtering:
		return m.filterKey(msg)
	case m.compose != nil:
		return m.composeKey(msg)
	case m.typingChat():
		return m.chatTyping(msg)
	}
	if m.keys.Matches("help", k) {
		m.openHelp()
		return nil
	}
	if m.logv != nil {
		return m.logKey(k)
	}
	if m.claudeShown() {
		return m.chatWaiting(k)
	}
	return m.listKey(k)
}

func (m *Model) paste(msg tea.PasteMsg) tea.Cmd {
	switch {
	case m.filtering:
		m.filter += msg.Content
		m.tab().sel, m.tab().offset = 0, 0
	case m.triage != nil && m.triage.typing:
		var cmd tea.Cmd
		m.triage.input, cmd = m.triage.input.Update(msg)
		return cmd
	case m.compose != nil:
		return m.compose.update(msg)
	case m.typingChat():
		c := m.chat()
		var cmd tea.Cmd
		c.input, cmd = c.input.Update(msg)
		return cmd
	}
	return nil
}

// tab is the tab shown.
func (m *Model) tab() *tab { return m.tabs[m.cur] }
