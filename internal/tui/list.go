package tui

import (
	"fmt"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/babarot/gh-triage/internal/keymap"
	"github.com/babarot/gh-triage/internal/model"
	"github.com/babarot/gh-triage/internal/plan"
)

// listKey handles a key on the list.
func (m *Model) listKey(k string) tea.Cmd {
	t := m.tab()
	rs := m.rows(t)
	is := func(a keymap.Action) bool { return m.keys.Matches(a, k) }
	move := func(to int) tea.Cmd {
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
	switch {
	case is("quit"):
		return tea.Quit
	case k == keymap.Back:
		if m.filter != "" {
			m.filter = ""
			return nil
		}
		return tea.Quit
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
	case is("tab_1"), is("tab_2"), is("tab_3"), is("tab_4"):
		n := map[keymap.Action]int{"tab_1": 0, "tab_2": 1, "tab_3": 2, "tab_4": 3}
		for a, i := range n {
			if is(a) && i < len(m.tabs) {
				return m.switchTab(i)
			}
		}
	case is("filter"):
		m.filtering, m.filter = true, ""
	case is("refresh"):
		return m.loadTab(m.cur)
	case is("toggle_bots"):
		if t.grouped {
			t.folded[plan.GroupBots] = !t.folded[plan.GroupBots]
		}
	case is("show_closed"):
		if t.inbox {
			t.showClosed = !t.showClosed
			t.sel = 0
		}
	case is("toggle_pane"):
		m.pane.on = !m.pane.on
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
		if p, _, ok := m.currentPR(); ok {
			m.pane.unfold[p.Key] = !m.pane.unfold[p.Key]
		}
	}
	r, ok := m.current()
	if !ok {
		return nil
	}
	if r.kind == rowGroup {
		if is("open") || is("select") {
			t.folded[r.group] = !t.folded[r.group]
		}
		return nil
	}
	p := r.pr
	switch {
	case is("open"):
		return m.openPR(p)
	case is("approve"):
		return m.askApprove(p)
	case is("comment"):
		return m.startCompose(p, model.EventComment)
	case is("request_changes"):
		return m.startCompose(p, model.EventRequestChanges)
	case is("browse"):
		return m.browse(p)
	case is("hide"):
		return m.hide(p, r.notif)
	}
	return nil
}

func (m *Model) switchTab(i int) tea.Cmd {
	m.cur = (i + len(m.tabs)) % len(m.tabs)
	m.pane.scroll = 0
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
	case msg.Text != "":
		m.filter += msg.Text
	}
	m.tab().sel, m.tab().offset = 0, 0
	return m.ensure()
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

func (m *Model) browse(p model.PR) tea.Cmd {
	if p.URL == "" {
		return nil
	}
	url := p.URL
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
	if m.pane.on && m.set.PreviewHeight > 0 {
		pane = m.set.PreviewHeight + 2
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

func (m *Model) columns(t *tab) []column {
	w := m.width
	cols := []column{{id: "pr", title: "    PR", width: 28}, {id: "title", title: "TITLE"}}
	showAuthor := w >= 110 && !m.allMine(t)
	if showAuthor {
		cols = append(cols, column{id: "author", title: "AUTHOR", width: 14})
	}
	cols = append(cols, column{id: "ci", title: "CI", width: 2}, column{id: "review", title: "REVIEW", width: 7})
	if t.grouped || t.inbox {
		cols = append(cols, column{id: "why", title: "WHY", width: 10})
	}
	if w >= 120 {
		cols = append(cols, column{id: "size", title: "SIZE", width: 11})
	}
	cols = append(cols, column{id: "age", title: "AGE", width: 4, right: true})
	fixed := 0
	for _, c := range cols {
		fixed += c.width + 2
	}
	for i := range cols {
		if cols[i].width == 0 {
			cols[i].width = max(w-fixed-1, 10)
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
		b.WriteString(m.st.muted.Render("  reading "+t.name+"…") + "\n")
		lines++
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
	if t.inbox && !t.showClosed && lines < height {
		if _, closed := m.inboxRows(t); closed > 0 && len(rs) < height {
			noun := "notifications"
			if closed == 1 {
				noun = "notification"
			}
			b.WriteString(m.st.dim.Render(fmt.Sprintf("  %d %s on merged or closed pull requests hidden · %s to show",
				closed, noun, m.firstKey("show_closed"))) + "\n")
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
			return m.st.sel.Render(fit(line, m.width))
		}
		return line
	}
	var cells []string
	for _, c := range cols {
		s := m.cell(c.id, r, sel)
		if c.right {
			s = spaces(c.width-lipgloss.Width(s)) + s
		}
		cells = append(cells, fit(s, c.width))
	}
	line := strings.Join(cells, "  ")
	if sel {
		return m.st.sel.Render(fit(line, m.width))
	}
	return line
}

var groupLabel = map[plan.Group]string{
	plan.GroupYou:  "requested from you",
	plan.GroupTeam: "via team",
	plan.GroupBots: "bots",
}

func (m *Model) cell(id string, r row, sel bool) string {
	p, st := r.pr, m.st
	switch id {
	case "pr":
		cursor, mark := " ", " "
		if sel {
			cursor = st.blue.Render("▎")
		}
		if _, ok := m.unread[p.Key]; ok {
			mark = st.blue.Render("●")
		}
		repo := p.Key.Repository()
		if slices.Contains(m.set.HomeOwners, p.Key.Owner) {
			repo = p.Key.Repo
		}
		name, num := st.sub1, st.muted
		if p.Draft {
			name, num = st.dim, st.dim
		}
		return cursor + " " + mark + " " + name.Render(repo) + num.Render(fmt.Sprintf("#%d", p.Key.Number))
	case "title":
		switch {
		case p.State == model.StateMerged || p.State == model.StateClosed:
			return st.dim.Render(p.Title) + " " + st.dim.Render(strings.ToLower(string(p.State)))
		case p.Draft:
			return st.dim.Render(p.Title)
		}
		return st.text.Render(p.Title)
	case "author":
		if p.AuthorIsBot {
			return st.dim.Render(p.Author)
		}
		return st.sub0.Render(p.Author)
	case "ci":
		return m.ciMark(p.Checks)
	case "review":
		s := m.decisionMark(p.Decision)
		if r, ok := p.ReviewBy(m.viewer); ok && m.viewer != "" {
			s += " " + st.me.Render("me"+reviewMark[r.State])
		}
		return s
	case "why":
		if r.notif != nil {
			return st.sub0.Render(strings.TrimSuffix(r.notif.Reason, "_requested"))
		}
		if p.RequestedFrom(m.viewer) {
			return st.me.Render("you")
		}
		var teams []string
		for _, rq := range p.Requested {
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
