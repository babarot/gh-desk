package tui

import (
	"fmt"
	"slices"
	"strings"

	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"

	"github.com/babarot/gh-desk/internal/tui/keymap"
)

// scopeChooser is the popup that chooses the scope's value: every value
// with how many rows each tab shows of it, narrowed by what is typed.
type scopeChooser struct {
	input textarea.Model
	// sel is the value under the cursor, an index of the values shown;
	// top is the first one drawn.
	sel, top int
	pointer
}

// openChooser opens the scope chooser on the value chosen.
func (m *Model) openChooser() tea.Cmd {
	if m.set.Scope == "" {
		m.refuse("no scope is set: list.scope narrows every tab by an owner, a repo or an author")
		return nil
	}
	ta := newInput(m.st)
	ta.ShowLineNumbers = false
	ta.SetHeight(1)
	m.chooser = &scopeChooser{input: ta}
	m.chooser.sel = max(slices.Index(m.chooserValues(), m.scope), 0)
	return m.chooser.input.Focus()
}

// chooserValues are the values the chooser shows: those that hold what is
// typed, none among them while "all" does.
func (m *Model) chooserValues() []string {
	q := ""
	if m.chooser != nil {
		q = strings.ToLower(strings.TrimSpace(m.chooser.input.Value()))
	}
	var out []string
	for _, v := range m.scopeChoices() {
		name := v
		if v == "" {
			name = allScope
		}
		if strings.Contains(strings.ToLower(name), q) {
			out = append(out, v)
		}
	}
	return out
}

// chooserKey handles a key while the chooser is open: the arrows move, enter
// chooses, esc closes, and the rest is typed. j and k are typed, as a name
// may hold them.
func (m *Model) chooserKey(msg tea.KeyPressMsg) tea.Cmd {
	c, k := m.chooser, msg.String()
	is := func(a keymap.Action) bool { return m.keys.Matches(a, k) }
	vs := m.chooserValues()
	switch {
	case k == keymap.Back:
		m.chooser = nil
	case is("choose"):
		return m.chooseScope()
	case is("down") && msg.Text == "":
		c.sel = min(c.sel+1, max(len(vs)-1, 0))
	case is("up") && msg.Text == "":
		c.sel = max(c.sel-1, 0)
	default:
		before := c.input.Value()
		var cmd tea.Cmd
		c.input, cmd = c.input.Update(msg)
		if c.input.Value() != before {
			c.sel, c.top = 0, 0
		}
		return cmd
	}
	return nil
}

// chooseScope chooses the value under the chooser's cursor and closes it.
func (m *Model) chooseScope() tea.Cmd {
	vs := m.chooserValues()
	c := m.chooser
	m.chooser = nil
	if len(vs) == 0 {
		return nil
	}
	return m.setScope(vs[min(c.sel, len(vs)-1)])
}

// scopeCounts are, by value, how many rows each tab shows of it, counted as
// a tab's count is while the rows are narrowed (narrowedCount); "" holds
// each tab's count with none chosen, as the header shows it.
func (m *Model) scopeCounts() map[string][]int {
	kept := m.scope
	defer func() { m.scope = kept }()
	m.scope = ""
	out := map[string][]int{}
	for i, t := range m.tabs {
		for _, r := range m.rowsOf(t, m.filter, t.ruleFilter, nil) {
			if r.kind != rowPR {
				continue
			}
			if _, unread := m.unread[r.pr.Key]; t.inbox && !unread {
				continue
			}
			v := m.scopeOf(r.pr)
			if out[v] == nil {
				out[v] = make([]int, len(m.tabs))
			}
			out[v][i]++
		}
	}
	all := make([]int, len(m.tabs))
	for i, t := range m.tabs {
		all[i] = m.tabCount(t)
	}
	out[""] = all
	return out
}

// scopeTitle is what the chooser's top edge says it chooses.
func (m *Model) scopeTitle() string {
	what := "a " + m.set.Scope
	if m.set.Scope == ScopeOwner {
		what = "an owner"
	}
	return "choose " + what + " for every tab"
}

// renderChooser draws the chooser, and where its text box is among its
// lines.
func (m *Model) renderChooser() ([]string, int) {
	st, c := m.st, m.chooser
	w := min(max(m.width-8, 40), 110)
	inner := w - 4
	c.input.SetWidth(inner)
	vs := m.chooserValues()
	c.sel = min(c.sel, max(len(vs)-1, 0))
	counts := m.scopeCounts()

	lines := []string{c.input.View(), ""}
	items := noItems(len(lines) + 1)
	room := max(min(16, m.popupHeight()-6), 1)
	c.top = min(max(c.top, c.sel-room+1, 0), c.sel)
	if len(vs) == 0 {
		lines = append(lines, st.dim.Render("  no "+m.set.Scope+" holds that"))
		items = append(items, -1)
	}
	nameW := 34
	for i := c.top; i < min(c.top+room, len(vs)); i++ {
		v := vs[i]
		name := v
		if v == "" {
			name = allScope
		}
		cursor, mark := "  ", "  "
		if i == c.sel {
			cursor = st.blue.Render("▎") + " "
		}
		if v == m.scope {
			mark = st.blue.Render("•") + " "
		}
		n := counts[v]
		total := 0
		for _, x := range n {
			total += x
		}
		nameStyle := st.sub1
		if total == 0 {
			nameStyle = st.dim
		}
		line := cursor + mark + fit(nameStyle.Render(safeLine(name)), nameW)
		for j, t := range m.tabs {
			x := 0
			if n != nil {
				x = n[j]
			}
			cnt := st.dim.Render(fmt.Sprintf("%3d", x))
			if x > 0 {
				cnt = st.mauve.Render(fmt.Sprintf("%3d", x))
			}
			line += st.dim.Render(fit(safeLine(t.name), 9)) + " " + cnt + "  "
		}
		if i == c.sel {
			line = onBackground(line, st.sel, inner)
		}
		lines = append(lines, line)
		items = append(items, i)
	}
	if len(vs) > room {
		lines = append(lines, st.dim.Render(fmt.Sprintf("  %d–%d of %d", c.top+1, min(c.top+room, len(vs)), len(vs))))
		items = append(items, -1)
	}
	lines = append(lines, "")
	foot := keyLines(st, inner,
		st.key.Render("type")+" "+st.muted.Render("to narrow"),
		st.key.Render("↑↓")+" "+st.muted.Render("move"),
		st.key.Render(m.firstKey("choose"))+" "+st.muted.Render("choose"),
		st.key.Render(keymap.Back)+" "+st.muted.Render("cancel"))
	lines = append(lines, foot...)
	items = append(items, noItems(1+len(foot)+1)...)
	c.items = items
	return box(st, m.scopeTitle(), lines, w), 1
}

// tabCount is t's count as the header shows it, 0 while it is not read.
func (m *Model) tabCount(t *tab) int {
	switch {
	case !t.loaded || !t.inbox && t.err != nil:
		return 0
	case m.narrowed(t) || m.scoped():
		return m.narrowedCount(t)
	case t.inbox:
		return m.unreadShown(t)
	}
	return m.searchCount(t)
}
