package tui

import (
	"fmt"
	"slices"
	"strings"

	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

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
		if strings.Contains(strings.ToLower(scopeName(v)), q) {
			out = append(out, v)
		}
	}
	return out
}

// chooserKey handles a key while the chooser is open: the arrows and the
// keys of history_next and history_prev move, enter chooses, esc closes,
// and the rest is typed. j and k are typed, as a name
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
	// ctrl+n and ctrl+p move too, as in the triage's lists, since they
	// type nothing.
	case is("down") && msg.Text == "", is("history_next"):
		c.sel = min(c.sel+1, max(len(vs)-1, 0))
	case is("up") && msg.Text == "", is("history_prev"):
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
	vs := m.chooserValues()
	cols := m.scopeColumns(vs, min(max(m.width-8, 40), 110, m.width)-4)
	w := min(max(scopeLead+cols.name+cols.sum+4, 60), max(m.width-8, 40), 110, m.width)
	inner := w - 4
	// The counts go to the right edge: the names take what is left.
	cols.name = max(cols.name, inner-scopeLead-cols.sum)
	c.input.SetWidth(inner)
	c.sel = min(c.sel, max(len(vs)-1, 0))
	counts := m.scopeCounts()

	// The tabs' names head the columns of their counts, once.
	var head strings.Builder
	head.WriteString(spaces(scopeLead + cols.name))
	for j, t := range m.tabs {
		head.WriteString("  " + st.dim.Render(fitLeft(safeLine(t.name), cols.tabs[j])))
	}
	lines := []string{c.input.View(), "", head.String()}
	items := noItems(len(lines) + 1)
	room := max(min(16, m.popupHeight()-6), 1)
	c.top = min(max(c.top, c.sel-room+1, 0), c.sel)
	if len(vs) == 0 {
		lines = append(lines, st.dim.Render("  no "+m.set.Scope+" holds that"))
		items = append(items, -1)
	}
	for i := c.top; i < min(c.top+room, len(vs)); i++ {
		v := vs[i]
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
		line := cursor + mark + fit(nameStyle.Render(safeLine(scopeName(v))), cols.name)
		for j := range m.tabs {
			x := 0
			if n != nil {
				x = n[j]
			}
			cnt := st.dim
			if x > 0 {
				cnt = st.mauve
			}
			line += "  " + cnt.Render(fmt.Sprintf("%*d", cols.tabs[j], x))
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

// scopeName is a value as the chooser names it: all for none.
func scopeName(v string) string {
	if v == "" {
		return allScope
	}
	return v
}

// scopeLead is the cells before a value's name in the chooser: the cursor
// and the mark of the value chosen.
const scopeLead = 4

// scopeWidths are the cells the chooser gives a value's name and each tab's
// count, the tab's name heading it; sum is those of the counts with the
// gaps before them.
type scopeWidths struct {
	name int
	tabs []int
	sum  int
}

// minScopeName is the fewest cells a value's name is cut to before the
// tabs' names are.
const minScopeName = 12

// scopeColumns fit the chooser's columns in inner cells: a name as wide as
// the longest, at most 34, and each count as wide as its tab's name. When
// they do not fit, the names are cut, then the tabs' names, down to the
// counts.
func (m *Model) scopeColumns(vs []string, inner int) scopeWidths {
	var c scopeWidths
	for _, v := range vs {
		c.name = max(c.name, ansi.StringWidth(safeLine(scopeName(v))))
	}
	c.name = min(c.name, 34)
	c.tabs = make([]int, len(m.tabs))
	sum := func() int {
		n := 0
		for _, w := range c.tabs {
			n += 2 + w
		}
		return n
	}
	for j, t := range m.tabs {
		c.tabs[j] = max(ansi.StringWidth(safeLine(t.name)), 3)
	}
	if over := scopeLead + c.name + sum() - inner; over > 0 {
		c.name = max(c.name-over, min(c.name, minScopeName))
	}
	for over := scopeLead + c.name + sum() - inner; over > 0; over = scopeLead + c.name + sum() - inner {
		j := 0
		for k, w := range c.tabs {
			if w > c.tabs[j] {
				j = k
			}
		}
		if c.tabs[j] <= 3 {
			break
		}
		c.tabs[j]--
	}
	c.sum = sum()
	return c
}

// fitLeft is s cut to w cells and padded on the left, as a count's head is
// over its column.
func fitLeft(s string, w int) string {
	s = ansi.Truncate(s, w, "…")
	return spaces(w-ansi.StringWidth(s)) + s
}
