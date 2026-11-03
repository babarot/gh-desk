package tui

import (
	"slices"
	"strconv"
	"testing"

	"github.com/babarot/gh-triage/internal/tui/keymap"
)

// d on a selection puts away every selected pull request of the tab, and
// they leave the selection, so a later batch does not take what is out of
// sight.
func TestHideSelection(t *testing.T) {
	m := start(t, fixture(), keymap.Default())
	press(m, "b", "V")
	n := len(m.selected)
	if n < 2 {
		t.Fatalf("selected = %v", m.selected)
	}
	press(m, "d")
	if len(m.selected) != 0 {
		t.Errorf("hidden pull requests stayed selected: %v", m.selected)
	}
	if len(m.tab().hidden) != n {
		t.Errorf("hidden = %v, want %d", m.tab().hidden, n)
	}
	press(m, "M")
	if m.bulk != nil {
		for _, it := range m.bulk.items {
			if m.tab().hidden[it.pr.Key] {
				t.Errorf("a merge took %s, which was hidden", it.pr.Key)
			}
		}
	}
}

// In the inbox, d on a selection marks each notification done.
func TestDoneSelection(t *testing.T) {
	f := fixture()
	m := start(t, f, keymap.Default())
	press(m, "4", "a", "V", "d")
	if len(f.done) != 2 {
		t.Fatalf("done = %v", f.done)
	}
	if len(m.selected) != 0 {
		t.Errorf("selection = %v", m.selected)
	}
}

// What is selected in another tab stays selected when d puts away this
// tab's.
func TestHideSelectionKeepsOtherTabs(t *testing.T) {
	f := fixture()
	m := start(t, f, keymap.Default())
	press(m, "b", "V")
	n := len(m.selected)
	if !m.isSelected(key("acme", "api", 12)) {
		t.Fatalf("selected = %v", m.selected)
	}
	press(m, "4", "d")
	if len(f.done) != 1 || f.done[0] != "acme/api#12" {
		t.Fatalf("done = %v", f.done)
	}
	if len(m.selected) != n-1 || m.isSelected(key("acme", "api", 12)) {
		t.Errorf("selection = %v, want the other %d", m.selected, n-1)
	}
}

// o on a selection opens each selected pull request.
func TestBrowseSelection(t *testing.T) {
	f := fixture()
	for _, prs := range f.searches {
		for i := range prs {
			prs[i].URL = "https://github.com/" + prs[i].Key.Owner + "/" + prs[i].Key.Repo + "/pull/" + strconv.Itoa(prs[i].Key.Number)
		}
	}
	m := start(t, f, keymap.Default())
	press(m, "b", "V")
	var want []string
	for _, p := range m.menuTargets() {
		if p.URL != "" {
			want = append(want, p.URL)
		}
	}
	if len(want) < 2 {
		t.Fatalf("selected = %v", m.selected)
	}
	press(m, "o")
	if !slices.Equal(f.browsed, want) {
		t.Errorf("browsed %v, want %v", f.browsed, want)
	}
}

// i talks about one pull request; with a selection it is refused.
func TestAskClaudeRefusesSelection(t *testing.T) {
	m := start(t, fixture(), keymap.Default())
	press(m, "V", "i")
	if len(m.chats) != 0 {
		t.Error("a chat opened on a selection")
	}
}
