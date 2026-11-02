package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/babarot/gh-triage/internal/keymap"
)

// The ? popup tells every key, a section to a scope, as the preview pane
// tells its parts: the section of the screen it was opened from comes
// first and unfolded, the rest folded to their titles and how many keys
// they have. z or a click on a section's header folds or unfolds it;
// the moving keys and the wheel scroll it.

// helpSection is a section of the ? popup: its title, its rows as keys
// and what they do, and whether its keys work on the screen ? was opened
// from.
type helpSection struct {
	title string
	rows  [][2]string
	works bool
}

// helpScopes are the scopes the ? popup tells, with their titles.
var helpScopes = []struct {
	s     keymap.Scope
	title string
}{
	{keymap.ScopeGlobal, "anywhere"}, {keymap.ScopeNav, "moving"}, {keymap.ScopeList, "the list and its preview"},
	{keymap.ScopePopup, "a confirm popup"},
	{keymap.ScopeTriage, "claude on the list"}, {keymap.ScopeChat, "claude on a pull request"},
	{keymap.ScopeCompose, "writing a review"},
}

const helpKeyWidth = 16

// helpScope is the scope of the screen ? was opened from.
func (m *Model) helpScope() keymap.Scope {
	switch m.view {
	case viewChat:
		return keymap.ScopeChat
	default:
		return keymap.ScopeList
	}
}

// helpRows are the key rows of scope s. The tab_N actions, one key each,
// are told in one row.
func (m *Model) helpRows(s keymap.Scope) [][2]string {
	var rows [][2]string
	var tabs []string
	tabsAt := -1
	for _, a := range m.keys.Actions(s) {
		ks := m.keys.Keys(a)
		if !ready[a] || len(ks) == 0 || !m.hasTabFor(a) {
			continue
		}
		if strings.HasPrefix(string(a), "tab_") && len(ks) == 1 {
			if tabsAt < 0 {
				tabsAt = len(rows)
				rows = append(rows, [2]string{})
			}
			tabs = append(tabs, ks[0])
			continue
		}
		rows = append(rows, [2]string{strings.Join(ks, " "), m.keys.Help(a)})
	}
	if tabsAt >= 0 {
		keys := strings.Join(tabs, " ")
		if len(tabs) > 1 && lipgloss.Width(keys) > helpKeyWidth-2 {
			keys = tabs[0] + " … " + tabs[len(tabs)-1]
		}
		rows[tabsAt] = [2]string{keys, fmt.Sprintf("tab 1 to %d", len(tabs))}
	}
	return rows
}

// helpSections are the sections of the ? popup: the screen's own first,
// then the scopes in their order, the fixed keys among anywhere, and the
// mouse last. The keys of the screen's own scope, of anywhere and moving,
// and the mouse work there.
func (m *Model) helpSections() []helpSection {
	here := m.helpScope()
	rowsOf := func(s keymap.Scope) [][2]string {
		rows := m.helpRows(s)
		if s == keymap.ScopeGlobal {
			rows = append(rows, [2]string{keymap.Interrupt, "quit"}, [2]string{keymap.Back, "back"})
		}
		return rows
	}
	works := func(s keymap.Scope) bool {
		return s == here || s == keymap.ScopeGlobal || s == keymap.ScopeNav
	}
	var out []helpSection
	for _, sc := range helpScopes {
		if sc.s == here {
			out = append(out, helpSection{sc.title, rowsOf(sc.s), true})
		}
	}
	for _, sc := range helpScopes {
		if rows := rowsOf(sc.s); sc.s != here && len(rows) > 0 {
			out = append(out, helpSection{sc.title, rows, works(sc.s)})
		}
	}
	return append(out, helpSection{"mouse", [][2]string{
		{"click", "tab, row, pane tab"},
		{"double click", "actions"},
		{"click", "@you, an author, a PR: browser"},
		{"wheel", "scroll"},
		{"drag", "the pane's top edge: resize"},
	}, true})
}

// openHelp opens the ? popup at its top, with only the section of the
// screen it is opened from unfolded.
func (m *Model) openHelp() {
	m.help, m.helpTop = true, 0
	m.helpFolded = map[string]bool{}
	for i, s := range m.helpSections() {
		m.helpFolded[s.title] = i > 0
	}
}

// helpLines are the lines of the ? popup w cells wide, with the title of
// the section each header line is of ("" for the other lines). A section
// whose keys work on the screen ? was opened from is drawn in color and
// says so; the others are grayed.
func (m *Model) helpLines(w int) ([]string, []string) {
	st := m.st
	var lines, heads []string
	for _, s := range m.helpSections() {
		mark, title, key, what := st.blue, st.blue.Bold(true), st.key, st.muted
		if !s.works {
			mark, title, key, what = st.dim, st.muted.Bold(true), st.muted, st.dim
		}
		arrow, rest := "▾ ", ""
		if m.helpFolded[s.title] {
			arrow, rest = "▸ ", st.dim.Render(fmt.Sprintf(" (%d)", len(s.rows)))
		}
		if s.works {
			rest += st.dim.Render(" · works here")
		}
		h := mark.Render(arrow) + title.Render(s.title) + rest + " "
		h += st.border.Render(strings.Repeat("─", max(w-lipgloss.Width(h), 0)))
		lines, heads = append(lines, h), append(heads, s.title)
		if !m.helpFolded[s.title] {
			for _, r := range s.rows {
				for _, l := range hang("  "+fit(key.Render(r[0]), helpKeyWidth), what.Render(r[1]), w) {
					lines, heads = append(lines, l), append(heads, "")
				}
			}
		}
	}
	return lines, heads
}

// helpWidth is the width of the ? popup's box.
func (m *Model) helpWidth() int { return min(64, max(m.width-2, 30)) }

// helpHeight is how many of n lines the ? popup shows at once: at most
// about two thirds of the terminal, so that the screen stays around it.
func (m *Model) helpHeight(n int) int {
	return max(min(n, (m.height-2)*2/3), 1)
}

// helpShown are the ? popup's lines and the titles of their headers, with
// where it is scrolled to, kept within them.
func (m *Model) helpShown() (lines, heads []string, top, h int) {
	lines, heads = m.helpLines(m.helpWidth() - 4)
	h = m.helpHeight(len(lines))
	m.helpTop = min(max(m.helpTop, 0), max(len(lines)-h, 0))
	return lines, heads, m.helpTop, h
}

// scrollHelp moves the ? popup by d lines, within its lines.
func (m *Model) scrollHelp(d int) {
	m.helpTop += d
	m.helpShown()
}

// foldHelp folds or unfolds the first section whose header is in sight,
// or, with none in sight, the one the popup is in; the header stays in
// sight.
func (m *Model) foldHelp() {
	_, heads, top, h := m.helpShown()
	for _, t := range heads[top:min(top+h, len(heads))] {
		if t != "" {
			m.helpFolded[t] = !m.helpFolded[t]
			m.helpShown()
			return
		}
	}
	for i := min(top, len(heads)-1); i >= 0; i-- {
		if heads[i] != "" {
			m.helpFolded[heads[i]] = !m.helpFolded[heads[i]]
			m.helpTop = i
			m.helpShown()
			return
		}
	}
}

// lastKey is the last key of action a, which is j for down rather than
// the arrow.
func (m *Model) lastKey(a keymap.Action) string {
	if ks := m.keys.Keys(a); len(ks) > 0 {
		return ks[len(ks)-1]
	}
	return "(unbound)"
}

// helpKey is a key pressed with the ? popup open: it scrolls the popup,
// folds a section, or closes it.
func (m *Model) helpKey(k string) {
	_, _, _, h := m.helpShown()
	page := max(h-1, 1)
	switch {
	case k == keymap.Back || m.keys.Matches("help", k) || m.keys.Matches("quit", k):
		m.help = false
	case m.keys.Matches("unfold", k):
		m.foldHelp()
	case m.keys.Matches("down", k):
		m.scrollHelp(1)
	case m.keys.Matches("up", k):
		m.scrollHelp(-1)
	case m.keys.Matches("page_down", k):
		m.scrollHelp(page)
	case m.keys.Matches("page_up", k):
		m.scrollHelp(-page)
	case m.keys.Matches("bottom", k):
		m.scrollHelp(1 << 20)
	case m.keys.Matches("top", k):
		m.scrollHelp(-(1 << 20))
	}
}

// helpClick is a click at x, y with the ? popup open: on a section's
// header it folds or unfolds it, outside the box it closes the popup.
func (m *Model) helpClick(x, y int) {
	b := m.helpBox
	if x < b.x || x >= b.x+b.w || y < b.y || y >= b.y+b.h {
		m.help = false
		return
	}
	if i := y - b.y - 1; i >= 0 && i < len(b.heads) && b.heads[i] != "" {
		m.helpFolded[b.heads[i]] = !m.helpFolded[b.heads[i]]
		m.helpShown()
	}
}

// helpBox is where the ? popup was last drawn, and the titles of the
// headers on its lines, for a click.
type helpBox struct {
	x, y, w, h int
	heads      []string
}

// renderHelp is the ? popup centered over lines: the part of its lines
// scrolled to, in a box whose bottom edge tells how much is above and
// below.
func (m *Model) renderHelp(lines []string) []string {
	st := m.st
	all, heads, top, h := m.helpShown()
	w := m.helpWidth()
	box := box(st, "keys · "+m.firstKey("unfold")+" fold · "+m.firstKey("help")+" esc close", all[top:top+h], w)
	if len(all) > h {
		box[len(box)-1] = scrollEdge(st, w, top, len(all)-top-h, m.scrollKeys()+" wheel")
	}
	y := max((len(lines)-len(box))/2, 0)
	m.helpBox = helpBox{x: max((m.width-w)/2, 0), y: y, w: w, h: len(box), heads: heads[top : top+h]}
	return m.overlay(lines, box, y, m.width)
}
