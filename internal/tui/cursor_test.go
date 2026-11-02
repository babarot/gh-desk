package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/babarot/gh-triage/internal/keymap"
)

// The terminal's cursor stands right after the text typed in each text
// box, where an input method shows the text being composed.
func TestCursorAfterTheTypedText(t *testing.T) {
	typed := []string{"a", "あ", "b"}
	for _, tc := range []struct {
		name string
		open []string
	}{
		{"comment", []string{"c"}},
		{"review", []string{"R"}},
		{"claude", []string{"C"}},
		{"chat", []string{"i", "i"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m, _ := startWith(t, fixture(), keymap.Default())
			fakeClaudeOf(t, m)
			press(m, tc.open...)
			press(m, typed...)
			s, cur := m.render()
			if cur == nil {
				t.Fatal("no cursor")
			}
			lines := strings.Split(ansi.Strip(s), "\n")
			if cur.Y < 0 || cur.Y >= len(lines) {
				t.Fatalf("cursor on row %d of %d", cur.Y, len(lines))
			}
			if left := ansi.Truncate(lines[cur.Y], cur.X, ""); !strings.HasSuffix(left, strings.Join(typed, "")) {
				t.Errorf("left of the cursor: %q", left)
			}
		})
	}
}

// No cursor is shown while nothing is typed in.
func TestNoCursorOnTheList(t *testing.T) {
	m, _ := startWith(t, fixture(), keymap.Default())
	if _, cur := m.render(); cur != nil {
		t.Errorf("cursor at %+v", cur)
	}
}
