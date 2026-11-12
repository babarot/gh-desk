package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

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

// esc alone closes a popup that holds a list: the key that opened it does
// not, and quit neither closes it nor quits (ADR-125).
func TestPopupListsClose(t *testing.T) {
	for _, p := range listPopups() {
		t.Run(p.name, func(t *testing.T) {
			m, _ := startWith(t, fixture(), keymap.Default())
			p.open(m)
			_, cmd := m.Update(keyMsg("q"))
			if quits(cmd) || !p.shown(m) {
				t.Fatalf("q quit (%v) or closed the popup (%v)", quits(cmd), !p.shown(m))
			}
			// The menu's enter chooses, and runs an action.
			if p.name != "menu" {
				press(m, p.opener)
				if !p.shown(m) {
					t.Errorf("%s closed it", p.opener)
				}
			}
			press(m, "esc")
			if p.shown(m) {
				t.Error("esc did not close it")
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

// esc alone closes a popup with a body, and the release notes go back to
// the ? list on it; the key that opened one does not close it (ADR-125).
func TestPopupBodiesClose(t *testing.T) {
	m, _ := startWith(t, fixture(), keymap.Default())
	press(m, "?", "?")
	if !m.help {
		t.Error("? closed the ? list")
	}
	press(m, "esc")
	if m.help {
		t.Error("esc did not close the ? list")
	}
	m = startReleases(t, &Release{Running: "0.7.0", How: "gh extension upgrade desk", Latest: "0.8.0", Source: &fakeReleases{list: releaseList()}})
	press(m, "?", "w", "w", "?")
	if m.relBox == nil || !m.help {
		t.Errorf("w or ? closed the release notes: relBox %v, help %v", m.relBox, m.help)
	}
	press(m, "esc")
	if m.relBox != nil || !m.help {
		t.Errorf("esc did not go back to the ? list: relBox %v, help %v", m.relBox, m.help)
	}
	tm, _ := startTriage(t, `{"reply":"Nothing to do.","actions":[]}`)
	press(tm, "C")
	if !tm.triage.open {
		t.Error("C closed the triage")
	}
	press(tm, "esc")
	if tm.triage.open {
		t.Error("esc did not close the triage")
	}
}

// In the comment of a close, every key is typed and none confirms or
// changes the popup: enter is a new line, and y, x, f and w are letters
// (ADR-066, ADR-125). esc takes the focus out, and the popup's keys work
// again.
func TestCloseCommentTakesEveryKey(t *testing.T) {
	m, r := startWith(t, fixture(), keymap.Default())
	press(m, "enter")
	for i, it := range m.menuItems() {
		if it.action == "close" {
			m.menu.sel = i
		}
	}
	press(m, "enter")
	if m.bulk == nil || m.bulk.close == nil {
		t.Fatal("close did not open its popup")
	}
	del := m.bulk.close.del
	press(m, "i", "y", "x", "enter", "f", "w")
	if m.bulk == nil || len(r.closes) != 0 {
		t.Fatalf("a key in the comment confirmed the close: %v", r.closes)
	}
	if got := m.bulk.close.ta.Value(); got != "yx\nfw" {
		t.Errorf("the comment is %q, want %q", got, "yx\nfw")
	}
	if m.bulk.close.del != del {
		t.Error("x in the comment changed whether the branches are deleted")
	}
	press(m, "esc", "x")
	if m.bulk == nil || m.bulk.close.editing || m.bulk.close.del == del {
		t.Errorf("out of the comment, x did not change the check: editing %v", m.bulk.close.editing)
	}
}

// The scope chooser types j and k, moves on the arrows and ctrl+n, and esc
// closes it at once, as its text box has the focus as long as it is open.
func TestScopeChooserKeys(t *testing.T) {
	set, _ := scopedBy(ScopeOwner, "")
	m := startSet(t, set, 130)
	press(m, "O")
	if m.chooser == nil {
		t.Fatal("O did not open the chooser")
	}
	press(m, "j", "k")
	if got := m.chooser.input.Value(); got != "jk" {
		t.Errorf("typed %q, want jk", got)
	}
	press(m, "backspace", "backspace", "ctrl+n")
	if m.chooser.sel != 1 {
		t.Errorf("ctrl+n left the cursor on %d", m.chooser.sel)
	}
	press(m, "esc")
	if m.chooser != nil {
		t.Error("esc did not close the chooser")
	}
}

// A paste goes only into a text box with the focus: out of the text box of
// a review Claude drafted, it reaches neither the review nor the question
// box of the chat under it.
func TestPasteOutOfTheComposeTextBox(t *testing.T) {
	m, _ := startWith(t, fixture(), keymap.Default())
	fakeClaudeOf(t, m)
	press(m, "i", "o", "n", "e", "enter")
	c := m.chat()
	press(m, "ctrl+x")
	if m.compose == nil {
		t.Fatal("ctrl+x did not open the drafted review")
	}
	review := m.compose.ta.Value()
	press(m, "esc")
	run(m, m.paste(tea.PasteMsg{Content: "pasted"}))
	if strings.Contains(c.input.Value(), "pasted") || m.compose.ta.Value() != review {
		t.Errorf("the paste went in: chat %q, review %q", c.input.Value(), m.compose.ta.Value())
	}
}

// While a popup is open, the key line holds only its most important keys,
// the writes, and what esc does there; the rest are in its frame (ADR-125).
func TestKeyLineUnderAPopup(t *testing.T) {
	cases := []struct {
		name string
		open func(*testing.T) *Model
		want string
	}{
		{"menu", func(t *testing.T) *Model {
			m, _ := startWith(t, fixture(), keymap.Default())
			press(m, "enter")
			return m
		}, "esc close"},
		{"confirm", func(t *testing.T) *Model {
			m, _ := startWith(t, fixture(), keymap.Default())
			press(m, "A")
			if m.bulk == nil || m.bulk.loading {
				t.Fatalf("A did not open a confirm popup that was read: %+v", m.bulk)
			}
			return m
		}, "y yes · esc no"},
		{"compose", func(t *testing.T) *Model {
			m, _ := startWith(t, fixture(), keymap.Default())
			press(m, "R")
			return m
		}, "ctrl+s post · esc done writing"},
		{"compose out of its text box", func(t *testing.T) *Model {
			m, _ := startWith(t, fixture(), keymap.Default())
			press(m, "R", "esc")
			return m
		}, "ctrl+s post · esc discard"},
		{"? list", func(t *testing.T) *Model {
			m, _ := startWith(t, fixture(), keymap.Default())
			press(m, "?")
			return m
		}, "esc close"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := c.open(t)
			if got := ansi.Strip(m.hints()); got != c.want {
				t.Errorf("key line %q, want %q", got, c.want)
			}
		})
	}
}
