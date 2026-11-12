package tui

import (
	"testing"

	"github.com/babarot/gh-desk/internal/plan"
	"github.com/babarot/gh-desk/internal/runner"
	"github.com/babarot/gh-desk/internal/tui/keymap"
)

// listPopup opens one of the popups that hold a list, with at least four
// items, and returns its cursor and whether it is still open.
type listPopup struct {
	name   string
	opener string
	open   func(*Model)
	sel    func(*Model) int
	shown  func(*Model) bool
}

func listPopups() []listPopup {
	return []listPopup{
		{name: "menu", opener: "enter", open: func(m *Model) { press(m, "enter") },
			sel: func(m *Model) int { return m.menu.sel }, shown: func(m *Model) bool { return m.menu != nil }},
		{name: "picker", opener: "S", open: func(m *Model) {
			for range 4 {
				m.set.Rules = append(m.set.Rules, plan.Rule{Name: "more", Authors: []string{"renovate"}})
			}
			press(m, "S")
		}, sel: func(m *Model) int { return m.picker.sel }, shown: func(m *Model) bool { return m.picker != nil }},
		{name: "notifications", opener: "n", open: func(m *Model) {
			for range 4 {
				m.pushNote("✓", "merged", []noteItem{{key: key("acme", "api", 12), text: "merged", result: runner.Merged}})
			}
			m.openNotes()
		}, sel: func(m *Model) int { return m.notesAt.sel }, shown: func(m *Model) bool { return m.notesAt != nil }},
	}
}

// Every popup that holds a list moves on all of nav, as the list does
// (ADR-125).
func TestPopupListsMoveOnNav(t *testing.T) {
	for _, p := range listPopups() {
		t.Run(p.name, func(t *testing.T) {
			m, _ := startWith(t, fixture(), keymap.Default())
			p.open(m)
			press(m, "G")
			last := p.sel(m)
			if last < 3 {
				t.Fatalf("G left the cursor on %d", last)
			}
			press(m, "g")
			if got := p.sel(m); got != 0 {
				t.Errorf("g left the cursor on %d", got)
			}
			press(m, "ctrl+d")
			if got := p.sel(m); got == 0 {
				t.Error("page down did not move the cursor")
			}
			press(m, "ctrl+u")
			if got := p.sel(m); got != 0 {
				t.Errorf("page up left the cursor on %d", got)
			}
		})
	}
}

// esc closes a popup that holds a list, and so does the key that opened it
// unless the list reads it; quit neither closes it nor quits (ADR-125).
func TestPopupListsClose(t *testing.T) {
	for _, p := range listPopups() {
		t.Run(p.name, func(t *testing.T) {
			m, _ := startWith(t, fixture(), keymap.Default())
			p.open(m)
			_, cmd := m.Update(keyMsg("q"))
			if quits(cmd) || !p.shown(m) {
				t.Fatalf("q quit (%v) or closed the popup (%v)", quits(cmd), !p.shown(m))
			}
			press(m, "esc")
			if p.shown(m) {
				t.Error("esc did not close it")
			}
			p.open(m)
			press(m, p.opener)
			// The menu's enter chooses, so it runs an action instead.
			if p.name != "menu" && p.shown(m) {
				t.Errorf("%s did not close it", p.opener)
			}
		})
	}
}
