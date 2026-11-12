package tui

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/babarot/gh-desk/internal/claude"
	"github.com/babarot/gh-desk/internal/tui/keymap"
)

// View draws the screen.
func (m *Model) View() tea.View {
	s, cur := m.render()
	v := tea.NewView(s)
	// The terminal's own cursor, not one drawn: an input method shows the
	// text being composed where it is.
	v.Cursor = cur
	v.AltScreen = true
	// Clicks, the wheel, and dragging the pane's edge; see mouse.go.
	v.MouseMode = tea.MouseModeCellMotion
	return v
}

// render draws the screen and says where the cursor of the input being
// typed in is, nil when nothing is.
func (m *Model) render() (string, *tea.Cursor) {
	if m.width == 0 || m.height == 0 {
		return "", nil
	}
	var b strings.Builder
	var cur *tea.Cursor
	b.WriteString(m.renderHeader() + "\n")
	if m.filterLine() {
		if m.filtering {
			cur = tea.NewCursor(min(lipgloss.Width("/ "+m.filter), max(m.width-1, 0)), 1)
		}
		// Cut to the width, as a line that wraps moves every row under it.
		b.WriteString(fit(m.renderFilter(), m.width) + "\n")
	}
	l := m.layout()
	m.renderList(&b, l.list)
	switch {
	case l.pane > 0 && m.logInPane():
		for _, s := range m.renderViewer(m.width, l.pane) {
			b.WriteString(s + "\n")
		}
	case l.pane > 0 && m.claudeOpen:
		if c := m.renderClaude(&b, l.pane); c != nil {
			cur = c
		}
	case l.pane > 0:
		m.renderPane(&b, l.pane)
	}
	b.WriteString(fit(m.statusText(), m.width) + "\n")
	b.WriteString(fit(m.hints(), m.width))
	lines := m.drawArmed(strings.Split(b.String(), "\n"))
	if m.logv != nil && !m.logInPane() {
		// Over the list's right side, from under the header to above the
		// status line; what is under it is not dimmed, as it is no popup.
		w, _ := m.logWidth()
		for i, s := range m.renderViewer(w, len(lines)-3) {
			lines[1+i] = place(lines[1+i], s, m.width-w)
		}
	}
	if m.compose != nil && m.bulk == nil {
		// The confirm popup of what was written takes its place, in a box
		// as wide, and esc there gives the text box back: no box is drawn
		// over another (ADR-097).
		popup, at := m.renderCompose()
		lines = m.overlay(lines, popup, 2, m.width)
		cur = inPopup(m.compose.ta.Cursor(), popup, 2+at, m.width)
	}
	switch {
	case m.bulk != nil:
		cur = nil
		popup := m.renderBulk()
		m.popupAt = boxAt(popup, 2, m.width)
		lines = m.overlay(lines, popup, 2, m.width)
		if f := m.bulk.close; f != nil && f.editing {
			cur = inPopup(f.ta.Cursor(), popup, 2+f.at, m.width)
		}
	case m.notesAt != nil:
		cur = nil
		popup := m.renderNotes()
		m.popupAt = boxAt(popup, 2, m.width)
		lines = m.overlay(lines, popup, 2, m.width)
	case m.picker != nil:
		cur = nil
		popup := m.renderPick()
		m.popupAt = boxAt(popup, 2, m.width)
		lines = m.overlay(lines, popup, 2, m.width)
	case m.chooser != nil:
		popup, at := m.renderChooser()
		m.popupAt = boxAt(popup, 2, m.width)
		lines = m.overlay(lines, popup, 2, m.width)
		cur = inPopup(m.chooser.input.Cursor(), popup, 2+at, m.width)
	case m.menu != nil:
		cur = nil
		popup := m.renderMenu()
		m.popupAt = boxAt(popup, 2, m.width)
		lines = m.overlay(lines, popup, 2, m.width)
	case m.triage != nil && m.triage.open:
		popup, at := m.renderTriage()
		m.popupAt = boxAt(popup, 2, m.width)
		lines = m.overlay(lines, popup, 2, m.width)
		cur = nil
		if at >= 0 {
			in := &m.triage.input
			if s := m.triage.search; s != nil {
				in = &s.input
			}
			cur = inPopup(in.Cursor(), popup, 2+at, m.width)
		}
	case m.help:
		cur = nil
		if m.relBox != nil {
			lines = m.renderReleaseNotes(lines)
		} else {
			lines = m.renderHelp(lines)
		}
	case m.toast != nil && len(lines) > 3:
		toast := m.st.border.Render("╭ ") + m.noteIcon(m.toast.icon) + " " + m.st.sub1.Render(m.toast.title) + " " +
			m.st.dim.Render("· "+m.firstKey("events")+" to see") + m.st.border.Render(" ╮")
		at := len(lines) - 3
		lines[at] = place(lines[at], toast, max(m.width-lipgloss.Width(toast)-1, 0))
	}
	if len(lines) > m.height {
		lines = lines[:m.height]
	}
	if cur != nil && cur.Y >= len(lines) {
		cur = nil
	}
	return strings.Join(lines, "\n"), cur
}

// inPopup moves the cursor of an input drawn in a box from row at of the
// screen, inside the box's left edge, to where it is on the screen.
func inPopup(c *tea.Cursor, popup []string, at, width int) *tea.Cursor {
	if c == nil || len(popup) == 0 {
		return nil
	}
	c.X += max((width-lipgloss.Width(popup[0]))/2, 0) + lipgloss.Width("│ ")
	c.Y += at
	return c
}

// overlay puts a popup over the screen's lines, the screen dimmed under
// it: drawn in one faint color of the theme, so only the popup keeps its
// colors and stands out, as a dialog over a darkened page does.
func (m *Model) overlay(lines, popup []string, top, width int) []string {
	under := make([]string, len(lines))
	for i, l := range lines {
		under[i] = m.st.dim.Render(ansi.Strip(l))
	}
	return overlay(under, popup, top, width)
}

// overlay puts popup over lines from row top, in the middle, keeping what
// is left and right of it.
func overlay(lines, popup []string, top, width int) []string {
	out := append([]string(nil), lines...)
	left := 0
	if len(popup) > 0 {
		left = max((width-lipgloss.Width(popup[0]))/2, 0)
	}
	for i, p := range popup {
		if top+i < len(out) {
			out[top+i] = place(out[top+i], p, left)
		}
	}
	return out
}

// place draws over on top of base from cell x, keeping what is left and
// right of it. A wide character cut by an edge of over leaves a gap.
func place(base, over string, x int) string {
	left := ansi.Truncate(base, x, "")
	left += spaces(x - ansi.StringWidth(left))
	end := x + ansi.StringWidth(over)
	right := ansi.TruncateLeft(base, end, "")
	// A wide character across over's right edge comes back whole, which
	// would push the rest of the line a cell right; drop it.
	if ansi.StringWidth(right) > ansi.StringWidth(base)-end {
		right = ansi.TruncateLeft(base, end+1, "")
	}
	right = spaces(ansi.StringWidth(base)-end-ansi.StringWidth(right)) + right
	return left + "\x1b[m" + over + "\x1b[m" + right
}

func (m *Model) renderHeader() string {
	left := m.headerLeft()
	right := m.headerRight()
	gap := m.width - lipgloss.Width(left) - lipgloss.Width(right) - 1
	return m.st.header.Render(fit(left+spaces(gap)+right, m.width))
}

// headerLeft is the header's left: the app, the viewer and the tabs.
func (m *Model) headerLeft() string {
	from, to := m.shownTabs()
	var tabs []string
	for i := from; i < to; i++ {
		tabs = append(tabs, m.tabLabel(i))
	}
	return m.headerLead() + m.st.dim.Render(tabsBefore(from)) + strings.Join(tabs, tabGap) + m.st.dim.Render(tabsAfter(to, len(m.tabs)))
}

// shownTabs are the tabs the header has room for, from (included) to
// (left out), the shown one among them. The tabs shown move only when the
// shown one would be out of them, and take what room is left.
func (m *Model) shownTabs() (from, to int) {
	n := len(m.tabs)
	ws := make([]int, n)
	for i := range n {
		ws[i] = lipgloss.Width(m.tabLabel(i))
	}
	room := m.width - lipgloss.Width(m.headerLead()) - lipgloss.Width(m.headerRight()) - 1
	width := func(from, to int) int {
		w := lipgloss.Width(tabsBefore(from)) + lipgloss.Width(tabsAfter(to, n)) + len(tabGap)*(to-from-1)
		for _, x := range ws[from:to] {
			w += x
		}
		return w
	}
	from = min(m.tabFrom, m.cur)
	for from < m.cur && width(from, m.cur+1) > room {
		from++
	}
	to = m.cur + 1
	for to < n && width(from, to+1) <= room {
		to++
	}
	for from > 0 && width(from-1, to) <= room {
		from--
	}
	return from, to
}

// tabGap is what comes between two tabs: the header's, the pane's and
// the diff's file tabs. The pills' own padding spaces them
// out.
const tabGap = " "

// pill is a tab named s, drawn on if it is the one shown and off if not,
// padded by a space on each side either way.
func pill(on, off lipgloss.Style, s string, shown bool) string {
	if shown {
		return on.Render(" " + s + " ")
	}
	return off.Render(" " + s + " ")
}

// tabsBefore and tabsAfter mark the tabs the header leaves out.
func tabsBefore(from int) string {
	if from > 0 {
		return "… "
	}
	return ""
}

func tabsAfter(to, n int) string {
	if to < n {
		return " …"
	}
	return ""
}

// headerLead is what the header shows before the tabs: the viewer, or with
// a scope set, the scope switch in its place (scopeSwitch).
func (m *Model) headerLead() string {
	if m.set.Scope != "" {
		return m.st.app.Render("desk") + m.st.dim.Render(" // ") + m.scopeSwitch() + "   "
	}
	return m.st.app.Render("desk") + m.st.dim.Render(" // ") + m.st.muted.Render(m.viewerName()) + "   "
}

// maxScopeWidth is the most cells the scope switch gives a value; a longer
// one is cut.
const maxScopeWidth = 24

// scopeSwitch is the value chosen between the marks that step to the one
// before and after it: as wide as the longest value, so that the tabs do
// not move as it changes, and in the active color while one is chosen.
func (m *Model) scopeSwitch() string {
	st := m.st
	w := m.scopeNameWidth()
	name := st.sub0.Render(fit(allScope, w))
	if m.scope != "" {
		name = st.blue.Bold(true).Render(fit(safeLine(m.scope), w))
	}
	return st.dim.Render("‹") + " " + name + " " + st.dim.Render("›")
}

// scopeNameWidth is the cells the scope switch gives the value's name.
func (m *Model) scopeNameWidth() int {
	w := lipgloss.Width(allScope)
	for _, v := range m.scopeChoices() {
		w = max(w, lipgloss.Width(safeLine(v)))
	}
	return min(w, maxScopeWidth)
}

// viewerName is the viewer as the header shows it, which a click opens.
func (m *Model) viewerName() string {
	if m.viewer == "" {
		return ""
	}
	return "@" + m.viewer
}

// tabLabel is tab i as the header shows it.
// searchCount is how many pull requests a search tab found, GitHub's
// count, less those it leaves out while it does not show them, as the
// inbox's unread count leaves out what it excludes (D31).
func (m *Model) searchCount(t *tab) int {
	if t.showExcluded {
		return t.total
	}
	return max(t.total-m.excludedOf(t).n, 0)
}

func (m *Model) tabLabel(i int) string {
	t := m.tabs[i]
	// A count read is shown while the rest of the tab is read: the status
	// line tells how far the reading is (ADR-050).
	count := "…"
	narrowed := false
	switch {
	case errors.Is(t.err, errNotRead):
		count = "-"
	case t.loaded && (t.inbox || t.err == nil) && m.narrowed(t):
		count, narrowed = fmt.Sprint(m.narrowedCount(t)), true
	case t.loaded && (t.inbox || t.err == nil) && m.scoped():
		// A scope is no narrowing typed: its count is the tab's, of the
		// value chosen, counted as the narrowed rows are.
		count = fmt.Sprint(m.narrowedCount(t))
	case t.inbox && t.loaded:
		count = fmt.Sprint(m.unreadShown(t))
	case t.loaded && t.err == nil:
		count = fmt.Sprint(m.searchCount(t))
	case t.err != nil:
		count = "!"
	}
	// The key is fainter than the name, and so is the count in parentheses
	// except on the tab shown, so neither reads as part of the name. The
	// key is inside the tab's pill, so that it reads as this tab's. Every
	// tab is padded as the pill of the one shown is, so that none moves
	// when it changes.
	st := m.st
	key := ""
	if ks := m.keys.Keys(keymap.Action(fmt.Sprintf("tab_%d", i+1))); len(ks) > 0 {
		key = " " + ks[0]
	}
	// A count of rows narrowed is in the filter line's yellow, so that it
	// reads as the filter's and not as the tab's whole.
	if i == m.cur {
		c := st.tabOn.Render("(" + count + ")")
		if narrowed {
			c = st.tabOn.Foreground(st.p.yellow).Render("(" + count + ")")
		}
		return st.tabKeyOn.Render(key) + st.tabOn.Render(" "+t.name+" ") + c + st.tabOn.Render(" ")
	}
	c := st.dim.Render("(" + count + ")")
	if narrowed {
		c = st.yellow.Render("(" + count + ")")
	}
	return st.dim.Render(key) + st.tabOff.Render(" "+t.name+" ") + c + " "
}

func (m *Model) headerRight() string {
	return m.st.sub0.Render(strings.Join(m.headerParts(), headerSep))
}

// headerSep is what sets the parts of the header's right apart.
const headerSep = " · "

// eventsAt is where the header draws the count of the events not seen,
// which a click opens: from x, w cells wide.
func (m *Model) eventsAt() (x, w int, ok bool) {
	parts := m.headerParts()
	i := slices.IndexFunc(parts, func(s string) bool { return strings.HasPrefix(ansi.Strip(s), "✦") })
	if i < 0 {
		return 0, 0, false
	}
	right := lipgloss.Width(strings.Join(parts, headerSep))
	// renderHeader leaves a cell after the right part, which it cuts
	// when it has no room.
	if lipgloss.Width(m.headerLeft())+right+1 > m.width {
		return 0, 0, false
	}
	x = m.width - 1 - right
	for _, s := range parts[:i] {
		x += lipgloss.Width(s) + len(headerSep)
	}
	return x, lipgloss.Width(parts[i]), true
}

// headerParts are the parts of the header's right.
func (m *Model) headerParts() []string {
	t, st := m.tab(), m.st
	var parts []string
	// The rows are from the cache, or a reading again failed.
	if !t.asOf.IsZero() {
		parts = append(parts, st.yellow.Render("as of "+t.asOf.Local().Format("15:04")))
	}
	if n := m.unseen(); n > 0 {
		parts = append(parts, st.mauve.Render(fmt.Sprintf("✦%d", n))+" "+st.dim.Render(m.firstKey("events")))
	}
	if s := m.tally(); s != "" {
		parts = append(parts, s)
	}
	if n := len(m.selected); n > 0 {
		parts = append(parts, st.mauve.Render(fmt.Sprintf("%d selected", n)))
	}
	// Results on the rows take the place of the tab's counts.
	if len(m.progress) > 0 {
		return parts
	}
	if t.loaded && t.total > len(t.prs) && !t.inbox {
		parts = append(parts, fmt.Sprintf("showing %d of %d", len(t.prs), t.total))
	}
	if len(t.warnings) > 0 {
		parts = append(parts, st.yellow.Render("⚠ read in part"))
	}
	return parts
}

// toldInPlace are the actions whose key is told where it works, in the
// status line or at the foot of a popup, and not in the hints or the ? list.
// Every other action is listed.
var toldInPlace = map[keymap.Action]bool{
	"retry": true, "chat_restart": true, "release_notes": true,
}

// hints is the key line of what has the focus. It holds the ways in (the
// menu of enter, and ? for every key) and the keys nothing else on the
// screen tells: the menu shows what can be done to a pull request, the
// preview pane its own keys, and the header the key of the events.
func (m *Model) hints() string {
	var as []keymap.Action
	// esc is told where it steps back; on the list it clears a selection or
	// a filter, and is not told when it would quit.
	back := "back"
	recall := ""
	switch {
	case m.bulk != nil && m.bulk.approves():
		as = []keymap.Action{"confirm", "include_failing"}
		if m.bulk.atWork() > 0 {
			as = append(as, "wait_for_checks")
		}
	case m.bulk != nil && m.bulk.close != nil && m.bulk.close.editing:
		as = nil
	case m.bulk != nil && m.bulk.close != nil:
		as = []keymap.Action{"confirm", "delete_branch", "write"}
	case m.bulk != nil:
		as = []keymap.Action{"confirm"}
	case m.picker != nil:
		as = []keymap.Action{"toggle_rule", "choose", "add_to_selection", "filter_by_rule"}
	case m.notesAt != nil, m.menu != nil, m.chooser != nil:
		as = []keymap.Action{"choose"}
	case m.compose != nil:
		as = []keymap.Action{"next_kind", "post", "editor"}
		if !m.compose.ta.Focused() {
			as = append([]keymap.Action{"write"}, as...)
		}
	case m.triage != nil && m.triage.open:
		switch {
		case m.triage.typing:
			as = []keymap.Action{"accept"}
			if tr := m.triage; nextSuggestion(claude.TriageQuestions(), tr.input.Value()) != "" || askGhost(&tr.input, &tr.recall, claude.TriageQuestions(), tr.history) != "" {
				as = append(as, "complete")
			}
			recall = m.recallHint(&m.triage.recall, m.triage.history)
			if len(m.askMenu()) > 0 {
				as, recall, back = []keymap.Action{"accept", "complete"}, "", "close"
			} else if m.triage.search != nil {
				as, recall, back = []keymap.Action{"accept", "history_search"}, "", "cancel"
			} else if len(m.triage.history) > 0 {
				as = append(as, "history_search")
			}
		case m.triage.asking:
			as = []keymap.Action{"chat_close"}
			if !m.triage.stopping {
				back = "stop"
			}
		case m.applicable():
			as = []keymap.Action{"triage_apply", "write"}
		default:
			as = []keymap.Action{"write"}
		}
	case m.filtering:
		as = []keymap.Action{"accept"}
		if m.canComplete() {
			as = append(as, "complete")
		}
	case m.typingChat():
		as = []keymap.Action{"accept"}
		if c := m.chat(); nextSuggestion(suggestions(c), c.input.Value()) != "" || askGhost(&c.input, &c.recall, suggestions(c), chatHistory(c)) != "" {
			as = append(as, "complete")
		}
		if len(chatHistory(m.chat())) > 0 {
			as = append(as, "history_search")
		}
		as = append(as, "draft_review", "open_claude")
		recall = m.recallHint(&m.chat().recall, chatHistory(m.chat()))
		if m.chat().search != nil {
			as, recall, back = []keymap.Action{"accept", "history_search"}, "", "cancel"
		}
	case m.logv != nil:
		as = []keymap.Action{"log_this_pr", "bottom", "log", "help"}
		back = "close"
	case m.claudeShown():
		// While Claude answers: esc stops it, and chat_close goes back.
		// One answering in another gh-desk is not stopped here, and
		// esc goes back.
		as = []keymap.Action{"chat_close", "help"}
		back = ""
		if c := m.chat(); c.cancel != nil && !c.stopping {
			back = "stop"
		}
	case len(m.selected) > 0:
		as = []keymap.Action{"menu", "select", "select_shown", "clear_selection", "help"}
		back = "clear"
	default:
		as = []keymap.Action{"menu", "select", "select_rule", "triage", "log"}
		if m.retry != nil {
			as = append([]keymap.Action{"retry"}, as...)
		}
		if len(m.lastSelected) > 0 {
			as = append([]keymap.Action{"clear_selection"}, as...)
		}
		if m.tab().inbox {
			as = append(as, "hide")
		}
		// a is told where it has something to show or hide.
		if t := m.tab(); t.showExcluded || m.excludedOf(t).n > 0 {
			as = append(as, "show_excluded")
		}
		as = append(as, "filter", "help")
		back = ""
		if m.filter != "" || m.tab().ruleFilter != nil {
			back = "clear"
		}
	}
	// The keys of ? and esc are kept: the hints that do not fit go from
	// before them, the last first, since ? tells every key.
	var head, tail []string
	for _, a := range as {
		ks := m.keys.Keys(a)
		if len(ks) == 0 {
			continue
		}
		part := m.st.key.Render(ks[0]) + " " + m.st.muted.Render(m.keys.Help(a))
		if a == "help" {
			tail = append(tail, part)
		} else {
			head = append(head, part)
		}
	}
	if recall != "" {
		head = slices.Insert(head, min(1, len(head)), recall)
	}
	if back != "" {
		tail = append(tail, m.st.key.Render(keymap.Back)+" "+m.st.muted.Render(back))
	}
	sep := m.st.dim.Render(" · ")
	join := func() string { return strings.Join(append(slices.Clone(head), tail...), sep) }
	for len(head) > 0 && ansi.StringWidth(join()) > m.width {
		head = head[:len(head)-1]
	}
	return join()
}

// filterLine reports whether the line under the header shows a filter: the
// one typed, or the rules the tab's rows are narrowed to.
func (m *Model) filterLine() bool {
	return m.filtering || m.filter != "" || m.tab().ruleFilter != nil
}

// renderFilter is the line of the filters: the text typed, and the rules
// the tab is narrowed to with how many rows they leave.
func (m *Model) renderFilter() string {
	st, t := m.st, m.tab()
	var parts []string
	if m.filtering || m.filter != "" {
		// No cursor is drawn: render gives the terminal's to the end of it.
		s := st.yellow.Render("/ ") + renderFilterText(m.filter, st.yellow, st.blue)
		// While it is typed, what the filter may ask follows the cursor,
		// faint.
		if m.filtering {
			s += st.dim.Render(m.filterGhost())
		}
		parts = append(parts, s)
	}
	if t.ruleFilter != nil {
		n := 0
		for _, r := range m.rowsOf(t, m.filter, t.ruleFilter, nil) {
			if r.kind == rowPR {
				n++
			}
		}
		rule := st.yellow.Render("rule: "+ruleNames(t.ruleFilter)) + st.dim.Render(fmt.Sprintf(" · %d PRs", n))
		if t.readingBodies {
			rule += " " + m.working(st.dim, "reading descriptions…")
		}
		parts = append(parts, rule)
	}
	return strings.Join(parts, st.dim.Render("   "))
}
