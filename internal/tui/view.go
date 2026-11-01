package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/babarot/gh-triage/internal/keymap"
	"github.com/babarot/gh-triage/internal/plan"
)

// View draws the screen.
func (m *Model) View() tea.View {
	v := tea.NewView(m.render())
	v.AltScreen = true
	// Clicks, the wheel, and dragging the pane's edge; see mouse.go.
	v.MouseMode = tea.MouseModeCellMotion
	return v
}

func (m *Model) render() string {
	if m.width == 0 || m.height == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(m.renderHeader() + "\n")
	if m.filtering || m.filter != "" {
		cursor := ""
		if m.filtering {
			cursor = "▏"
		}
		b.WriteString(m.st.yellow.Render("/ "+m.filter+cursor) + "\n")
	}
	body := m.height - 3
	if m.filtering || m.filter != "" {
		body--
	}
	switch m.view {
	case viewPR:
		m.renderPR(&b, body)
	case viewCompose:
		m.renderCompose(&b, body)
	case viewChat:
		m.renderChat(&b, body)
	default:
		l := m.layout()
		m.renderList(&b, l.list)
		if l.pane > 0 {
			m.renderPane(&b, l.pane)
		}
	}
	st := m.st.muted
	if m.statusErr {
		st = m.st.red
	}
	// Claude answering elsewhere takes the line until it is done, unless
	// the line tells of a failure.
	if s := m.claudeStatus(); s != "" && !m.statusErr {
		b.WriteString(fit(s, m.width) + "\n")
	} else {
		b.WriteString(fit(st.Render(m.status), m.width) + "\n")
	}
	b.WriteString(fit(m.hints(), m.width))
	lines := strings.Split(b.String(), "\n")
	switch {
	case m.bulk != nil:
		lines = overlay(lines, m.renderBulk(), 2, m.width)
	case m.notesAt != nil:
		lines = overlay(lines, m.renderNotes(), 2, m.width)
	case m.picker != nil:
		lines = overlay(lines, m.renderPick(), 2, m.width)
	case m.confirm != nil:
		lines = overlay(lines, m.renderConfirm(), 2, m.width)
	case m.triage != nil && m.triage.open:
		lines = overlay(lines, m.renderTriage(), 2, m.width)
	case m.help:
		help := m.renderHelp()
		lines = overlay(lines, help, max((len(lines)-len(help))/2, 0), m.width)
	case m.toast != nil && len(lines) > 3:
		toast := m.st.border.Render("╭ ") + m.noteIcon(m.toast.icon) + " " + m.st.sub1.Render(m.toast.title) + " " +
			m.st.dim.Render("· "+m.firstKey("notifications")+" to see") + m.st.border.Render(" ╮")
		at := len(lines) - 3
		lines[at] = place(lines[at], toast, max(m.width-lipgloss.Width(toast)-1, 0))
	}
	if len(lines) > m.height {
		lines = lines[:m.height]
	}
	return strings.Join(lines, "\n")
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
	from, to := m.shownTabs()
	var tabs []string
	for i := from; i < to; i++ {
		tabs = append(tabs, m.tabLabel(i))
	}
	left := m.headerLead() + m.st.dim.Render(tabsBefore(from)) + strings.Join(tabs, tabGap) + m.st.dim.Render(tabsAfter(to, len(m.tabs)))
	right := m.headerRight()
	gap := m.width - lipgloss.Width(left) - lipgloss.Width(right) - 1
	return m.st.header.Render(fit(left+spaces(gap)+right, m.width))
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

// tabGap is what comes between two tabs: the header's, the PR screen's,
// the pane's and the diff's file tabs. The pills' own padding spaces them
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

// headerLead is what the header shows before the tabs.
func (m *Model) headerLead() string {
	return m.st.app.Render("triage") + m.st.dim.Render(" // ") + m.st.sub0.Render(m.viewerName()) + "   "
}

// viewerName is the viewer as the header shows it, which a click opens.
func (m *Model) viewerName() string {
	if m.viewer == "" {
		return ""
	}
	return "@" + m.viewer
}

// tabLabel is tab i as the header shows it.
func (m *Model) tabLabel(i int) string {
	t := m.tabs[i]
	count := "…"
	switch {
	case !m.tabDone(t):
		count = m.spinner()
	case t.inbox && t.loaded:
		count = fmt.Sprint(len(m.unread))
	case t.loaded && t.err == nil:
		count = fmt.Sprint(t.total)
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
	if i == m.cur {
		return st.tabKeyOn.Render(key) + st.tabOn.Render(" "+t.name+" ("+count+") ")
	}
	return st.dim.Render(key) + st.tabOff.Render(" "+t.name+" ") + st.dim.Render("("+count+")") + " "
}

func (m *Model) headerRight() string {
	t, st := m.tab(), m.st
	var parts []string
	// The rows are from the cache, or a reading again failed.
	if !t.asOf.IsZero() {
		parts = append(parts, st.yellow.Render("as of "+t.asOf.Local().Format("15:04")))
	}
	if n := m.unseen(); n > 0 {
		parts = append(parts, st.mauve.Render(fmt.Sprintf("✦%d", n))+" "+st.dim.Render(m.firstKey("notifications")))
	}
	if s := m.tally(); s != "" {
		parts = append(parts, s)
	}
	if n := len(m.selected); n > 0 {
		parts = append(parts, st.mauve.Render(fmt.Sprintf("%d selected", n)))
	}
	// Results on the rows take the place of the tab's counts.
	if len(m.progress) > 0 {
		return st.sub0.Render(strings.Join(parts, " · "))
	}
	if t.grouped && t.loaded {
		n := map[plan.Group]int{}
		for _, p := range t.prs {
			n[plan.GroupOf(p, m.viewer)]++
		}
		// A group with no pull requests is left out, as in the list.
		for _, g := range t.groups {
			if n[g] > 0 {
				parts = append(parts, fmt.Sprintf("%d %s", n[g], groupCount[g]))
			}
		}
	}
	if t.loaded && t.total > len(t.prs) && !t.inbox {
		parts = append(parts, fmt.Sprintf("showing %d of %d", len(t.prs), t.total))
	}
	if len(t.warnings) > 0 {
		parts = append(parts, st.yellow.Render("⚠ read in part"))
	}
	return st.sub0.Render(strings.Join(parts, " · "))
}

// ready are the actions this version does; the hints and the ? list show
// only these.
var ready = map[keymap.Action]bool{
	"quit": true, "help": true, "refresh": true,
	"up": true, "down": true, "top": true, "bottom": true, "page_up": true, "page_down": true,
	"next_tab": true, "prev_tab": true, "tab_1": true, "tab_2": true, "tab_3": true, "tab_4": true, "tab_5": true, "tab_6": true, "tab_7": true, "tab_8": true, "tab_9": true,
	"open": true, "expand": true, "collapse": true, "approve": true, "comment": true, "request_changes": true, "browse": true, "hide": true,
	"notifications": true, "select": true, "select_shown": true, "clear_selection": true, "merge": true, "pr_merge": true,
	"include_failing": true, "choose": true, "select_rule": true,
	"toggle_bots": true, "show_settled": true, "filter": true,
	"pane_down": true, "pane_up": true, "pane_grow": true, "pane_shrink": true, "next_file": true, "prev_file": true, "pane_tab": true, "unfold": true, "toggle_pane": true,
	"pr_prev_tab": true, "pr_next_tab": true, "pr_approve": true, "pr_comment": true, "pr_request_changes": true, "pr_browse": true,
	"confirm": true, "next_kind": true, "post": true, "editor": true, "accept": true, "erase": true,
	"claude": true, "triage_apply": true, "triage_more": true, "pr_claude": true, "ask_more": true, "draft_review": true, "open_claude": true, "chat_approve": true,
}

// hints is the key line of what has the focus.
func (m *Model) hints() string {
	var as []keymap.Action
	switch {
	case m.bulk != nil:
		as = []keymap.Action{"confirm", "include_failing"}
	case m.notesAt != nil, m.picker != nil:
		as = []keymap.Action{"choose"}
	case m.confirm != nil:
		as = []keymap.Action{"confirm"}
	case m.triage != nil && m.triage.open:
		switch {
		case m.triage.typing:
			as = []keymap.Action{"accept"}
		case m.triage.asking:
		case m.applicable():
			as = []keymap.Action{"triage_apply", "triage_more"}
		default:
			as = []keymap.Action{"triage_more"}
		}
	case m.view == viewList && len(m.selected) > 0:
		as = []keymap.Action{"select", "select_shown", "clear_selection", "approve", "merge", "pane_tab", "filter", "help"}
	case m.filtering:
		as = []keymap.Action{"accept"}
	case m.view == viewCompose:
		as = []keymap.Action{"next_kind", "post", "editor"}
	case m.view == viewChat && m.chat() != nil && m.chat().typing:
		as = []keymap.Action{"accept"}
	case m.view == viewChat:
		as = []keymap.Action{"ask_more", "draft_review", "chat_approve", "open_claude", "down", "help"}
	case m.view == viewPR:
		as = []keymap.Action{"pr_next_tab", "down", "pr_approve", "pr_merge", "pr_comment", "pr_claude", "pr_browse", "help"}
	default:
		as = []keymap.Action{"open", "select", "select_rule", "approve", "merge", "comment", "claude", "browse", "pane_tab", "help"}
		if m.tab().inbox {
			as = append(as[:len(as)-1], "hide", "show_settled", "help")
		}
		if p, _, ok := m.currentPR(); ok && m.own(p) {
			as = []keymap.Action{"open", "select", "merge", "comment", "browse", "pane_tab", "filter", "help"}
		}
	}
	var parts []string
	for _, a := range as {
		if ks := m.keys.Keys(a); len(ks) > 0 {
			parts = append(parts, m.st.key.Render(ks[0])+" "+m.st.muted.Render(m.keys.Help(a)))
		}
	}
	parts = append(parts, m.st.key.Render(keymap.Back)+" "+m.st.muted.Render("back"))
	return strings.Join(parts, m.st.dim.Render(" · "))
}

// helpScopes are the scopes the ? popup tells, with their titles.
var helpScopes = []struct {
	s     keymap.Scope
	title string
}{
	{keymap.ScopeGlobal, "anywhere"}, {keymap.ScopeNav, "moving"}, {keymap.ScopeList, "the list and its preview"},
	{keymap.ScopePR, "a pull request"}, {keymap.ScopePopup, "a confirm popup"},
	{keymap.ScopeTriage, "claude on the list"}, {keymap.ScopeChat, "claude on a pull request"},
	{keymap.ScopeCompose, "writing a review"},
}

// helpScope is the scope of the screen ? was opened from.
func (m *Model) helpScope() keymap.Scope {
	switch m.view {
	case viewPR:
		return keymap.ScopePR
	case viewChat:
		return keymap.ScopeChat
	default:
		return keymap.ScopeList
	}
}

// helpRow is a row of the ? popup: its keys and what they do, in kw cells
// for the keys.
func (m *Model) helpRow(keys, what string, kw int) string {
	return "  " + fit(m.st.key.Render(keys), kw) + m.st.muted.Render(what)
}

// helpRows are the key rows of scope s. The tab_N actions, one key each,
// are told in one row.
func (m *Model) helpRows(s keymap.Scope, kw int) []string {
	var rows, tabs []string
	tabsAt := -1
	for _, a := range m.keys.Actions(s) {
		ks := m.keys.Keys(a)
		if !ready[a] || len(ks) == 0 || !m.hasTabFor(a) {
			continue
		}
		if strings.HasPrefix(string(a), "tab_") && len(ks) == 1 {
			if tabsAt < 0 {
				tabsAt = len(rows)
				rows = append(rows, "")
			}
			tabs = append(tabs, ks[0])
			continue
		}
		rows = append(rows, m.helpRow(strings.Join(ks, " "), m.keys.Help(a), kw))
	}
	if tabsAt >= 0 {
		keys := strings.Join(tabs, " ")
		if len(tabs) > 1 && lipgloss.Width(keys) > kw-2 {
			keys = tabs[0] + " … " + tabs[len(tabs)-1]
		}
		rows[tabsAt] = m.helpRow(keys, fmt.Sprintf("tab 1 to %d", len(tabs)), kw)
	}
	return rows
}

// helpGroup appends a titled group of rows to lines, a blank line apart
// from what is above it.
func helpGroup(lines []string, title string, rows []string) []string {
	if len(rows) == 0 {
		return lines
	}
	if len(lines) > 0 {
		lines = append(lines, "")
	}
	return append(append(lines, title), rows...)
}

// sideBySide puts right beside left, each column w cells wide.
func sideBySide(left, right []string, w int) []string {
	out := make([]string, max(len(left), len(right)))
	for i := range out {
		var l, r string
		if i < len(left) {
			l = left[i]
		}
		if i < len(right) {
			r = right[i]
		}
		out[i] = fit(l, w) + "  " + r
	}
	return out
}

// renderHelp is the ? popup. The keys of the screen it was opened from
// come first, marked here, then those read everywhere (the fixed keys
// among them), moving and the mouse; the other scopes are folded to their titles and how many keys
// they have. When the lines would not fit the terminal and it is wide
// enough, they are laid out in two columns.
func (m *Model) renderHelp() []string {
	st := m.st
	here := m.helpScope()
	title := func(s keymap.Scope) string {
		for _, sc := range helpScopes {
			if sc.s == s {
				return sc.title
			}
		}
		return string(s)
	}
	layout := func(kw int) (head string, hereRows, left, right []string) {
		head = st.title.Render(title(here)) + "  " + st.subOn.Render(" here ")
		hereRows = m.helpRows(here, kw)
		// The fixed keys are told with the actions read everywhere.
		anywhere := m.helpRows(keymap.ScopeGlobal, kw)
		anywhere = append(anywhere, m.helpRow(keymap.Interrupt, "quit", kw), m.helpRow(keymap.Back, "back", kw))
		left = helpGroup(left, st.group.Render(title(keymap.ScopeGlobal)), anywhere)
		right = helpGroup(right, st.group.Render(title(keymap.ScopeNav)), m.helpRows(keymap.ScopeNav, kw))
		var folded []string
		for _, sc := range helpScopes {
			if sc.s == here || sc.s == keymap.ScopeGlobal || sc.s == keymap.ScopeNav {
				continue
			}
			if n := len(m.helpRows(sc.s, kw)); n > 0 {
				folded = append(folded, "  "+st.muted.Render(fmt.Sprintf("%s · %d", sc.title, n)))
			}
		}
		left = helpGroup(left, st.muted.Render("other screens, in their key line"), folded)
		right = helpGroup(right, st.group.Render("mouse"), []string{
			m.helpRow("click", "tab, row, pane tab", kw),
			m.helpRow("double click", "open", kw),
			m.helpRow("click", "@you, an author, a PR: web", kw),
			m.helpRow("wheel", "scroll", kw),
			m.helpRow("drag", "pane's top edge: resize", kw),
		})
		return head, hereRows, left, right
	}
	maxLines := max(m.height-3, 5)
	w := min(64, max(m.width-2, 30))
	head, hereRows, left, right := layout(16)
	lines := append([]string{head}, hereRows...)
	lines = append(append(append(lines, ""), left...), append([]string{""}, right...)...)
	if len(lines) > maxLines && m.width-2 >= 90 {
		w = 90
		col := (w - 4 - 2) / 2
		head, hereRows, left, right = layout(14)
		half := (len(hereRows) + 1) / 2
		lines = append([]string{head}, sideBySide(hereRows[:half], hereRows[half:], col)...)
		lines = append(append(lines, ""), sideBySide(left, right, col)...)
	}
	if len(lines) > maxLines {
		lines = append(lines[:maxLines-1], st.dim.Render("  … a taller terminal shows the rest"))
	}
	return box(st, "keys · "+m.firstKey("help")+" esc q close", lines, w)
}
