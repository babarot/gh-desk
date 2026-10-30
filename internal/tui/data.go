package tui

import (
	"fmt"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/babarot/gh-triage/internal/diff"
	"github.com/babarot/gh-triage/internal/model"
	"github.com/babarot/gh-triage/internal/plan"
)

// tab is one tab: a section of pull requests, or the inbox.
type tab struct {
	name       string
	query      string
	grouped    bool
	folded     map[plan.Group]bool
	draftsLast bool
	limit      int

	inbox      bool
	showClosed bool

	loading  bool
	loaded   bool
	gen      int
	err      error
	prs      []model.PR
	total    int
	warnings []string

	notifs   []model.Notification
	inboxPRs map[model.PRKey]model.PR

	// hidden are pull requests put away here: hidden in a section, done in
	// the inbox.
	hidden map[model.PRKey]bool

	sel, offset int
	// keep is the pull request the cursor was on when a reading started.
	keep model.PRKey
}

func newSectionTab(s Section) *tab {
	t := &tab{name: s.Name, query: s.Query, grouped: s.Grouped, draftsLast: s.DraftsLast, limit: s.Limit,
		folded: map[plan.Group]bool{}, hidden: map[model.PRKey]bool{}}
	for _, g := range s.Folded {
		t.folded[plan.Group(g)] = true
	}
	return t
}

func newInboxTab(hideClosed bool) *tab {
	return &tab{name: "inbox", inbox: true, showClosed: !hideClosed, hidden: map[model.PRKey]bool{}, inboxPRs: map[model.PRKey]model.PR{}}
}

type rowKind int

const (
	rowPR rowKind = iota
	rowGroup
)

// row is a line of the list: a group's header, or a pull request with the
// notification it came from in the inbox.
type row struct {
	kind  rowKind
	group plan.Group
	count int
	pr    model.PR
	notif *model.Notification
}

// rows are the lines of t as shown: filtered, sorted, grouped and folded.
func (m *Model) rows(t *tab) []row {
	if t.inbox {
		rs, _ := m.inboxRows(t)
		return rs
	}
	var prs []model.PR
	for _, p := range t.prs {
		if !t.hidden[p.Key] && m.matches(p) {
			prs = append(prs, p)
		}
	}
	slices.SortStableFunc(prs, func(a, b model.PR) int {
		if t.draftsLast && a.Draft != b.Draft {
			if a.Draft {
				return 1
			}
			return -1
		}
		return b.Updated.Compare(a.Updated)
	})
	if !t.grouped {
		out := make([]row, len(prs))
		for i, p := range prs {
			out[i] = row{kind: rowPR, pr: p}
		}
		return out
	}
	var out []row
	for _, g := range plan.Groups {
		var in []model.PR
		for _, p := range prs {
			if plan.GroupOf(p, m.viewer) == g {
				in = append(in, p)
			}
		}
		out = append(out, row{kind: rowGroup, group: g, count: len(in)})
		if !t.folded[g] {
			for _, p := range in {
				out = append(out, row{kind: rowPR, pr: p})
			}
		}
	}
	return out
}

// inboxRows are the inbox's lines, and how many notifications on merged or
// closed pull requests they leave out.
func (m *Model) inboxRows(t *tab) ([]row, int) {
	var out []row
	closed := 0
	for i := range t.notifs {
		n := &t.notifs[i]
		if t.hidden[n.Key] {
			continue
		}
		p, ok := m.knownPR(n.Key)
		if !ok {
			p = model.PR{Key: n.Key, Title: n.Title, State: model.StateOpen}
		}
		if p.State != model.StateOpen && p.State != "" {
			closed++
			if !t.showClosed {
				continue
			}
		}
		if !m.matches(p) {
			continue
		}
		p.Updated = n.Updated
		out = append(out, row{kind: rowPR, pr: p, notif: n})
	}
	return out, closed
}

// knownPR finds a pull request in any tab.
func (m *Model) knownPR(k model.PRKey) (model.PR, bool) {
	for _, t := range m.tabs {
		if p, ok := t.inboxPRs[k]; ok {
			return p, true
		}
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

func (m *Model) matches(p model.PR) bool {
	if m.filter == "" {
		return true
	}
	f := strings.ToLower(m.filter)
	return strings.Contains(strings.ToLower(p.Key.String()+" "+p.Title+" "+p.Author), f)
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

// currentPR is the pull request the screen is about: the one opened, or
// the one under the cursor.
func (m *Model) currentPR() (model.PR, *model.Notification, bool) {
	if m.view != viewList {
		p, ok := m.knownPR(m.prv.key)
		return p, nil, ok
	}
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
	// the pull requests whose checks are pending and for a confirm popup.
	info     map[model.PRKey]model.MergeInfo
	bodies   map[model.PRKey]string
	diffs    map[string]*entry[[]diff.File]
	details  map[model.PRKey]*entry[model.Detail]
	comments map[string]*entry[model.Comment]
}

func newCache() cache {
	return cache{info: map[model.PRKey]model.MergeInfo{}, bodies: map[model.PRKey]string{}, diffs: map[string]*entry[[]diff.File]{}, details: map[model.PRKey]*entry[model.Detail]{},
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
	notifsMsg struct {
		notifs []model.Notification
		err    error
	}
	inboxPRsMsg struct {
		prs map[model.PRKey]model.PR
		err error
	}
	diffMsg struct {
		id    string
		files []diff.File
		err   error
	}
	detailMsg struct {
		key model.PRKey
		d   model.Detail
		err error
	}
	commentMsg struct {
		url string
		c   model.Comment
		err error
	}
	reviewMsg struct {
		pr    model.PR
		event model.ReviewEvent
		err   error
	}
	doneMsg struct {
		n   model.Notification
		err error
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
	if t.inbox {
		limit := m.set.InboxLimit
		return func() tea.Msg {
			ns, err := m.src.Notifications(m.ctx, limit)
			return notifsMsg{ns, err}
		}
	}
	t.gen++
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
		n = min(morePage, t.limit-len(t.prs))
	}
	return func() tea.Msg {
		prs, total, next, warnings, err := m.src.SearchPage(m.ctx, query, n, after)
		return tabMsg{tab: i, gen: gen, more: more, prs: prs, total: total, next: next, warnings: warnings, err: err}
	}
}

// handleData takes in what a command read.
func (m *Model) handleData(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case viewerMsg:
		if msg.err != nil {
			m.setStatus("could not read who you are on GitHub: "+msg.err.Error(), true)
			m.log.Error("viewer", "err", msg.err.Error())
			return nil
		}
		m.viewer = msg.login
		return tea.Batch(m.loadAll())
	case tabMsg:
		t := m.tabs[msg.tab]
		if msg.gen != t.gen {
			return nil
		}
		if msg.err != nil {
			t.loading, t.loaded, t.err = false, true, msg.err
			m.log.Error("search", "tab", t.name, "err", msg.err.Error())
			return nil
		}
		t.loaded, t.err, t.total = true, nil, msg.total
		if msg.more {
			t.prs = append(t.prs, safePRs(msg.prs)...)
			t.warnings = append(t.warnings, msg.warnings...)
		} else {
			t.prs, t.warnings = safePRs(msg.prs), msg.warnings
			m.restoreCursor(t)
		}
		// What was read of a pull request's checks is for its head; a new
		// push makes it stale.
		for _, p := range msg.prs {
			if mi, ok := m.cache.info[p.Key]; ok && mi.HeadOID != p.HeadOID {
				delete(m.cache.info, p.Key)
			}
		}
		m.refreshed[msg.tab] = m.set.Now()
		if msg.next != "" && len(t.prs) < t.limit {
			return tea.Batch(m.ensure(), m.searchPage(msg.tab, msg.gen, msg.next, true))
		}
		t.loading = false
		return tea.Batch(m.ensure(), m.ensureChecks(t.prs), m.dropGone(t))
	case notifsMsg:
		t := m.inboxTab()
		t.loading = false
		if msg.err != nil {
			t.loaded, t.err = true, msg.err
			m.log.Error("notifications", "err", msg.err.Error())
			return nil
		}
		t.notifs = make([]model.Notification, len(msg.notifs))
		for i, n := range msg.notifs {
			t.notifs[i] = safeNotification(n)
		}
		// What GitHub says now replaces what was put away here: a thread
		// with new activity is unread again.
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
		return func() tea.Msg {
			prs, err := m.src.PullRequests(m.ctx, keys)
			return inboxPRsMsg{prs, err}
		}
	case inboxPRsMsg:
		t := m.inboxTab()
		t.loaded, t.err = true, msg.err
		if msg.prs != nil {
			t.inboxPRs = map[model.PRKey]model.PR{}
			for k, p := range msg.prs {
				t.inboxPRs[k] = safePR(p)
			}
		}
		return m.ensure()
	case diffMsg:
		e := m.cache.diffs[msg.id]
		e.loading, e.err, e.val = false, msg.err, safeFiles(msg.files)
	case detailMsg:
		e := m.cache.details[msg.key]
		e.loading, e.err, e.val = false, msg.err, safeDetail(msg.d)
	case commentMsg:
		e := m.cache.comments[msg.url]
		e.loading, e.err, e.val = false, msg.err, safeComment(msg.c)
	case reviewMsg:
		return m.reviewed(msg)
	case doneMsg:
		if msg.err != nil {
			m.setStatus(fmt.Sprintf("could not mark %s done: %v", msg.n.Key, msg.err), true)
			delete(m.inboxTab().hidden, msg.n.Key)
			if msg.n.Unread {
				m.unread[msg.n.Key] = msg.n
			}
		}
	case browseMsg:
		if msg.err != nil {
			m.setStatus("could not open the browser: "+msg.err.Error(), true)
		}
	case editedMsg:
		return m.edited(msg)
	case bulkInfoMsg:
		return m.bulkInfo(msg)
	case bodiesMsg:
		m.bodies(msg)
	case checksMsg:
		for k, mi := range msg.got {
			m.cache.info[k] = mi
		}
	}
	return nil
}

func (m *Model) inboxTab() *tab { return m.tabs[len(m.tabs)-1] }

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
	if p.AuthorIsBot || m.pane.mode == paneBody || m.view == viewPR {
		cmds = append(cmds, m.ensureDetail(p.Key))
	}
	if n != nil && n.Unread && n.LatestCommentURL != "" {
		cmds = append(cmds, m.ensureComment(n.LatestCommentURL))
	}
	if m.view == viewList {
		rs := m.rows(m.tab())
		for _, d := range []int{1, -1, 2} {
			if i := m.tab().sel + d; i >= 0 && i < len(rs) && rs[i].kind == rowPR {
				cmds = append(cmds, m.ensureDiff(rs[i].pr))
			}
		}
	}
	return tea.Batch(cmds...)
}

func (m *Model) ensureDiff(p model.PR) tea.Cmd {
	id := diffID(p)
	if _, ok := m.cache.diffs[id]; ok || p.HeadOID == "" {
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
	m.cache.details[k] = &entry[model.Detail]{loading: true}
	return func() tea.Msg {
		d, err := m.src.Detail(m.ctx, k)
		return detailMsg{k, d, err}
	}
}

func (m *Model) ensureComment(url string) tea.Cmd {
	if _, ok := m.cache.comments[url]; ok {
		return nil
	}
	m.cache.comments[url] = &entry[model.Comment]{loading: true}
	return func() tea.Msg {
		c, err := m.src.Comment(m.ctx, url)
		return commentMsg{url, c, err}
	}
}

type checksMsg struct {
	got map[model.PRKey]model.MergeInfo
}

// ensureChecks reads the single checks of the pull requests whose checks
// are pending, in one query, to tell those waiting only for an approval
// (CI shows ◌) from those still running.
func (m *Model) ensureChecks(prs []model.PR) tea.Cmd {
	if len(m.set.Plan.ApprovalChecks) == 0 {
		return nil
	}
	var ids []string
	keys := map[string]model.PRKey{}
	for _, p := range prs {
		if _, ok := m.cache.info[p.Key]; ok || p.ID == "" {
			continue
		}
		if p.Checks == model.CheckPending || p.Checks == model.CheckExpected {
			ids = append(ids, p.ID)
			keys[p.ID] = p.Key
		}
		if len(ids) == 50 {
			break
		}
	}
	if len(ids) == 0 {
		return nil
	}
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
