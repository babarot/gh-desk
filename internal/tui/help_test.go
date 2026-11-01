package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// The ? popup tells the keys of the screen it was opened from, folds the
// other scopes, and fits a terminal of 34 lines, leaving the screen around
// it.
func TestHelpTellsTheScreenItWasOpenedFrom(t *testing.T) {
	m := newModel(fixture(), cachedReview())
	run(m, m.Init())
	press(m, "?")
	s := screen(m)
	for _, want := range []string{"the list and its preview   here", "select by rule", "a pull request · 8", "anywhere", "moving", "mouse"} {
		if !strings.Contains(s, want) {
			t.Errorf("the list's ? popup does not tell %q:\n%s", want, s)
		}
	}
	if strings.Contains(s, "taller terminal") {
		t.Errorf("the list's ? popup does not fit 34 lines:\n%s", s)
	}
	if !strings.Contains(s, "triage") {
		t.Errorf("the ? popup covers the header:\n%s", s)
	}

	press(m, "esc", "enter", "?")
	s = screen(m)
	if !strings.Contains(s, "a pull request   here") || !strings.Contains(s, "the list and its preview · ") {
		t.Errorf("the PR screen's ? popup does not put its own keys first:\n%s", s)
	}
}

// A narrow terminal gets one column; what does not fit is told.
func TestHelpInOneColumn(t *testing.T) {
	m := newModel(fixture(), cachedReview())
	run(m, m.Init())
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 34})
	press(m, "?")
	s := screen(m)
	if !strings.Contains(s, "  tab             next tab") || !strings.Contains(s, "a taller terminal shows the rest") {
		t.Errorf("the ? popup is not one column, cut at the bottom:\n%s", s)
	}
}
