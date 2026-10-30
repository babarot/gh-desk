package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/babarot/gh-triage/internal/keymap"
	"github.com/babarot/gh-triage/internal/logging"
)

func press(m *Model, k string) tea.Cmd {
	_, cmd := m.key(k)
	return cmd
}

func TestQuitFollowsTheKeymap(t *testing.T) {
	km, ps := keymap.Apply(keymap.Default(), map[string][]string{"quit": {"x"}})
	if len(ps) > 0 {
		t.Fatal(ps)
	}
	m := New(km, logging.Discard())
	if cmd := press(m, "q"); cmd != nil {
		t.Error("q quit after quit was rebound to x")
	}
	if cmd := press(m, "x"); cmd == nil {
		t.Error("x did not quit")
	}
	if cmd := press(m, "ctrl+c"); cmd == nil {
		t.Error("ctrl+c did not quit")
	}
}

func TestHelpListsBoundKeys(t *testing.T) {
	m := New(keymap.Default(), logging.Discard())
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	press(m, "?")
	out := m.render()
	for _, want := range []string{"approve", "merge", "ctrl+s"} {
		if !strings.Contains(out, want) {
			t.Errorf("help does not show %q", want)
		}
	}
	press(m, "esc")
	if m.help {
		t.Error("esc did not close help")
	}
}
