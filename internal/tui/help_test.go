package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// helpText is the ? popup's lines, unstyled.
func helpText(m *Model) string {
	lines, _ := m.helpLines(60)
	return ansi.Strip(strings.Join(lines, "\n"))
}

// The ? popup has a section for every scope; that of the screen it was
// opened from comes first and unfolded, the rest folded with their counts.
func TestHelpUnfoldsTheScreenItWasOpenedFrom(t *testing.T) {
	m := newModel(fixture(), cachedReview())
	run(m, m.Init())
	press(m, "?")
	s := helpText(m)
	if !strings.HasPrefix(s, "▾ the list and its preview · works here") || !strings.Contains(s, "select by rule") {
		t.Errorf("the list's section is not first and unfolded:\n%s", s)
	}
	for _, want := range []string{"▸ anywhere (6) · works here", "▸ moving (6) · works here", "▸ writing a review (3) ─", "▸ mouse (5) · works here"} {
		if !strings.Contains(s, want) {
			t.Errorf("the ? popup has no %q:\n%s", want, s)
		}
	}
	if n := len(m.renderHelp(make([]string, 34))); n != 34 {
		t.Errorf("the popup changed the screen to %d lines", n)
	}
	if !strings.Contains(screen(m), "triage") {
		t.Errorf("the ? popup covers the header:\n%s", screen(m))
	}

	press(m, "esc", "i", "?")
	if s := helpText(m); !strings.HasPrefix(s, "▾ claude on a pull request · works here") || !strings.Contains(s, "▸ the list and its preview (31) ─") {
		t.Errorf("the Claude screen's section is not first and unfolded:\n%s", s)
	}
}

// z folds the first section in sight, and a click on a header folds or
// unfolds that one; a click outside the box closes the popup.
func TestHelpFolds(t *testing.T) {
	m := newModel(fixture(), cachedReview())
	run(m, m.Init())
	press(m, "?", "z")
	if s := helpText(m); !strings.HasPrefix(s, "▸ the list and its preview (31) · works here") {
		t.Errorf("z did not fold the list's section:\n%s", s)
	}
	x, y := lineOf(t, m, "▸ moving")
	mouseAt(m, tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
	if s := helpText(m); !strings.Contains(s, "▾ moving") || !strings.Contains(s, "page down") {
		t.Errorf("a click on the header did not unfold moving:\n%s", s)
	}
	mouseAt(m, tea.MouseClickMsg{X: 0, Y: 0, Button: tea.MouseLeft})
	if m.help {
		t.Error("a click outside the popup did not close it")
	}
}

// j, G and the wheel scroll the ? popup, within its lines.
func TestHelpScrolls(t *testing.T) {
	m := newModel(fixture(), cachedReview())
	run(m, m.Init())
	press(m, "?")
	press(m, "j", "j")
	if m.helpTop != 2 {
		t.Errorf("j j scrolled to %d, want 2", m.helpTop)
	}
	press(m, "G")
	s := screen(m)
	if !strings.Contains(s, "▸ mouse (5) · works here") || strings.Contains(s, "↓ ") {
		t.Errorf("G did not scroll to the end:\n%s", s)
	}
	end := m.helpTop
	press(m, "j")
	if m.helpTop != end {
		t.Errorf("j scrolled past the end: %d, want %d", m.helpTop, end)
	}
	mouseAt(m, tea.MouseWheelMsg{X: 70, Y: 10, Button: tea.MouseWheelUp})
	if m.helpTop != end-wheelLines {
		t.Errorf("the wheel scrolled to %d, want %d", m.helpTop, end-wheelLines)
	}
	press(m, "?", "?")
	if !m.help || m.helpTop != 0 {
		t.Errorf("? again opens the popup at %d, want its top", m.helpTop)
	}
}

// The key line keeps ? at any width, and esc where it steps back: the
// hints that do not fit go from before them. On the list esc would quit,
// and is not told until there is a selection or a filter to clear.
func TestHintsKeepHelpAndBack(t *testing.T) {
	last := func(m *Model) string {
		lines := strings.Split(screen(m), "\n")
		l := lines[len(lines)-1]
		if w := ansi.StringWidth(l); w > m.width {
			t.Errorf("width %d: key line is %d wide: %q", m.width, w, l)
		}
		return strings.TrimSpace(l)
	}
	for _, w := range []int{130, 80, 50} {
		m := startSet(t, testSettings(), w)
		if got := last(m); !strings.HasSuffix(got, "? keys") {
			t.Errorf("width %d: key line = %q, want it to end with ?", w, got)
		}
		press(m, "space")
		if got := last(m); !strings.HasSuffix(got, "? keys · esc clear") {
			t.Errorf("width %d, a selection: key line = %q, want it to end with ? and esc", w, got)
		}
	}
}
