package tui

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/babarot/gh-desk/internal/diff"
	"github.com/babarot/gh-desk/internal/logging"
	"github.com/babarot/gh-desk/internal/model"
	"github.com/babarot/gh-desk/internal/plan"
)

// tab is one tab: a section of pull requests, or the inbox.
type tab struct {
	name    string
	query   string
	grouped bool
	limit   int

	// sections are the parts the rows are split into, in the order shown;
	// folded are those folded, by name. other is the section that takes
	// what no section's rule does (plan.Layout).
	sections []plan.Section
	folded   map[string]bool
	other    int
	inbox    bool
	// keepSettled has the inbox show the notifications on merged or
	// closed pull requests rather than exclude them (show_settled).
	keepSettled bool
	// showExcluded shows the rows the exclusion leaves out, dimmed; a
	// toggles it.
	showExcluded bool

	loading  bool
	loaded   bool
	gen      int
	err      error
	prs      []model.PR
	total    int
	warnings []string

	notifs   []model.Notification
	inboxPRs map[model.PRKey]model.PR
	// readLive is set once the inbox is read from GitHub in this run, not
	// only from the cache.
	readLive bool
	// newUnread are the pull requests newly unread in the inbox's readings
	// since the bell last asked, rung for once their pull requests are
	// read.
	newUnread []model.PRKey

	// hidden are pull requests put away here: hidden in a section, done in
	// the inbox.
	hidden map[model.PRKey]bool
	// ruleFilter are the rules the rows are narrowed to, all of which a row
	// must match; nil for none. They are matched again at every draw, and
	// kept per tab since the bodies and heavy fields they read are read per
	// tab.
	ruleFilter []plan.Rule
	// readingBodies is set while the bodies a rule reads are on their way.
	readingBodies bool
	// bodiesFailing is set while reading the descriptions fails, so the
	// failure is kept in the notifications once.
	bodiesFailing bool

	sel, offset int
	// at is the row of the pull request the cursor is on, as last shown;
	// nil on a group's header or with no rows. seen is set once the user
	// has pressed a key or used the mouse with the cursor on it: only then
	// does the cursor follow it. gone is set while that pull request has
	// left the rows: its row stays, as last shown, on line goneAt, until
	// the cursor leaves it (ADR-115).
	at         *row
	seen, gone bool
	goneAt     int

	// readAt is when the rows were read, and asOf the same while they are
	// not what GitHub has now: from the cache until read again, or after a
	// reading again failed. Zero asOf is live.
	readAt, asOf time.Time
	// failing is set while reading the tab again fails, so the failure is
	// kept in the notifications once.
	failing bool
	// pend gathers a reading again of a tab that has rows, page by page,
	// to put in place at once when every page is in; nil otherwise.
	pend *pending
	// heavyLeft is how many reads of heavy fields of the reading gen are
	// not back.
	heavyLeft int
	// want is how many pull requests the reading gen lists, zero until
	// its first page is back, and heavyGot how many of them have their
	// heavy fields back: how far the reading is, for the status line.
	want, heavyGot int
	// told is set when the reading gen is said on the status line: the
	// first one, read while the tab has no rows to show.
	told bool
	// kept are the rows a reading again no longer had, kept to show their
	// results until a refresh.
	kept map[model.PRKey]bool
}

// pending is a reading again of a tab, gathered.
type pending struct {
	prs      []model.PR
	total    int
	warnings []string
}

func newTab(s Tab) *tab {
	t := &tab{name: s.Name, query: s.Query, grouped: len(s.Sections) > 0, limit: s.Limit,
		inbox: s.Inbox, keepSettled: s.ShowSettled, folded: map[string]bool{}, hidden: map[model.PRKey]bool{}}
	t.sections, t.other = plan.Layout(s.Sections)
	for _, sc := range t.sections {
		if sc.Show == plan.ShowFolded && sc.Name != "" {
			t.folded[sc.Name] = true
		}
	}
	if s.Inbox {
		t.inboxPRs = map[model.PRKey]model.PR{}
	}
	return t
}

type rowKind int

const (
	rowPR rowKind = iota
	rowGroup
)

// row is a line of the list: a section's head, or a pull request with the
// notification it came from in the inbox.
type row struct {
	kind rowKind
	// section is the tab's section a head is of, an index of tab.sections.
	section int
	count   int
	pr      model.PR
	notif   *model.Notification
	// ex is why the row is excluded, shown only while the tab shows the
	// excluded, and whether an exclusion rule waits to tell.
	ex exclusion
	// gone is set on the row of the pull request the cursor is on, kept
	// after it left the rows.
	gone bool
}

// rows are the lines of t as shown: filtered, sorted, grouped and folded,
// with the row the cursor stays on after its pull request left them.
func (m *Model) rows(t *tab) []row {
	rs := m.rowsOf(t, m.filter, t.ruleFilter, t.folded)
	if !t.gone || !m.stays(t) || slices.ContainsFunc(rs, isPR(t.at.pr.Key)) {
		return rs
	}
	r := *t.at
	r.gone = true
	return slices.Insert(rs, min(t.goneAt, len(rs)), r)
}

// isPR reports whether a row is that of the pull request k.
func isPR(k model.PRKey) func(row) bool {
	return func(r row) bool { return r.kind == rowPR && r.pr.Key == k }
}

// listed are the pull requests of t in the order its rows show them, with
// no filter and no group folded; hidden ones are left out.
func (m *Model) listed(t *tab) []row {
	var out []row
	for _, r := range m.rowsOf(t, "", nil, nil) {
		if r.kind == rowPR {
			out = append(out, r)
		}
	}
	return out
}

// rowsOf are the lines of t under filter and rules, with the sections in
// folded folded.
func (m *Model) rowsOf(t *tab, filter string, rules []plan.Rule, folded map[string]bool) []row {
	if t.inbox {
		return m.inboxRows(t, filter, rules, folded)
	}
	var prs []model.PR
	ex := map[model.PRKey]exclusion{}
	now := m.set.Now()
	for _, p := range t.prs {
		if x, ok := m.shows(t, p, nil, filter, rules, now); ok {
			prs = append(prs, p)
			ex[p.Key] = x
		}
	}
	// A row showing a result sorts where it was when its batch started.
	slices.SortStableFunc(prs, func(a, b model.PR) int {
		return m.asHeld(b).Updated.Compare(m.asHeld(a).Updated)
	})
	rs := make([]row, len(prs))
	for i, p := range prs {
		rs[i] = row{kind: rowPR, pr: p, ex: ex[p.Key]}
	}
	return t.split(rs, folded)
}

// split lays the rows of t out in its sections, in their order, each under
// its head, those in folded folded; a tab with no sections has them as
// they are.
func (t *tab) split(rs []row, folded map[string]bool) []row {
	if !t.grouped {
		return rs
	}
	var out []row
	for i, sc := range t.sections {
		var in []row
		for _, r := range rs {
			if r.ex.at == i {
				in = append(in, r)
			}
		}
		// A section with no rows, filtered or not, is left out with its
		// head.
		if len(in) == 0 {
			continue
		}
		// A section with no name has no head: its rows follow the section
		// above, and it never folds.
		if sc.Name == "" {
			out = append(out, in...)
			continue
		}
		out = append(out, row{kind: rowGroup, section: i, count: len(in)})
		if !folded[sc.Name] {
			out = append(out, in...)
		}
	}
	return out
}

// inboxRows are the inbox's lines under filter and rules, with the
// sections in folded folded.
func (m *Model) inboxRows(t *tab, filter string, rules []plan.Rule, folded map[string]bool) []row {
	now := m.set.Now()
	var out []row
	for i := range t.notifs {
		n := &t.notifs[i]
		p := m.notifPR(n)
		x, ok := m.shows(t, p, n, filter, rules, now)
		if !ok {
			continue
		}
		// The row shows the notification's time; the rules matched the
		// pull request's own.
		p.Updated = n.Updated
		out = append(out, row{kind: rowPR, pr: p, notif: n, ex: x})
	}
	return t.split(out, folded)
}

// notifPR is the pull request of the notification n, as far as it is
// read.
func (m *Model) notifPR(n *model.Notification) model.PR {
	if p, ok := m.knownPR(n.Key); ok {
		return p
	}
	return model.PR{Key: n.Key, Title: n.Title, State: model.StateOpen}
}

// settled reports whether p is merged or closed.
func settled(p model.PR) bool {
	return p.State != model.StateOpen && p.State != ""
}

// settledName is what the inbox says excludes a notification on a merged
// or closed pull request.
const settledName = "merged or closed"

// exclusion is why a pull request is left out of a tab: the names of what
// excludes it (the exclusion rules that say Yes of it, the tab's excluded
// section it is in, and settledName in the inbox), and, when none does,
// whether a rule or a section cannot tell yet.
type exclusion struct {
	by      []string
	pending bool
	// settled is set when the inbox excludes it as merged or closed.
	settled bool
	// at is the tab's section it is in, an index of tab.sections; -1 in a
	// tab with no sections, or when no section takes it.
	at int
}

func (x exclusion) excluded() bool { return len(x.by) > 0 }

// place is the section of t that p goes in, and whether a section passed
// over could not tell yet: what no section takes goes in other.
func (t *tab) place(p model.PR, viewer string, f plan.Facts) (int, bool) {
	return plan.PlaceIn(t.sections, t.other, p, viewer, f)
}

// sectionOf is the section of t that p is in, -1 when no section takes
// it or t has none.
func (m *Model) sectionOf(t *tab, p model.PR) int {
	if !t.grouped {
		return -1
	}
	at, _ := m.placeIn(t, p, m.notifOf(t, p.Key), m.set.Now())
	return at
}

// placeIn is the section of t that p, the pull request of the notification
// n in the inbox, is in, and whether a section was passed over as it cannot
// tell yet. A row showing a result is placed as it was when its batch
// started (hold).
func (m *Model) placeIn(t *tab, p model.PR, n *model.Notification, now time.Time) (int, bool) {
	p = m.asHeld(p)
	f := m.ruleFacts(p, now)
	if n != nil {
		// In the inbox, a section may ask for the reason of the row's
		// notification.
		f.Reason = n.Reason
	}
	return t.place(p, m.viewer, f)
}

// unfoldFor unfolds the section of t that p is in.
func (m *Model) unfoldFor(t *tab, p model.PR) {
	t.setFolded(m.sectionOf(t, p), false)
}

// foldedAt reports whether the section i of t is folded. A section with no
// name has no head to unfold it by, so it is never folded.
func (t *tab) foldedAt(i int) bool {
	return i >= 0 && i < len(t.sections) && t.sections[i].Name != "" && t.folded[t.sections[i].Name]
}

// setFolded folds or unfolds the section i of t; one with no name stays
// as it is.
func (t *tab) setFolded(i int, folded bool) {
	if i >= 0 && i < len(t.sections) && t.sections[i].Name != "" {
		t.folded[t.sections[i].Name] = folded
	}
}

// sectionNamed is the section of t called name, -1 when none is.
func (t *tab) sectionNamed(name string) int {
	return slices.IndexFunc(t.sections, func(s plan.Section) bool { return s.Name == name })
}

// hasRowsIn reports whether the section i of t holds a row, filtered or
// not: what Claude is told of the tab.
func (m *Model) hasRowsIn(t *tab, i int) bool {
	return slices.ContainsFunc(m.listed(t), func(r row) bool { return r.ex.at == i })
}

// sectionName is what a section is called where it is shown: its name, or
// what its rule asks for when it has none.
func sectionName(s plan.Section) string {
	if s.Name != "" {
		return s.Name
	}
	return ruleName(s.Rule)
}

// ruleName is what a rule is called where it is shown: its name, or what
// it asks for when it has none.
func ruleName(r plan.Rule) string {
	if r.Name != "" {
		return r.Name
	}
	return ruleWords(r)
}

// exclusionOf is why t leaves out p, the pull request of the notification
// n in the inbox (nil elsewhere): its rules are matched again each time,
// as the rules a tab is narrowed to are (ADR-027).
func (m *Model) exclusionOf(t *tab, p model.PR, n *model.Notification, now time.Time) exclusion {
	x := exclusion{at: -1}
	if t.inbox && !t.keepSettled && settled(p) {
		x.by = append(x.by, settledName)
		x.settled = true
	}
	if len(m.set.Exclude) == 0 && !t.grouped {
		return x
	}
	f := m.ruleFacts(p, now)
	if n != nil {
		f.Reason = n.Reason
	}
	if t.grouped {
		// A section that cannot tell yet is passed over, and the row is
		// not left out on it: it is drawn as not judged (ADR-062).
		at, pending := m.placeIn(t, p, n, now)
		x.at, x.pending = at, pending
		if at >= 0 && t.sections[at].Show == plan.ShowExcluded && !pending {
			x.by = append(x.by, sectionName(t.sections[at]))
		}
	}
	for _, r := range m.set.Exclude {
		switch r.Judge(p, m.viewer, f) {
		case plan.Yes:
			x.by = append(x.by, ruleName(r))
		case plan.Pending:
			x.pending = true
		}
	}
	if x.excluded() {
		x.pending = false
	}
	return x
}

// shows reports whether a row of t shows p, the pull request of the
// notification n in the inbox: not put away, not excluded unless the tab
// shows the excluded, and through filter and rules. Every place that asks
// whether a row is shown asks this, so that nothing out of sight is acted
// on (ADR-027). x is why it is excluded, when it is shown.
func (m *Model) shows(t *tab, p model.PR, n *model.Notification, filter string, rules []plan.Rule, now time.Time) (x exclusion, ok bool) {
	if t.hidden[p.Key] {
		return exclusion{}, false
	}
	x = m.exclusionOf(t, p, n, now)
	if x.excluded() && !t.showExcluded {
		return x, false
	}
	return x, m.passes(p, filter, rules, now)
}

// excludedCount is how many of t's pull requests, or the inbox's
// notifications, are excluded, of them how many as merged or closed and
// how many unread, put away ones left out.
type excludedCount struct {
	n, settled, unread int
}

// excludedOf counts what t excludes, whatever is typed in the filter: the
// tab's count and the key line ask it.
func (m *Model) excludedOf(t *tab) excludedCount { return m.excludedPassing(t, "", nil) }

// excludedPassing counts what t excludes that passes filter and rules: what
// a would show among the rows narrowed. The line under the rows asks it, so
// that the excluded are filtered as the rows are, out of sight.
func (m *Model) excludedPassing(t *tab, filter string, rules []plan.Rule) excludedCount {
	var c excludedCount
	now := m.set.Now()
	add := func(p model.PR, n *model.Notification) {
		x := m.exclusionOf(t, p, n, now)
		if t.hidden[p.Key] || !x.excluded() || !m.passes(p, filter, rules, now) {
			return
		}
		c.n++
		if x.settled {
			c.settled++
		}
		if _, ok := m.unread[p.Key]; ok {
			c.unread++
		}
	}
	if t.inbox {
		for i := range t.notifs {
			add(m.notifPR(&t.notifs[i]), &t.notifs[i])
		}
		return c
	}
	for _, p := range m.allOf(t) {
		add(p, nil)
	}
	return c
}

// narrowed reports whether the rows of t are narrowed, by the filter typed
// or by the rules of S, so that its count is what the narrowed rows hold.
func (m *Model) narrowed(t *tab) bool { return m.filter != "" || t.ruleFilter != nil }

// narrowedCount is the count of t while its rows are narrowed: the pull
// requests its rows hold, or in the inbox the unread notifications they
// hold, with no group folded. Only what was read can be matched, so a
// search tab counts none beyond what it reads (ADR-083).
func (m *Model) narrowedCount(t *tab) int {
	n := 0
	for _, r := range m.rowsOf(t, m.filter, t.ruleFilter, nil) {
		if r.kind != rowPR {
			continue
		}
		if t.inbox {
			if _, ok := m.unread[r.pr.Key]; !ok {
				continue
			}
		}
		n++
	}
	return n
}

// unreadShown is how many unread notifications the inbox t shows: those
// it excludes count only while they are shown, so that the tab's count is
// what its rows hold. While the rows are narrowed, narrowedCount counts.
func (m *Model) unreadShown(t *tab) int {
	if t.showExcluded {
		return len(m.unread)
	}
	n, now := 0, m.set.Now()
	for _, nt := range m.unread {
		if !m.exclusionOf(t, m.notifPR(&nt), &nt, now).excluded() {
			n++
		}
	}
	return n
}

// knownPR finds a pull request in any tab: in the inbox first, which reads
// every field at once, then in the lists, whose heavy fields come later.
func (m *Model) knownPR(k model.PRKey) (model.PR, bool) {
	if p, ok := m.inboxTab().inboxPRs[k]; ok {
		return p, true
	}
	for _, t := range m.tabs {
		for _, p := range t.prs {
			if p.Key == k {
				return p, true
			}
		}
	}
	return model.PR{}, false
}

// updatePR replaces a pull request wherever a tab holds it.
func (m *Model) updatePR(p model.PR) {
	for _, t := range m.tabs {
		if _, ok := t.inboxPRs[p.Key]; ok {
			t.inboxPRs[p.Key] = p
		}
		for i := range t.prs {
			if t.prs[i].Key == p.Key {
				t.prs[i] = p
			}
		}
	}
}

// passes reports whether p is shown under the scope chosen, the filter
// typed and the rules the rows are narrowed to, as of now. Every place that asks whether a row
// is shown asks this, so nothing hidden by a filter is acted on.
func (m *Model) passes(p model.PR, filter string, rules []plan.Rule, now time.Time) bool {
	return m.inScope(p) && matches(p, filter) && (len(rules) == 0 || matchesAll(rules, p, m.viewer, m.ruleFacts(p, now)))
}

// current is the row under the cursor, if any.
func (m *Model) current() (row, bool) {
	t := m.tab()
	rs := m.rows(t)
	if len(rs) == 0 {
		return row{}, false
	}
	t.sel = min(max(t.sel, 0), len(rs)-1)
	return rs[t.sel], true
}

// currentPR is the pull request under the cursor.
func (m *Model) currentPR() (model.PR, *model.Notification, bool) {
	r, ok := m.current()
	if !ok || r.kind != rowPR {
		return model.PR{}, nil, false
	}
	return r.pr, r.notif, true
}

// entry is a value read on demand.
type entry[T any] struct {
	loading bool
	err     error
	val     T
}

// cache holds what is read for one pull request when it is looked at.
type cache struct {
	// info is what deciding a merge needs, with single checks, read for
	// the pull request the pane shows, those whose checks are pending and
	// for a confirm popup; infoRead are the pull requests the pane asked it
	// for, kept after a failed read so it is not asked again until r.
	info     map[model.PRKey]model.MergeInfo
	infoRead map[model.PRKey]bool
	// unknown counts the reads of each pull request's merge information
	// that found GitHub still working out whether it can be merged.
	unknown  map[model.PRKey]int
	bodies   map[model.PRKey]string
	diffs    map[string]*entry[[]diff.File]
	details  map[model.PRKey]*entry[model.Detail]
	comments map[string]*entry[model.Comment]
}

func newCache() cache {
	return cache{info: map[model.PRKey]model.MergeInfo{}, infoRead: map[model.PRKey]bool{}, unknown: map[model.PRKey]int{}, bodies: map[model.PRKey]string{}, diffs: map[string]*entry[[]diff.File]{}, details: map[model.PRKey]*entry[model.Detail]{},
		comments: map[string]*entry[model.Comment]{}}
}

// diffID names a pull request's diff at a head, so a new push reads it
// again.
func diffID(p model.PR) string { return p.Key.String() + "@" + p.HeadOID }

// Messages carrying what was read.
type (
	viewerMsg struct {
		login string
		err   error
	}
	tabMsg struct {
		tab int
		// gen is the reading the page belongs to; a page of an older one
		// is dropped.
		gen      int
		more     bool
		prs      []model.PR
		total    int
		next     string
		warnings []string
		err      error
	}
	// The inbox's messages carry the reading gen they are for; one of an
	// older reading is dropped.
	notifsMsg struct {
		gen    int
		notifs []model.Notification
		err    error
	}
	inboxPRsMsg struct {
		gen int
		prs map[model.PRKey]model.PR
		err error
	}
	diffMsg struct {
		id    string
		files []diff.File
		err   error
	}
	// A detail or a comment carries the cache entry its read was started
	// for: a refresh may have cleared that entry, or put another in its
	// place, and then the read is dropped.
	detailMsg struct {
		key model.PRKey
		e   *entry[model.Detail]
		d   model.Detail
		err error
	}
	commentMsg struct {
		url string
		e   *entry[model.Comment]
		c   model.Comment
		err error
	}
	doneMsg struct {
		n   model.Notification
		err error
		// asked is set when the user put it away, so a failure can be
		// tried again; one marked done after a review was not asked for.
		asked bool
	}
	browseMsg struct{ err error }
)

func (m *Model) loadViewer() tea.Cmd {
	return func() tea.Msg {
		login, err := m.src.Viewer(m.ctx)
		return viewerMsg{login, err}
	}
}

// loadAll reads every tab and the notifications.
func (m *Model) loadAll() tea.Cmd {
	var cmds []tea.Cmd
	for i := range m.tabs {
		cmds = append(cmds, m.loadTab(i))
	}
	return tea.Batch(cmds...)
}

func (m *Model) loadTab(i int) tea.Cmd {
	t := m.tabs[i]
	t.loading = true
	t.gen++
	t.want, t.heavyGot, t.told = 0, 0, !t.loaded
	if t.inbox {
		limit, gen := t.limit, t.gen
		return func() tea.Msg {
			ns, err := m.src.Notifications(m.ctx, limit)
			return notifsMsg{gen, ns, err}
		}
	}
	t.heavyLeft = 0
	t.pend = nil
	if len(t.prs) > 0 {
		t.pend = &pending{}
	}
	return m.searchPage(i, t.gen, "", false)
}

// First and later page sizes: GitHub's search takes longer the more it
// returns, so a small first page shows the tab soon.
const (
	firstPage = 20
	morePage  = 50
)

func (m *Model) searchPage(i, gen int, after string, more bool) tea.Cmd {
	t := m.tabs[i]
	query, n := t.query, min(firstPage, t.limit)
	if more {
		n = min(morePage, t.limit-t.reading())
	}
	return func() tea.Msg {
		prs, total, next, warnings, err := m.src.SearchPage(m.ctx, query, n, after)
		return tabMsg{tab: i, gen: gen, more: more, prs: prs, total: total, next: next, warnings: warnings, err: err}
	}
}

// handleData takes in what a command read.
func (m *Model) handleData(msg tea.Msg) tea.Cmd {
	// A message may finish the reading the scope kept waits for.
	defer m.checkScope()
	switch msg := msg.(type) {
	case viewerMsg:
		if msg.err != nil {
			m.log.Error("viewer", "err", msg.err.Error())
			return m.failNoted("could not read who you are on GitHub: "+msg.err.Error(), model.PRKey{})
		}
		m.viewer = msg.login
		return tea.Batch(m.loadAll(), m.autoApprove())
	case tabMsg:
		return m.tabRead(msg)
	case notifsMsg:
		t := m.inboxTab()
		if msg.gen != t.gen {
			return nil
		}
		if msg.err != nil {
			t.loading = false
			m.log.Error("notifications", "err", msg.err.Error())
			if len(t.notifs) > 0 {
				t.asOf = t.readAt
				return nil
			}
			t.loaded, t.err = true, msg.err
			return nil
		}
		first := firstInbox(t, msg.notifs)
		m.noteNewUnread(t, msg.notifs)
		keys := m.setNotifs(t, msg.notifs)
		gen := msg.gen
		return tea.Batch(m.readInboxFirst(gen, first), func() tea.Msg {
			prs, err := m.src.PullRequests(m.ctx, keys)
			return inboxPRsMsg{gen, prs, err}
		})
	case inboxFirstMsg:
		return m.inboxFirstRead(msg)
	case inboxPRsMsg:
		// The inbox is read once its pull requests are.
		t := m.inboxTab()
		if msg.gen != t.gen {
			return nil
		}
		t.loading, t.loaded, t.err = false, true, msg.err
		if msg.prs != nil {
			t.inboxPRs = map[model.PRKey]model.PR{}
			for k, p := range msg.prs {
				t.inboxPRs[k] = safePR(p)
			}
		}
		if msg.err != nil {
			return tea.Batch(m.ensure(), m.ringBell(t))
		}
		t.asOf, t.readAt = time.Time{}, m.set.Now()
		// The inbox reads every field at once, so its pull requests do
		// not come through the reads of heavy fields, which read the
		// single checks of the lists' ones.
		return tea.Batch(m.ensure(), m.saveInbox(t), m.readBodies(m.inbox), m.ensureChecks(slices.Collect(maps.Values(t.inboxPRs))), m.ringBell(t))
	case diffMsg:
		e := m.cache.diffs[msg.id]
		e.loading, e.err, e.val = false, msg.err, safeFiles(msg.files)
	case detailMsg:
		if m.cache.details[msg.key] != msg.e {
			return nil
		}
		msg.e.loading, msg.e.err, msg.e.val = false, msg.err, safeDetail(msg.d)
	case commentMsg:
		if m.cache.comments[msg.url] != msg.e {
			return nil
		}
		msg.e.loading, msg.e.err, msg.e.val = false, msg.err, safeComment(msg.c)
	case doneMsg:
		if msg.err != nil {
			delete(m.inboxTab().hidden, msg.n.Key)
			if msg.n.Unread {
				m.unread[msg.n.Key] = msg.n
			}
			what := fmt.Sprintf("could not mark %s done: %v", msg.n.Key, msg.err)
			if msg.asked {
				m.retryDone(msg.n)
				what += fmt.Sprintf(" · %s to try again", m.firstKey("retry"))
			}
			return m.failNoted(what, msg.n.Key)
		}
	case browseMsg:
		if msg.err != nil {
			m.log.Warn("could not open the browser", "err", msg.err.Error())
			m.fail("could not open the browser: " + msg.err.Error())
		}
	case notesMsg:
		return m.notesRead(msg)
	case chatStepMsg:
		return m.chatStep(msg)
	case chatReadyMsg:
		return m.chatReady(msg)
	case chatPreparedMsg:
		return m.chatPrepared(msg)
	case chatAnswerMsg:
		return m.chatAnswer(msg)
	case claudeExitMsg:
		return m.claudeExited(msg)
	case editedMsg:
		return m.edited(msg)
	case bulkInfoMsg:
		return m.bulkInfo(msg)
	case basedOnMsg:
		m.basedOnRead(msg)
		return nil
	case bodiesMsg:
		return m.bodies(msg)
	case checksMsg:
		maps.Copy(m.cache.info, msg.got)
	case infoMsg:
		return m.infoRead(msg)
	case infoAgainMsg:
		// Still unknown: read it again, if the pane still shows it.
		if mi, ok := m.cache.info[msg.key]; ok && mi.MergeState == model.MergeUnknown {
			delete(m.cache.info, msg.key)
			return m.ensure()
		}
	}
	return nil
}

func (m *Model) inboxTab() *tab { return m.tabs[m.inbox] }

// toFirstPR puts the cursor of a tab just read on its first pull request
// when it is still on the first line, a group's header.
func (m *Model) toFirstPR(t *tab) {
	if t.sel != 0 {
		return
	}
	for i, r := range m.rows(t) {
		if r.kind == rowPR {
			t.sel = i
			return
		}
	}
}

// ensure starts reading what the pull request under the cursor shows, and
// the diffs of its neighbors, so moving through the list does not wait.
func (m *Model) ensure() tea.Cmd {
	var cmds []tea.Cmd
	p, n, ok := m.currentPR()
	if !ok {
		return nil
	}
	cmds = append(cmds, m.ensureDiff(p))
	if m.pane.on {
		cmds = append(cmds, m.ensureInfo(p))
	}
	if p.AuthorIsBot || m.pane.mode == paneOverview {
		cmds = append(cmds, m.ensureDetail(p.Key))
	}
	if n != nil && n.Unread && n.LatestCommentURL != "" {
		cmds = append(cmds, m.ensureComment(n.LatestCommentURL))
	}
	rs := m.rows(m.tab())
	for _, d := range []int{1, -1, 2} {
		if i := m.tab().sel + d; i >= 0 && i < len(rs) && rs[i].kind == rowPR {
			cmds = append(cmds, m.ensureDiff(rs[i].pr))
		}
	}
	return tea.Batch(cmds...)
}

func (m *Model) ensureDiff(p model.PR) tea.Cmd {
	id := diffID(p)
	if _, ok := m.cache.diffs[id]; ok || p.HeadOID == "" {
		return nil
	}
	if m.set.Offline {
		m.cache.diffs[id] = &entry[[]diff.File]{err: errNotRead}
		return nil
	}
	m.cache.diffs[id] = &entry[[]diff.File]{loading: true}
	return func() tea.Msg {
		b, err := m.src.Diff(m.ctx, p)
		return diffMsg{id, diff.Parse(b), err}
	}
}

func (m *Model) ensureDetail(k model.PRKey) tea.Cmd {
	if _, ok := m.cache.details[k]; ok {
		return nil
	}
	if m.set.Offline {
		m.cache.details[k] = &entry[model.Detail]{err: errNotRead}
		return nil
	}
	e := &entry[model.Detail]{loading: true}
	m.cache.details[k] = e
	return func() tea.Msg {
		d, err := m.src.Detail(m.ctx, k)
		return detailMsg{key: k, e: e, d: d, err: err}
	}
}

// infoMsg is what deciding a merge needs of the pull request the pane
// shows.
type infoMsg struct {
	key model.PRKey
	mi  model.MergeInfo
	ok  bool
}

// ensureInfo reads what deciding a merge needs of an open pull request, so
// the pane says what a merge would do: that it conflicts, is behind its
// base, or waits for checks.
func (m *Model) ensureInfo(p model.PR) tea.Cmd {
	if _, ok := m.cache.info[p.Key]; ok || m.cache.infoRead[p.Key] || p.ID == "" || p.State != model.StateOpen {
		return nil
	}
	m.cache.infoRead[p.Key] = true
	return func() tea.Msg {
		got, err := m.src.MergeInfo(m.ctx, []string{p.ID}, true)
		if err != nil {
			m.log.Warn("merge info", logging.KeyPR, p.Key.String(), "err", err.Error())
		}
		mi, ok := got[p.ID]
		return infoMsg{key: p.Key, mi: mi, ok: ok}
	}
}

// infoAgainMsg asks for a pull request's merge information again, read
// while GitHub was still working out whether it can be merged.
type infoAgainMsg struct{ key model.PRKey }

// GitHub works out whether a pull request can be merged when it is first
// asked, and says it does not know yet meanwhile: such a read is tried
// again after unknownWait, up to unknownTries times.
const (
	unknownWait  = 3 * time.Second
	unknownTries = 3
)

func (m *Model) infoRead(msg infoMsg) tea.Cmd {
	if !msg.ok {
		return nil
	}
	m.cache.info[msg.key] = msg.mi
	delete(m.cache.infoRead, msg.key)
	if msg.mi.MergeState != model.MergeUnknown || m.cache.unknown[msg.key] >= unknownTries {
		return nil
	}
	m.cache.unknown[msg.key]++
	return m.after(unknownWait, infoAgainMsg{msg.key})
}

func (m *Model) ensureComment(url string) tea.Cmd {
	if _, ok := m.cache.comments[url]; ok {
		return nil
	}
	if m.set.Offline {
		m.cache.comments[url] = &entry[model.Comment]{err: errNotRead}
		return nil
	}
	e := &entry[model.Comment]{loading: true}
	m.cache.comments[url] = e
	return func() tea.Msg {
		c, err := m.src.Comment(m.ctx, url)
		return commentMsg{url: url, e: e, c: c, err: err}
	}
}

type checksMsg struct {
	got map[model.PRKey]model.MergeInfo
}

// checksBatch is the most pull requests one read of single checks takes.
const checksBatch = 50

// ensureChecks reads the single checks of the pull requests whose checks
// are pending, a batch at a time, to tell those waiting only for an
// approval (CI shows ◌) from those still running. A rule on the checks
// waits for this, so none is left out.
func (m *Model) ensureChecks(prs []model.PR) tea.Cmd {
	if len(m.set.Plan.ApprovalChecks) == 0 {
		return nil
	}
	var ids []string
	keys := map[string]model.PRKey{}
	for _, p := range prs {
		if _, ok := m.cache.info[p.Key]; ok || p.ID == "" || keys[p.ID] != (model.PRKey{}) {
			continue
		}
		if p.Checks == model.CheckPending || p.Checks == model.CheckExpected {
			ids = append(ids, p.ID)
			keys[p.ID] = p.Key
		}
	}
	var cmds []tea.Cmd
	for batch := range slices.Chunk(ids, checksBatch) {
		cmds = append(cmds, m.readChecks(batch, keys))
	}
	return tea.Batch(cmds...)
}

// readChecks reads the single checks of the pull requests ids, whose keys
// keys gives.
func (m *Model) readChecks(ids []string, keys map[string]model.PRKey) tea.Cmd {
	return func() tea.Msg {
		got, err := m.src.MergeInfo(m.ctx, ids, true)
		if err != nil {
			m.log.Warn("checks", "err", err.Error())
		}
		out := map[model.PRKey]model.MergeInfo{}
		for id, mi := range got {
			out[keys[id]] = mi
		}
		return checksMsg{out}
	}
}

// reading is how many pull requests the tab's reading has so far.
func (t *tab) reading() int {
	if t.pend != nil {
		return len(t.pend.prs)
	}
	return len(t.prs)
}

// tabRead takes in a page of a tab's search. A tab with no rows shows each
// page as it comes; one with rows keeps them until every page of the new
// reading is in, so a list of 98 never drops to the 50 of a first page.
func (m *Model) tabRead(msg tabMsg) tea.Cmd {
	t := m.tabs[msg.tab]
	if msg.gen != t.gen {
		return nil
	}
	now := m.set.Now()
	if msg.err != nil {
		t.loading = false
		m.log.Error("search", "tab", t.name, "err", msg.err.Error())
		if t.pend != nil {
			// The rows stay, as of when they were read.
			t.pend = nil
			t.asOf = t.readAt
			if t == m.tab() {
				m.fail(fmt.Sprintf("could not read the %q tab: %s · showing the list as of %s · %s to try again",
					t.name, short80(msg.err), t.readAt.Local().Format("15:04"), m.firstKey("refresh")))
			}
			// Kept once, not at every reading the clock starts while the
			// network stays down.
			if t.failing {
				return nil
			}
			t.failing = true
			return m.pushNote("✗", fmt.Sprintf("could not read the %q tab: %s", t.name, short80(msg.err)), nil)
		}
		t.loaded, t.err = true, msg.err
		return nil
	}
	// What was read of a pull request's checks is for its head; a new
	// push makes it stale.
	for _, p := range msg.prs {
		if mi, ok := m.cache.info[p.Key]; ok && mi.HeadOID != p.HeadOID {
			delete(m.cache.info, p.Key)
			delete(m.cache.infoRead, p.Key)
			delete(m.cache.unknown, p.Key)
		}
	}
	t.want = min(msg.total, t.limit)
	prs := safePRs(msg.prs)
	if t.pend != nil {
		t.pend.prs = append(t.pend.prs, prs...)
		t.pend.total = msg.total
		t.pend.warnings = append(t.pend.warnings, msg.warnings...)
		if msg.next != "" && len(t.pend.prs) < t.limit {
			return m.searchPage(msg.tab, msg.gen, msg.next, true)
		}
		return m.swap(msg.tab)
	}
	t.loaded, t.err, t.total, t.asOf, t.failing = true, nil, msg.total, time.Time{}, false
	if msg.more {
		t.prs = append(t.prs, prs...)
		t.warnings = append(t.warnings, msg.warnings...)
	} else {
		t.prs, t.warnings, t.kept = prs, msg.warnings, nil
		m.firstRows(t)
	}
	cmds := []tea.Cmd{m.ensure(), m.queueHeavy(msg.tab, msg.gen, prs)}
	if msg.next != "" && len(t.prs) < t.limit {
		return tea.Batch(append(cmds, m.searchPage(msg.tab, msg.gen, msg.next, true))...)
	}
	t.loading, t.readAt = false, now
	m.refreshed[msg.tab] = now
	cmds = append(cmds, m.dropGone(t), m.readBodies(msg.tab))
	if m.tabDone(t) {
		// No heavy fields to wait for: an empty list, or all back.
		cmds = append(cmds, m.saveTab(msg.tab))
	}
	return tea.Batch(cmds...)
}

// swap puts a reading again in place of the rows. A row of the same pull
// request at the same head keeps its heavy fields until they are read
// again; rows added or changed are marked for a moment; rows that still
// show a result stay until a refresh.
func (m *Model) swap(i int) tea.Cmd {
	t := m.tabs[i]
	p := t.pend
	t.pend = nil
	now := m.set.Now()
	old := map[model.PRKey]model.PR{}
	for _, o := range t.prs {
		old[o.Key] = o
	}
	in := map[model.PRKey]bool{}
	fresh := false
	for j := range p.prs {
		n := &p.prs[j]
		in[n.Key] = true
		o, ok := old[n.Key]
		if ok && o.HeadOID == n.HeadOID && o.HeavyRead() {
			n.SetHeavy(o.Heavy())
		}
		if !ok || !o.Updated.Equal(n.Updated) {
			m.fresh[n.Key] = now.Add(freshFor)
			fresh = true
		}
	}
	rows := p.prs
	t.kept = nil
	for _, o := range t.prs {
		if _, shown := m.progress[o.Key]; shown && !in[o.Key] {
			if t.kept == nil {
				t.kept = map[model.PRKey]bool{}
			}
			t.kept[o.Key] = true
			rows = append(rows, o)
		}
	}
	t.prs, t.total, t.warnings = rows, p.total, p.warnings
	t.loading, t.loaded, t.err, t.asOf, t.readAt, t.failing = false, true, nil, time.Time{}, now, false
	m.refreshed[i] = now
	m.firstRows(t)
	cmds := []tea.Cmd{m.ensure(), m.queueHeavy(i, t.gen, p.prs), m.dropGone(t), m.readBodies(i)}
	if m.tabDone(t) {
		cmds = append(cmds, m.saveTab(i))
	}
	if fresh {
		m.freshGen++
		gen := m.freshGen
		cmds = append(cmds, m.after(freshFor, freshMsg{gen}))
	}
	return tea.Batch(cmds...)
}

func short80(err error) string {
	s, _, _ := strings.Cut(err.Error(), "\n")
	if len(s) > 80 {
		s = s[:79] + "…"
	}
	return s
}

// setNotifs shows notifications and returns their pull requests. What
// GitHub says replaces what was put away here: a thread with new activity
// is unread again.
func (m *Model) setNotifs(t *tab, ns []model.Notification) []model.PRKey {
	t.notifs = make([]model.Notification, len(ns))
	for i, n := range ns {
		t.notifs[i] = safeNotification(n)
	}
	t.hidden = map[model.PRKey]bool{}
	m.unread = map[model.PRKey]model.Notification{}
	var keys []model.PRKey
	for _, n := range t.notifs {
		if n.Unread {
			m.unread[n.Key] = n
		}
		if !slices.Contains(keys, n.Key) {
			keys = append(keys, n.Key)
		}
	}
	return keys
}
