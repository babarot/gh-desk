package tui

import (
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/babarot/gh-desk/internal/model"
	"github.com/babarot/gh-desk/internal/plan"
	"github.com/babarot/gh-desk/internal/runner"
	"github.com/babarot/gh-desk/internal/tui/keymap"
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
		case m.filter != "" || t.ruleFilter != nil:
			m.filter, t.ruleFilter = "", nil
		default:
			m.say(m.nothingToClear())
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
		return tea.Batch(m.refresh(), m.takeLock())
	case is("events"):
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
	case is("triage"):
		return m.openTriage()
	case is("approve") && len(m.selected) > 0:
		return m.startApprove()
	case is("merge"):
		return m.startBulk(runner.Merge)
	case is("ready_for_review"):
		return m.startBulk(runner.Ready)
	case is("close"):
		return m.startClose()
	case is("comment"):
		return m.startComment()
	case is("review") && len(m.selected) > 0:
		return m.startReview()
	case is("ask_claude") && len(m.selected) > 0:
		m.refuse("ask claude: one pull request at a time")
		return nil
	case is("hide") && len(m.selected) > 0:
		return m.hideSelected()
	case is("browse") && len(m.selected) > 0:
		return m.browseAll(m.menuTargets())
	case is("fold_bots"):
		if t.grouped {
			t.folded[plan.GroupBots] = !t.folded[plan.GroupBots]
		}
	case is("show_excluded"):
		back := m.holdCursor(t)
		t.showExcluded = !t.showExcluded
		back()
	case is("log"):
		return m.openLog()
	case is("preview"):
		m.pane.on = !m.pane.on
	case is("preview_grow"):
		return m.resizePane(m.pane.height + 1)
	case is("preview_shrink"):
		return m.resizePane(m.pane.height - 1)
	case is("preview_tab"):
		m.pane.toggleMode()
		return m.ensure()
	case is("preview_down"):
		m.pane.scroll += 3
	case is("preview_up"):
		m.pane.scroll = max(0, m.pane.scroll-3)
	case is("next_file"):
		m.jumpFile(1)
	case is("prev_file"):
		m.jumpFile(-1)
	case is("fold"):
		m.foldPart()
	}
	r, ok := m.current()
	if !ok {
		return nil
	}
	if is("menu") {
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
			now := m.set.Now()
			for _, p := range t.prs {
				if _, ok := m.shows(t, p, nil, m.filter, t.ruleFilter, now); ok && plan.GroupOf(p, m.viewer) == r.group {
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
		return m.startApprove()
	case is("review"):
		return m.startReview()
	case is("ask_claude"):
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
	if !m.keys.Matches("complete", k) {
		m.cycle = filterCycle{}
	}
	switch {
	case k == keymap.Back:
		m.filtering, m.filter = false, ""
	case m.keys.Matches("accept", k):
		m.filtering = false
	case m.keys.Matches("complete", k):
		m.complete()
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

// nothingToClear tells why esc did nothing at the top of the list, and
// how to quit instead.
func (m *Model) nothingToClear() string {
	quit := keymap.Interrupt
	if ks := m.keys.Keys("quit"); len(ks) > 0 {
		quit = ks[0]
	}
	return fmt.Sprintf("esc: nothing to clear; %s quits", quit)
}

// eraseWord drops the last word of s and the spaces after it, as ctrl+w
// does in a shell.
func eraseWord(s string) string {
	s = strings.TrimRightFunc(s, unicode.IsSpace)
	return s[:strings.LastIndexFunc(s, unicode.IsSpace)+1]
}

// hide puts a pull request away: in a section it is hidden here only; in
// the inbox its notification is marked done on GitHub. It leaves the
// selection, so what is out of sight is not acted on.
func (m *Model) hide(p model.PR, n *model.Notification) tea.Cmd {
	cmd, done := m.putAway(p.Key, n)
	if done {
		m.say(fmt.Sprintf("marked done: %s", p.Key))
	} else {
		m.say(fmt.Sprintf("hidden here: %s (the request on GitHub stays)", p.Key))
	}
	return tea.Batch(m.ensure(), cmd)
}

// putAway hides k in the tab shown and takes it out of the selection; in
// the inbox, with its notification n, it also marks n done, and done says
// so. One selected that fails to be marked done is selected again.
func (m *Model) putAway(k model.PRKey, n *model.Notification) (cmd tea.Cmd, done bool) {
	t := m.tab()
	t.hidden[k] = true
	was := m.isSelected(k)
	delete(m.selected, k)
	if !t.inbox || n == nil {
		return nil, false
	}
	delete(m.unread, k)
	note := *n
	return func() tea.Msg { return doneMsg{note, m.src.MarkDone(m.ctx, note), was} }, true
}

// selectedHere are the selected pull requests the tab shown shows, each
// with its notification in the inbox: through the filter and the rules,
// and in the inbox not left out as settled. A folded group counts as
// shown, since its header is. What is selected and not shown is never put
// away unseen.
func (m *Model) selectedHere() ([]model.PRKey, []*model.Notification) {
	t := m.tab()
	var ks []model.PRKey
	var ns []*model.Notification
	for _, r := range m.rowsOf(t, m.filter, t.ruleFilter, nil) {
		if r.kind == rowPR && m.isSelected(r.pr.Key) && !slices.Contains(ks, r.pr.Key) {
			ks, ns = append(ks, r.pr.Key), append(ns, r.notif)
		}
	}
	return ks, ns
}

// hideSelected puts away the selected pull requests the tab shown shows,
// as hide does one; the others stay selected.
func (m *Model) hideSelected() tea.Cmd {
	ks, ns := m.selectedHere()
	if len(ks) == 0 {
		m.refuse("hide: none of the selection is shown here")
		return nil
	}
	cmds := []tea.Cmd{nil}
	for i, k := range ks {
		cmd, _ := m.putAway(k, ns[i])
		cmds = append(cmds, cmd)
	}
	what := fmt.Sprintf("hidden here: %d pull requests (the requests on GitHub stay)", len(ks))
	if m.tab().inbox {
		what = fmt.Sprintf("marked done: %d pull requests", len(ks))
	}
	if left := len(m.selected); left > 0 {
		what += fmt.Sprintf("; %d selected not shown here stay selected", left)
	}
	m.say(what)
	cmds[0] = m.ensure()
	return tea.Batch(cmds...)
}

// browseAll opens each of prs in the browser.
func (m *Model) browseAll(prs []model.PR) tea.Cmd {
	var cmds []tea.Cmd
	for _, p := range prs {
		cmds = append(cmds, m.browse(p))
	}
	return tea.Batch(cmds...)
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

func (m *Model) layout() layout { return m.layoutFor(m.pane.on || m.logInPane()) }

// layoutFor is the layout with the bottom pane shown or not; the log
// viewer takes its place when the screen is narrow, whether the preview
// is on or off.
func (m *Model) layoutFor(on bool) layout {
	// header, column header, status line, key line
	rest := m.height - 4
	if m.filterLine() {
		rest--
	}
	pane := 0
	if on && m.pane.height > 0 {
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

// listHeight is how many lines the rows of the tab shown take.
func (m *Model) listHeight() int { return m.rowLines(m.tab(), m.layout().list) }

// rowLines are the lines of a list of height lines that t's rows take:
// while the tab leaves some out, its last line says how many, whatever
// the rows are (D30).
func (m *Model) rowLines(t *tab, height int) int {
	if height > 1 && !t.showExcluded && m.excludedOf(t).n > 0 {
		return height - 1
	}
	return height
}

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
	rowsH := m.rowLines(t, height)
	if t.sel < t.offset {
		t.offset = t.sel
	}
	if t.sel >= t.offset+rowsH {
		t.offset = t.sel - rowsH + 1
	}
	lines := 0
	switch {
	case !t.loaded && t.loading:
		// No group heads yet: their counts are not known, and a 0 there
		// looks like the list stopped.
		b.WriteString("  " + m.st.muted.Render("reading "+t.name+"… GitHub's search takes a few seconds") + "\n")
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
	for i := t.offset; i < len(rs) && lines < rowsH; i++ {
		b.WriteString(m.renderRow(rs[i], cols, i == t.sel, i) + "\n")
		lines++
	}
	if rowsH < height {
		// The line under the rows, the last one when they fill the list,
		// says how many the tab leaves out.
		b.WriteString(m.st.dim.Render("  "+excludedLine(m.excludedOf(t))+" · "+m.firstKey("show_excluded")+" to show") + "\n")
		lines++
	}
	for ; lines < height; lines++ {
		b.WriteString("\n")
	}
}

// answering reports whether Claude is answering here about the pull
// request k.
func (m *Model) answering(k model.PRKey) bool {
	c, ok := m.chats[k]
	return ok && c.Running()
}

// shownPR is the pull request k as a row of t would show it, when one
// would (shows). ok is false otherwise.
func (m *Model) shownPR(t *tab, k model.PRKey) (p model.PR, ok bool) {
	now := m.set.Now()
	for _, p := range t.prs {
		if p.Key != k {
			continue
		}
		if _, ok := m.shows(t, p, nil, m.filter, t.ruleFilter, now); ok {
			return p, true
		}
	}
	return model.PR{}, false
}

// excludedLine says how many c are excluded, and of them how many as
// merged or closed and how many unread.
func excludedLine(c excludedCount) string {
	noun := "pull requests"
	if c.n == 1 {
		noun = "pull request"
	}
	s := fmt.Sprintf("%d %s excluded", c.n, noun)
	var parts []string
	if c.settled > 0 {
		parts = append(parts, fmt.Sprintf("%d %s", c.settled, settledName))
	}
	if c.unread > 0 {
		parts = append(parts, fmt.Sprintf("%d unread", c.unread))
	}
	if len(parts) > 0 {
		s += " (" + strings.Join(parts, ", ") + ")"
	}
	return s
}

// claudeIn reports whether Claude is answering about a pull request in
// group g of the tab shown, and whether an answer came in it unseen.
func (m *Model) claudeIn(g plan.Group) (answering, unseen bool) {
	t := m.tab()
	for k, c := range m.chats {
		if !c.Running() && !c.unseen {
			continue
		}
		if p, ok := m.shownPR(t, k); ok && plan.GroupOf(p, m.viewer) == g {
			answering = answering || c.Running()
			unseen = unseen || !c.Running() && c.unseen
		}
	}
	return answering, unseen
}

// onScreen reports whether the pull request k is on the screen: its row
// in the part of the list renderList draws, or the head of its folded
// group there.
func (m *Model) onScreen(k model.PRKey) bool {
	t := m.tab()
	if !t.loaded && t.loading {
		return false
	}
	h := m.rowLines(t, m.layout().list)
	if t.err != nil {
		// renderList gives the error a line before the rows.
		h--
	}
	rs := m.rows(t)
	for i := t.offset; i < len(rs) && i < t.offset+h; i++ {
		switch r := rs[i]; {
		case r.kind == rowPR && r.pr.Key == k:
			return true
		case r.kind == rowGroup && t.folded[r.group]:
			if p, ok := m.shownPR(t, k); ok && plan.GroupOf(p, m.viewer) == r.group {
				return true
			}
		}
	}
	return false
}

// renderRow is r drawn on line n of the list.
func (m *Model) renderRow(r row, cols []column, sel bool, n int) string {
	if r.kind == rowGroup {
		cursor := " "
		if sel {
			cursor = m.st.blue.Render("▎")
		}
		arrow := "▾"
		if m.tab().folded[r.group] {
			arrow = "▸"
		}
		label := m.st.group.Render(groupLabel[r.group])
		var tail string
		if m.tab().folded[r.group] {
			// A folded group tells what its rows would.
			switch answering, unseen := m.claudeIn(r.group); {
			case answering:
				if m.set.Shimmer {
					label = m.shimmer(m.st.group, groupLabel[r.group])
				}
				tail = " " + m.st.mauve.Render(m.claudeFrame())
			case unseen:
				tail = " " + m.st.mauve.Render("✦")
			}
		}
		line := cursor + " " + arrow + " " + label + " " + m.st.dim.Render(fmt.Sprint(r.count)) + tail
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
		s := m.cell(c.id, r, sel, n)
		if r.ex.excluded() && c.id != "pr" {
			// A row the exclusion leaves out, shown with a, is drawn dim
			// past its cursor and mark.
			s = m.st.dim.Render(ansi.Strip(s))
		}
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
	if bg, ok := m.rowBackground(r.pr.Key, sel); ok {
		return onBackground(line, bg, m.width)
	}
	return line
}

// rowBackground is the background a pull request's row is drawn on;
// false when it has none.
func (m *Model) rowBackground(k model.PRKey, sel bool) (lipgloss.Style, bool) {
	switch by := m.selected[k]; {
	case sel && by == byHand:
		return m.st.pickedSel, true
	case by == byHand:
		return m.st.picked, true
	case sel && by == byClaude:
		return m.st.claudeSel, true
	case by == byClaude:
		return m.st.claude, true
	case sel:
		return m.st.sel, true
	case m.fresh[k].After(m.set.Now()):
		return m.st.fresh, true
	}
	return lipgloss.Style{}, false
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

func (m *Model) cell(id string, r row, sel bool, n int) string {
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
		case m.answering(p.Key):
			mark = st.mauve.Render(m.claudeFrame())
		case m.chats[p.Key] != nil && m.chats[p.Key].unseen:
			mark = st.mauve.Render("✦")
		case r.ex.excluded():
			// Shown with a, as a row the exclusion leaves out.
			mark = st.dim.Render("⊘")
		case r.ex.pending && !m.set.Shimmer:
			// An exclusion rule cannot tell yet; with shimmer, the light
			// over the pull request says so instead.
			mark = st.yellow.Render("?")
		case func() bool { _, ok := m.unread[p.Key]; return ok }():
			mark = st.blue.Render("●")
		}
		repo := m.repoName(p)
		name, num := st.sub1, st.muted
		if p.Draft || r.ex.excluded() {
			name, num = st.dim, st.dim
		}
		number := fmt.Sprintf("#%d", p.Key.Number)
		// A row an exclusion rule cannot tell yet may leave once what the
		// rule waits for is read.
		if (m.answering(p.Key) || r.ex.pending) && m.set.Shimmer {
			return cursor + " " + mark + " " + m.shimmerParts(styled{repo, name}, styled{number, num})
		}
		return cursor + " " + mark + " " + name.Render(repo) + num.Render(number)
	case "title":
		if m.merged[p.Key] {
			return st.dim.Render(shownTitle(p.Title))
		}
		switch {
		case p.State == model.StateMerged || p.State == model.StateClosed:
			return st.dim.Render(shownTitle(p.Title)) + " " + st.dim.Render(strings.ToLower(string(p.State)))
		case p.Draft:
			return st.dim.Render(shownTitle(p.Title))
		}
		return st.text.Render(shownTitle(p.Title))
	case "author":
		return m.author(p).text
	case "ci", "review", "size":
		if !p.HeavyRead() {
			// Nothing in a cell moves: the status line tells how far the
			// reading is (ADR-050). ⋅, not ·, which is East Asian
			// Ambiguous and would move the columns after it.
			return st.dim.Render("⋅")
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
