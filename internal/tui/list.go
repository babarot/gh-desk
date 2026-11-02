package tui

import (
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/babarot/gh-triage/internal/keymap"
	"github.com/babarot/gh-triage/internal/model"
	"github.com/babarot/gh-triage/internal/plan"
	"github.com/babarot/gh-triage/internal/runner"
)

// selection is who selected a pull request: the user, or Claude in a plan
// the user applied.
type selection int

const (
	byHand selection = iota + 1
	byClaude
)

func (m *Model) isSelected(k model.PRKey) bool {
	_, ok := m.selected[k]
	return ok
}

// listKey handles a key on the list.
func (m *Model) listKey(k string) tea.Cmd {
	t := m.tab()
	rs := m.rows(t)
	is := func(a keymap.Action) bool { return m.keys.Matches(a, k) }
	move := m.moveTo
	switch {
	case is("quit"):
		return tea.Quit
	case k == keymap.Back:
		switch {
		case len(m.selected) > 0:
			clear(m.selected)
		case m.filter != "":
			m.filter = ""
		default:
			return tea.Quit
		}
		return nil
	case is("down"):
		return move(t.sel + 1)
	case is("up"):
		return move(t.sel - 1)
	case is("page_down"):
		return move(t.sel + m.listHeight())
	case is("page_up"):
		return move(t.sel - m.listHeight())
	case is("top"):
		return move(0)
	case is("bottom"):
		return move(len(rs) - 1)
	case is("next_tab"):
		return m.switchTab(m.cur + 1)
	case is("prev_tab"):
		return m.switchTab(m.cur - 1)
	case tabKey(is) >= 0:
		if i := tabKey(is); i < len(m.tabs) {
			return m.switchTab(i)
		}
	case is("filter"):
		m.filtering = true
	case is("refresh"):
		return m.refresh()
	case is("notifications"):
		m.openNotes()
		return nil
	case is("select_shown"):
		for _, r := range rs {
			if r.kind == rowPR && !m.isSelected(r.pr.Key) {
				m.selected[r.pr.Key] = byHand
			}
		}
		return nil
	case is("clear_selection"):
		clear(m.selected)
		return nil
	case is("select_rule"):
		return m.openPick()
	case is("claude"):
		return m.openTriage()
	case is("approve") && len(m.selected) > 0:
		return m.startBulk(runner.Approve)
	case is("merge"):
		return m.startBulk(runner.Merge)
	case is("ready_for_review"):
		return m.startBulk(runner.Ready)
	case is("comment"):
		return m.startComment()
	case is("review") && len(m.selected) > 0:
		m.setStatus("review: one pull request at a time", true)
		return nil
	case is("toggle_bots"):
		if t.grouped {
			t.folded[plan.GroupBots] = !t.folded[plan.GroupBots]
		}
	case is("show_settled"):
		if t.inbox {
			t.showSettled = !t.showSettled
			t.sel = 0
		}
	case is("toggle_pane"):
		m.pane.on = !m.pane.on
	case is("pane_grow"):
		return m.resizePane(m.pane.height + 1)
	case is("pane_shrink"):
		return m.resizePane(m.pane.height - 1)
	case is("pane_tab"):
		m.pane.toggleMode()
		return m.ensure()
	case is("pane_down"):
		m.pane.scroll += 3
	case is("pane_up"):
		m.pane.scroll = max(0, m.pane.scroll-3)
	case is("next_file"):
		m.jumpFile(1)
	case is("prev_file"):
		m.jumpFile(-1)
	case is("unfold"):
		m.foldPart()
	}
	r, ok := m.current()
	if !ok {
		return nil
	}
	if is("open") {
		return m.openRow(r)
	}
	if r.kind == rowGroup {
		switch {
		case is("expand"):
			t.folded[r.group] = false
		case is("collapse"):
			t.folded[r.group] = true
		case is("select"):
			// A group's header selects every pull request in it, or clears
			// them when all are selected.
			var in []model.PRKey
			for _, p := range t.prs {
				if !t.hidden[p.Key] && matches(p, m.filter) && plan.GroupOf(p, m.viewer) == r.group {
					in = append(in, p.Key)
				}
			}
			all := len(in) > 0
			for _, k := range in {
				all = all && m.isSelected(k)
			}
			for _, k := range in {
				switch {
				case all:
					delete(m.selected, k)
				case !m.isSelected(k):
					m.selected[k] = byHand
				}
			}
		}
		return nil
	}
	p := r.pr
	switch {
	case is("select"):
		if m.isSelected(p.Key) {
			delete(m.selected, p.Key)
		} else {
			m.selected[p.Key] = byHand
		}
		return move(t.sel + 1)
	case is("approve"):
		return m.askApprove(p)
	case is("review"):
		return m.startCompose(p, model.EventComment)
	case is("pr_claude"):
		return m.openChat(p)
	case is("browse"):
		return m.browse(p)
	case is("hide"):
		return m.hide(p, r.notif)
	case is("expand"):
		return m.stepPane(p, 1)
	case is("collapse"):
		return m.stepPane(p, -1)
	}
	return nil
}

// moveTo puts the cursor on row to of the tab shown.
func (m *Model) moveTo(to int) tea.Cmd {
	t := m.tab()
	rs := m.rows(t)
	if len(rs) == 0 {
		return nil
	}
	to = min(max(to, 0), len(rs)-1)
	if to != t.sel {
		t.sel = to
		m.pane.scroll = 0
	}
	return m.ensure()
}

// openRow opens the action menu of a pull request, or folds or unfolds a
// group.
func (m *Model) openRow(r row) tea.Cmd {
	if r.kind == rowGroup {
		m.tab().folded[r.group] = !m.tab().folded[r.group]
		return nil
	}
	return m.openMenu()
}

func (m *Model) switchTab(i int) tea.Cmd {
	m.cur = (i + len(m.tabs)) % len(m.tabs)
	m.tabFrom, _ = m.shownTabs()
	m.pane.scroll = 0
	// A tab not read for longer than the refresh interval is read again
	// when it is opened.
	if at, ok := m.refreshed[m.cur]; ok && m.set.Refresh > 0 && m.set.Now().Sub(at) > m.set.Refresh && !m.tab().loading {
		m.tab().keep = model.PRKey{}
		if r, ok := m.current(); ok && r.kind == rowPR {
			m.tab().keep = r.pr.Key
		}
		return tea.Batch(m.ensure(), m.loadTab(m.cur))
	}
	return m.ensure()
}

func (m *Model) filterKey(msg tea.KeyPressMsg) tea.Cmd {
	k := msg.String()
	switch {
	case k == keymap.Back:
		m.filtering, m.filter = false, ""
	case m.keys.Matches("accept", k):
		m.filtering = false
	case m.keys.Matches("erase", k):
		if r := []rune(m.filter); len(r) > 0 {
			m.filter = string(r[:len(r)-1])
		}
	case m.keys.Matches("erase_word", k):
		m.filter = eraseWord(m.filter)
	case msg.Text != "":
		m.filter += msg.Text
	}
	m.tab().sel, m.tab().offset = 0, 0
	return m.ensure()
}

// eraseWord drops the last word of s and the spaces after it, as ctrl+w
// does in a shell.
func eraseWord(s string) string {
	s = strings.TrimRightFunc(s, unicode.IsSpace)
	return s[:strings.LastIndexFunc(s, unicode.IsSpace)+1]
}

// hide puts a pull request away: in a section it is hidden here only; in
// the inbox its notification is marked done on GitHub.
func (m *Model) hide(p model.PR, n *model.Notification) tea.Cmd {
	t := m.tab()
	t.hidden[p.Key] = true
	if !t.inbox || n == nil {
		m.setStatus(fmt.Sprintf("hidden here: %s (the request on GitHub stays)", p.Key), false)
		return m.ensure()
	}
	delete(m.unread, p.Key)
	note := *n
	m.setStatus(fmt.Sprintf("marked done: %s", p.Key), false)
	return tea.Batch(m.ensure(), func() tea.Msg { return doneMsg{note, m.src.MarkDone(m.ctx, note)} })
}

func (m *Model) browse(p model.PR) tea.Cmd { return m.browseURL(p.URL) }

// browseURL opens a page in the browser.
func (m *Model) browseURL(url string) tea.Cmd {
	if url == "" {
		return nil
	}
	return func() tea.Msg { return browseMsg{m.src.Browse(url)} }
}

// layout is how the screen's lines are shared out.
type layout struct {
	list, pane int
}

func (m *Model) layout() layout {
	// header, column header, status line, key line
	rest := m.height - 4
	if m.filtering || m.filter != "" {
		rest--
	}
	pane := 0
	if m.pane.on && m.pane.height > 0 {
		pane = m.pane.height + 2
		// The list keeps at least a third of the lines; the pane shrinks
		// first, and goes when it would show fewer than six.
		if rest-pane < rest/3 {
			pane = rest - rest/3
		}
		if pane-2 < 6 {
			pane = 0
		}
	}
	return layout{list: max(rest-pane, 1), pane: pane}
}

func (m *Model) listHeight() int { return m.layout().list }

// column is a column of the list.
type column struct {
	id    string
	title string
	width int // 0 takes what is left
	right bool
}

// columnDefs are every column, in the default order.
var columnDefs = []column{
	{id: "pr", title: "    PR", width: 28}, {id: "title", title: "TITLE"}, {id: "author", title: "AUTHOR", width: 14},
	{id: "ci", title: "CI", width: 2}, {id: "review", title: "REVIEW", width: 7}, {id: "why", title: "WHY", width: 10},
	{id: "size", title: "SIZE", width: 11}, {id: "age", title: "AGE", width: 4, right: true},
}

// dropFirst are the columns left out on a narrow terminal, first to last,
// while TITLE would be narrower than minTitle; the others are never left
// out (ADR-017). minTitle keeps SIZE down to 120 columns with all the
// columns on a grouped tab.
var dropFirst = []string{"size", "author"}

const minTitle = 27

func (m *Model) columns(t *tab) []column {
	w := m.width
	ids := m.set.Columns
	if ids == nil {
		for _, c := range columnDefs {
			ids = append(ids, c.id)
		}
	}
	var cols []column
	for _, id := range ids {
		switch {
		case id == "author" && m.allMine(t):
		case id == "why" && !t.grouped && !t.inbox:
		default:
			if i := slices.IndexFunc(columnDefs, func(c column) bool { return c.id == id }); i >= 0 {
				cols = append(cols, columnDefs[i])
			}
		}
	}
	// TITLE takes what the others leave.
	titleWidth := func() int {
		fixed := 0
		for _, c := range cols {
			fixed += c.width + 2
		}
		return w - fixed - 1
	}
	for _, id := range dropFirst {
		if titleWidth() >= minTitle {
			break
		}
		cols = slices.DeleteFunc(cols, func(c column) bool { return c.id == id })
	}
	tw := max(titleWidth(), 10)
	for i := range cols {
		if cols[i].id == "title" {
			cols[i].width = tw
		}
	}
	return cols
}

// allMine reports whether every pull request of t is the viewer's, so the
// author column says nothing.
func (m *Model) allMine(t *tab) bool {
	if m.viewer == "" || t.inbox || len(t.prs) == 0 {
		return false
	}
	return !slices.ContainsFunc(t.prs, func(p model.PR) bool { return p.Author != m.viewer })
}

func (m *Model) renderList(b *strings.Builder, height int) {
	t := m.tab()
	cols := m.columns(t)
	var hdr []string
	for _, c := range cols {
		s := fit(c.title, c.width)
		if c.right {
			s = spaces(c.width-lipgloss.Width(c.title)) + c.title
		}
		hdr = append(hdr, m.st.colHeader.Render(s))
	}
	b.WriteString(strings.Join(hdr, "  ") + "\n")

	rs := m.rows(t)
	if len(rs) > 0 {
		t.sel = min(max(t.sel, 0), len(rs)-1)
	}
	if t.sel < t.offset {
		t.offset = t.sel
	}
	if t.sel >= t.offset+height {
		t.offset = t.sel - height + 1
	}
	lines := 0
	switch {
	case !t.loaded && t.loading:
		// No group heads yet: their counts are not known, and a 0 there
		// looks like the list stopped.
		b.WriteString("  " + m.st.mauve.Render(m.spinner()) + " " + m.st.muted.Render("reading "+t.name+"… GitHub's search takes a few seconds") + "\n")
		for lines = 1; lines < height; lines++ {
			b.WriteString("\n")
		}
		return
	case t.err != nil:
		b.WriteString(m.st.red.Render("  "+t.err.Error()) + "\n")
		lines++
	case len(rs) == 0:
		b.WriteString(m.st.muted.Render("  nothing here") + "\n")
		lines++
	}
	for i := t.offset; i < len(rs) && lines < height; i++ {
		b.WriteString(m.renderRow(rs[i], cols, i == t.sel) + "\n")
		lines++
	}
	if t.inbox && !t.showSettled && lines < height {
		if _, closed := m.inboxRows(t, m.filter); closed > 0 && len(rs) < height {
			noun := "notifications"
			if closed == 1 {
				noun = "notification"
			}
			b.WriteString(m.st.dim.Render(fmt.Sprintf("  %d %s on merged or closed pull requests hidden · %s to show",
				closed, noun, m.firstKey("show_settled"))) + "\n")
			lines++
		}
	}
	for ; lines < height; lines++ {
		b.WriteString("\n")
	}
}

func (m *Model) renderRow(r row, cols []column, sel bool) string {
	if r.kind == rowGroup {
		cursor := " "
		if sel {
			cursor = m.st.blue.Render("▎")
		}
		arrow := "▾"
		if m.tab().folded[r.group] {
			arrow = "▸"
		}
		line := cursor + " " + arrow + " " + m.st.group.Render(groupLabel[r.group]) + " " + m.st.dim.Render(fmt.Sprint(r.count))
		if sel {
			return onBackground(line, m.st.sel, m.width)
		}
		return line
	}
	var cells []string
	prog, running := m.progress[r.pr.Key]
	from, to, steal := progressOver(cols)
	for i := 0; i < len(cols); i++ {
		c := cols[i]
		if running && i == from {
			// The progress takes the cells from..to, and steal more from the
			// end of TITLE, so no column moves.
			w := steal
			for _, pc := range cols[from:to] {
				w += pc.width
			}
			w += 2 * max(to-from-1, 0)
			cells = append(cells, fit(m.progressText(prog), w))
			i = to - 1
			continue
		}
		s := m.cell(c.id, r, sel)
		width := c.width
		if running && c.id == "title" {
			width -= steal
		}
		if c.right {
			s = spaces(width-lipgloss.Width(s)) + s
		}
		cells = append(cells, fit(s, width))
	}
	if running && from == len(cols) {
		// No column right of TITLE: the progress is all taken from it.
		cells = append(cells, fit(m.progressText(prog), steal-2))
	}
	line := strings.Join(cells, "  ")
	switch by := m.selected[r.pr.Key]; {
	case sel && by == byHand:
		return onBackground(line, m.st.pickedSel, m.width)
	case by == byHand:
		return onBackground(line, m.st.picked, m.width)
	case sel && by == byClaude:
		return onBackground(line, m.st.claudeSel, m.width)
	case by == byClaude:
		return onBackground(line, m.st.claude, m.width)
	case sel:
		return onBackground(line, m.st.sel, m.width)
	case m.fresh[r.pr.Key].After(m.set.Now()):
		return onBackground(line, m.st.fresh, m.width)
	}
	return line
}

// progressWidth is the least room a row's progress is written in: as
// wide as WHY and SIZE.
const progressWidth = 23

// progressOver is where a row's progress is written: over cols[from:to],
// with steal columns more taken from the end of TITLE when they leave too
// little room. It is WHY and SIZE, or REVIEW and SIZE where there is no
// WHY, when the columns have them next to each other, or WHY (REVIEW)
// alone when there is no SIZE, as on a narrow terminal; otherwise every
// column right of TITLE, so that a row shows its result whatever the
// columns are (ADR-007).
func progressOver(cols []column) (from, to, steal int) {
	at := func(id string) int { return slices.IndexFunc(cols, func(c column) bool { return c.id == id }) }
	first := "review"
	if at("why") >= 0 {
		first = "why"
	}
	if i := at(first); i >= 0 {
		switch j := at("size"); {
		case j == i+1:
			return i, j + 1, 0
		case j < 0:
			return i, i + 1, 0
		}
	}
	from, to = at("title")+1, len(cols)
	// The room is the cells' widths and the gaps between them; with no cell
	// at all, the gap before the progress is taken from TITLE too.
	room := -2
	for _, c := range cols[from:to] {
		room += c.width + 2
	}
	if room < progressWidth {
		// Never all of TITLE.
		steal = min(progressWidth-room, cols[from-1].width-1)
	}
	return from, to, steal
}

var groupLabel = map[plan.Group]string{
	plan.GroupYou:   "requested from you",
	plan.GroupTeam:  "requested from a team",
	plan.GroupOther: "other",
	plan.GroupBots:  "bots",
}

// groupCount is how the header counts each group.
var groupCount = map[plan.Group]string{
	plan.GroupYou:   "to you",
	plan.GroupTeam:  "to a team",
	plan.GroupOther: "other",
	plan.GroupBots:  "bots",
}

// repoName is a pull request's repository as the PR column shows it:
// without the owner for the home owners.
func (m *Model) repoName(p model.PR) string {
	if slices.Contains(m.set.HomeOwners, p.Key.Owner) {
		return p.Key.Repo
	}
	return p.Key.Repository()
}

func (m *Model) cell(id string, r row, sel bool) string {
	p, st := r.pr, m.st
	switch id {
	case "pr":
		cursor, mark := " ", " "
		if sel {
			cursor = st.blue.Render("▎")
		}
		switch {
		case m.selected[p.Key] == byHand:
			mark = st.mauve.Render("✓")
		case m.selected[p.Key] == byClaude:
			mark = st.teal.Render("✓")
		case m.chats[p.Key] != nil && m.chats[p.Key].unseen:
			mark = st.mauve.Render("✦")
		case func() bool { _, ok := m.unread[p.Key]; return ok }():
			mark = st.blue.Render("●")
		}
		repo := m.repoName(p)
		name, num := st.sub1, st.muted
		if p.Draft {
			name, num = st.dim, st.dim
		}
		return cursor + " " + mark + " " + name.Render(repo) + num.Render(fmt.Sprintf("#%d", p.Key.Number))
	case "title":
		if m.merged[p.Key] {
			return st.dim.Render(p.Title)
		}
		switch {
		case p.State == model.StateMerged || p.State == model.StateClosed:
			return st.dim.Render(p.Title) + " " + st.dim.Render(strings.ToLower(string(p.State)))
		case p.Draft:
			return st.dim.Render(p.Title)
		}
		return st.text.Render(p.Title)
	case "author":
		return m.author(p).text
	case "ci", "review", "size":
		if !p.HeavyRead() {
			return st.dim.Render(m.spinner())
		}
	}
	switch id {
	case "ci":
		if mi, ok := m.cache.info[p.Key]; ok && (p.Checks == model.CheckPending || p.Checks == model.CheckExpected) &&
			plan.AwaitingApproval(mi.Checks, m.set.Plan.ApprovalChecks) {
			return st.muted.Render("◌")
		}
		return m.ciMark(p.Checks)
	case "review":
		s := m.decisionMark(p.Decision)
		if r, ok := p.ReviewBy(m.viewer); ok && m.viewer != "" {
			s += " " + st.me.Render("me"+reviewMark[r.State])
		}
		return s
	case "why":
		s := m.why(r)
		switch {
		case r.notif != nil:
			return st.sub0.Render(s)
		case s == "you":
			return st.me.Render(s)
		}
		return st.muted.Render(s)
	case "size":
		if p.Additions == 0 && p.Deletions == 0 && p.ChangedFiles == 0 {
			return ""
		}
		return st.green.Render(fmt.Sprintf("+%d", p.Additions)) + " " + st.red.Render(fmt.Sprintf("−%d", p.Deletions))
	case "age":
		if p.Updated.IsZero() {
			return ""
		}
		age := m.set.Now().Sub(p.Updated)
		if m.set.StaleDays > 0 && age > time.Duration(m.set.StaleDays)*24*time.Hour {
			return st.yellow.Render(shortAge(age))
		}
		return st.muted.Render(shortAge(age))
	}
	return ""
}

var reviewMark = map[model.ReviewState]string{
	model.ReviewApproved:         "✓",
	model.ReviewChangesRequested: "±",
	model.ReviewCommented:        "…",
}

func (m *Model) ciMark(c model.CheckState) string {
	switch c {
	case model.CheckSuccess:
		return m.st.green.Render("✓")
	case model.CheckFailure, model.CheckError:
		return m.st.red.Render("✗")
	case model.CheckPending, model.CheckExpected:
		return m.st.yellow.Render("●")
	}
	return m.st.dim.Render("·")
}

func (m *Model) decisionMark(d model.ReviewDecision) string {
	switch d {
	case model.DecisionApproved:
		return m.st.green.Render("✓")
	case model.DecisionChangesRequested:
		return m.st.red.Render("±")
	case model.DecisionReviewRequired:
		return m.st.muted.Render("○")
	}
	return m.st.dim.Render("·")
}

// shortAge writes a duration as 5m, 3h, 2d, 4mo or 1y.
func shortAge(d time.Duration) string {
	switch {
	case d < time.Hour:
		return fmt.Sprintf("%dm", max(1, int(d.Minutes())))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	case d < 30*24*time.Hour:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	case d < 365*24*time.Hour:
		return fmt.Sprintf("%dmo", int(d.Hours()/24/30))
	}
	return fmt.Sprintf("%dy", int(d.Hours()/24/365))
}

// firstKey is an action's first key, for a hint.
func (m *Model) firstKey(a keymap.Action) string {
	if ks := m.keys.Keys(a); len(ks) > 0 {
		return ks[0]
	}
	return "(unbound)"
}

// why is why a row's pull request is in its list, as the WHY column says:
// the reason of its notification, you, or the teams asked.
func (m *Model) why(r row) string {
	if r.notif != nil {
		return strings.TrimSuffix(r.notif.Reason, "_requested")
	}
	if r.pr.RequestedFrom(m.viewer) {
		return "you"
	}
	var teams []string
	for _, rq := range r.pr.Requested {
		if rq.Team {
			teams = append(teams, "@"+rq.Login)
		}
	}
	if len(teams) == 0 {
		return ""
	}
	s := teams[0]
	if len(teams) > 1 {
		s += fmt.Sprintf(" +%d", len(teams)-1)
	}
	return s
}

// tabKey is the tab, counted from 0, that the key matched by is goes to
// (tab_1 to tab_9), or -1.
func tabKey(is func(keymap.Action) bool) int {
	for i := range 9 {
		if is(keymap.Action(fmt.Sprintf("tab_%d", i+1))) {
			return i
		}
	}
	return -1
}

// hasTabFor reports whether a is no key to a tab, or one to a tab there
// is, so the ? list leaves out the numbers of tabs not in the config.
func (m *Model) hasTabFor(a keymap.Action) bool {
	var n int
	if _, err := fmt.Sscanf(string(a), "tab_%d", &n); err != nil {
		return true
	}
	return n <= len(m.tabs)
}
