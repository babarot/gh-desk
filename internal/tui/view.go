package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

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
	b.WriteString(fit(st.Render(m.status), m.width) + "\n")
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
	case m.help:
		lines = overlay(lines, m.renderHelp(), 1, m.width)
	case m.toast != nil && len(lines) > 3:
		toast := m.st.border.Render("╭ ") + m.noteIcon(m.toast.icon) + " " + m.st.sub1.Render(m.toast.title) + " " +
			m.st.dim.Render("· "+m.firstKey("notifications")+" to see") + m.st.border.Render(" ╮")
		at := len(lines) - 3
		pad := max(m.width-lipgloss.Width(toast)-1, 0)
		lines[at] = spaces(pad) + toast
	}
	if len(lines) > m.height {
		lines = lines[:m.height]
	}
	return strings.Join(lines, "\n")
}

// overlay puts popup over lines from row top, in the middle.
func overlay(lines, popup []string, top, width int) []string {
	out := append([]string(nil), lines...)
	left := 0
	if len(popup) > 0 {
		left = max((width-lipgloss.Width(popup[0]))/2, 0)
	}
	for i, p := range popup {
		if top+i < len(out) {
			out[top+i] = spaces(left) + p
		}
	}
	return out
}

func (m *Model) renderHeader() string {
	var tabs []string
	for i := range m.tabs {
		tabs = append(tabs, m.tabLabel(i))
	}
	left := m.headerLead() + strings.Join(tabs, "  ")
	right := m.headerRight()
	gap := m.width - lipgloss.Width(left) - lipgloss.Width(right) - 1
	return m.st.header.Render(fit(left+spaces(gap)+right, m.width))
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
	// The key is dim, and so is the count in parentheses except on the
	// tab shown, so neither reads as part of the name.
	st := m.st
	name, num := st.tabOff, st.dim
	if i == m.cur {
		name, num = st.tabOn, st.tabOn
	}
	label := name.Render(t.name) + name.Render(" ") + num.Render("("+count+")")
	if ks := m.keys.Keys(keymap.Action(fmt.Sprintf("tab_%d", i+1))); len(ks) > 0 {
		label = st.dim.Render(ks[0]) + " " + label
	}
	return label
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
		parts = append(parts, fmt.Sprintf("%d to you · %d via team · %d bots", n[plan.GroupYou], n[plan.GroupTeam], n[plan.GroupBots]))
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
	"next_tab": true, "prev_tab": true, "tab_1": true, "tab_2": true, "tab_3": true, "tab_4": true,
	"open": true, "expand": true, "collapse": true, "approve": true, "comment": true, "request_changes": true, "browse": true, "hide": true,
	"notifications": true, "select": true, "select_shown": true, "clear_selection": true, "merge": true, "pr_merge": true,
	"include_failing": true, "choose": true, "select_rule": true,
	"toggle_bots": true, "show_closed": true, "filter": true,
	"pane_down": true, "pane_up": true, "pane_grow": true, "pane_shrink": true, "next_file": true, "prev_file": true, "pane_tab": true, "unfold": true, "toggle_pane": true,
	"pr_prev_tab": true, "pr_next_tab": true, "pr_approve": true, "pr_comment": true, "pr_request_changes": true, "pr_browse": true,
	"confirm": true, "next_kind": true, "post": true, "editor": true, "accept": true, "erase": true,
	"claude": true, "pr_claude": true, "ask_more": true, "draft_review": true, "open_claude": true, "chat_approve": true,
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
			as = append(as[:len(as)-1], "hide", "show_closed", "help")
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

// renderHelp is the ? popup: every action this version does, with its
// keys, scope by scope.
func (m *Model) renderHelp() []string {
	st := m.st
	var lines []string
	scopes := []struct {
		s     keymap.Scope
		title string
	}{
		{keymap.ScopeGlobal, "anywhere"}, {keymap.ScopeNav, "moving"}, {keymap.ScopeList, "the list and its preview"},
		{keymap.ScopePR, "a pull request"}, {keymap.ScopePopup, "a confirm popup"}, {keymap.ScopeChat, "claude"},
		{keymap.ScopeCompose, "writing a review"},
	}
	for _, sc := range scopes {
		var rows []string
		for _, a := range m.keys.Actions(sc.s) {
			ks := m.keys.Keys(a)
			if !ready[a] || len(ks) == 0 {
				continue
			}
			rows = append(rows, "  "+fit(st.key.Render(strings.Join(ks, " ")), 16)+st.muted.Render(m.keys.Help(a)))
		}
		if len(rows) > 0 {
			lines = append(lines, st.group.Render(sc.title))
			lines = append(lines, rows...)
		}
	}
	lines = append(lines, st.group.Render("fixed"), "  "+fit(st.key.Render(keymap.Interrupt), 16)+st.muted.Render("quit"),
		"  "+fit(st.key.Render(keymap.Back), 16)+st.muted.Render("back"),
		st.group.Render("mouse"), "  "+fit(st.key.Render("click"), 16)+st.muted.Render("tab, row, pane tab"),
		"  "+fit(st.key.Render("double click"), 16)+st.muted.Render("open"),
		"  "+fit(st.key.Render("click"), 16)+st.muted.Render("@you, an author or a PR name: browser"),
		"  "+fit(st.key.Render("wheel"), 16)+st.muted.Render("scroll"),
		"  "+fit(st.key.Render("drag"), 16)+st.muted.Render("the pane's top edge to resize it"))
	maxLines := max(m.height-3, 5)
	if len(lines) > maxLines {
		lines = append(lines[:maxLines-1], st.dim.Render("  …"))
	}
	return box(st, "keys", lines, min(max(m.width-8, 30), 70))
}
