package tui

import (
	"slices"
	"testing"

	"github.com/babarot/gh-triage/internal/runner"
	"github.com/babarot/gh-triage/internal/tui/keymap"
)

// The menu offers the actions the config file names, in its order; one
// left out is not offered, and its key does nothing in the menu.
func TestMenuInTheOrderGiven(t *testing.T) {
	set := testSettings()
	set.Menu = []string{"merge", "approve", "browse"}
	m := startSet(t, set, 130)
	press(m, "enter")
	var got []keymap.Action
	for _, it := range m.menuItems() {
		got = append(got, it.action)
	}
	if want := []keymap.Action{"merge", "approve", "browse"}; !slices.Equal(got, want) {
		t.Fatalf("menu = %q, want %q", got, want)
	}
	press(m, "c")
	if m.menu == nil || m.compose != nil {
		t.Fatalf("c ran comment, which the menu does not offer: menu %v, compose %v", m.menu, m.compose)
	}
	press(m, "enter")
	if m.menu != nil || m.bulk == nil || m.bulk.kind != runner.Merge {
		t.Fatalf("enter on the first did not open the merge popup: menu %v, bulk %v", m.menu, m.bulk)
	}
}
