package tui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/babarot/gh-desk/internal/model"
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

// bodyPopup opens one of the popups that hold a body to scroll, long enough
// to scroll, and returns the first line it shows.
type bodyPopup struct {
	name  string
	open  func(*testing.T) *Model
	top   func(*Model) int
	shown func(*Model) bool
}

func bodyPopups() []bodyPopup {
	return []bodyPopup{
		{name: "help", open: func(t *testing.T) *Model {
			m, _ := startWith(t, fixture(), keymap.Default())
			press(m, "?")
			return m
		}, top: func(m *Model) int { _, _, top, _ := m.helpShown(); return top }, shown: func(m *Model) bool { return m.help }},
		{name: "release notes", open: func(t *testing.T) *Model {
			var list []model.Release
			for i := range 20 {
				list = append(list, model.Release{Version: fmt.Sprintf("0.%d.0", 30-i), Published: now, Body: "## What's Changed\n* A change"})
			}
			m := startReleases(t, &Release{Running: "0.30.0", How: "gh extension upgrade desk", Latest: "0.30.0", Source: &fakeReleases{list: list}})
			press(m, "?", "w")
			return m
		}, top: func(m *Model) int { return m.relBox.top }, shown: func(m *Model) bool { return m.relBox != nil }},
		{name: "confirm", open: func(t *testing.T) *Model {
			f, keys := longFixture()
			m := startSmall(t, f)
			selectAll(m, keys)
			press(m, "A")
			return m
		}, top: func(m *Model) int { screen(m); return m.bulk.offset }, shown: func(m *Model) bool { return m.bulk != nil }},
		{name: "triage", open: func(t *testing.T) *Model {
			m, _ := startTriage(t, `{"reply":"Nothing to do.","actions":[]}`)
			m.triage.turns = append([]triageTurn{{request: "bots", reply: strings.Repeat("A reply of many lines.\n", 60)}}, m.triage.turns...)
			return m
		}, top: func(m *Model) int { screen(m); return m.triage.top }, shown: func(m *Model) bool { return m.triage.open }},
	}
}

// Every popup that holds a body scrolls it on all of nav, and q neither
// closes it nor quits (ADR-125).
func TestPopupBodiesScrollOnNav(t *testing.T) {
	for _, p := range bodyPopups() {
		t.Run(p.name, func(t *testing.T) {
			m := p.open(t)
			if !p.shown(m) {
				t.Fatal("the popup did not open")
			}
			press(m, "g")
			press(m, "G")
			end := p.top(m)
			if end == 0 {
				t.Fatalf("G did not scroll:\n%s", screen(m))
			}
			press(m, "g")
			if got := p.top(m); got != 0 {
				t.Errorf("g left it at %d", got)
			}
			press(m, "ctrl+d")
			if got := p.top(m); got <= 1 {
				t.Errorf("page down scrolled to %d", got)
			}
			press(m, "ctrl+u")
			if got := p.top(m); got != 0 {
				t.Errorf("page up left it at %d", got)
			}
			_, cmd := m.Update(keyMsg("q"))
			if quits(cmd) || !p.shown(m) {
				t.Errorf("q quit (%v) or closed it (%v)", quits(cmd), !p.shown(m))
			}
		})
	}
}

// The key that opened a popup with a body closes it; the release notes go
// back to the ? list on theirs and on esc, and ? closes both.
func TestPopupBodiesClose(t *testing.T) {
	m, _ := startWith(t, fixture(), keymap.Default())
	press(m, "?", "?")
	if m.help {
		t.Error("? did not close the ? list")
	}
	m = startReleases(t, &Release{Running: "0.7.0", How: "gh extension upgrade desk", Latest: "0.8.0", Source: &fakeReleases{list: releaseList()}})
	press(m, "?", "w", "w")
	if m.relBox != nil || !m.help {
		t.Errorf("w did not go back to the ? list: relBox %v, help %v", m.relBox, m.help)
	}
	press(m, "w", "esc")
	if m.relBox != nil || !m.help {
		t.Errorf("esc did not go back to the ? list: relBox %v, help %v", m.relBox, m.help)
	}
	press(m, "w", "?")
	if m.relBox != nil || m.help {
		t.Errorf("? did not close both: relBox %v, help %v", m.relBox, m.help)
	}
	tm, _ := startTriage(t, `{"reply":"Nothing to do.","actions":[]}`)
	press(tm, "C")
	if tm.triage.open {
		t.Error("C did not close the triage")
	}
}
