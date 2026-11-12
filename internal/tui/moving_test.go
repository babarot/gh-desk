package tui

import (
	"testing"

	"github.com/babarot/gh-desk/internal/runner"
	"github.com/babarot/gh-desk/internal/tui/keymap"
)

// ctrl+n and ctrl+p move as j and k do, on the list and in the popups that
// read nav's down and up (ADR-125).
func TestCtrlNAndCtrlPMove(t *testing.T) {
	t.Run("list", func(t *testing.T) {
		m, _ := startWith(t, fixture(), keymap.Default())
		before := m.tab().sel
		press(m, "ctrl+n")
		if m.tab().sel == before {
			t.Fatalf("ctrl+n did not move the cursor from %d", before)
		}
		press(m, "ctrl+p")
		if m.tab().sel != before {
			t.Errorf("ctrl+p left the cursor on %d, want %d", m.tab().sel, before)
		}
	})
	t.Run("menu", func(t *testing.T) {
		m, _ := startWith(t, fixture(), keymap.Default())
		press(m, "enter", "ctrl+n", "ctrl+n")
		if m.menu == nil || m.menu.sel != 2 {
			t.Fatalf("menu cursor after two ctrl+n: %+v", m.menu)
		}
		press(m, "ctrl+p")
		if m.menu.sel != 1 {
			t.Errorf("menu cursor after ctrl+p: %d", m.menu.sel)
		}
	})
	t.Run("picker", func(t *testing.T) {
		m, _ := startWith(t, fixture(), keymap.Default())
		m.set.Rules = append(m.set.Rules, m.set.Rules[0])
		press(m, "S", "ctrl+n")
		if m.picker == nil || m.picker.sel != 1 {
			t.Fatalf("picker cursor after ctrl+n: %+v", m.picker)
		}
	})
	t.Run("notifications", func(t *testing.T) {
		m, _ := startWith(t, fixture(), keymap.Default())
		m.pushNote("✓", "merged", []noteItem{{key: key("acme", "api", 12), text: "merged", result: runner.Merged}})
		m.openNotes()
		press(m, "ctrl+n")
		if m.notesAt == nil || m.notesAt.sel != 1 {
			t.Fatalf("notifications cursor after ctrl+n: %+v", m.notesAt)
		}
	})
}
