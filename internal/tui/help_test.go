package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// The ? popup tells the keys of the screen it was opened from first, folds
// the other scopes, and takes about two thirds of the terminal, leaving
// the screen around it.
func TestHelpTellsTheScreenItWasOpenedFrom(t *testing.T) {
	m := newModel(fixture(), cachedReview())
	run(m, m.Init())
	press(m, "?")
	s := screen(m)
	for _, want := range []string{"the list and its preview   here", "select by rule", "↓ "} {
		if !strings.Contains(s, want) {
			t.Errorf("the list's ? popup does not tell %q:\n%s", want, s)
		}
	}
	if n := len(m.renderHelp()); n > 34*2/3+2 {
		t.Errorf("the ? popup is %d lines tall in a terminal of 34", n)
	}
	if !strings.Contains(s, "triage") {
		t.Errorf("the ? popup covers the header:\n%s", s)
	}

	press(m, "esc", "enter", "?")
	if s := ansiStrip(m.helpLines()); !strings.HasPrefix(s, "a pull request   here") || !strings.Contains(s, "the list and its preview · ") {
		t.Errorf("the PR screen's ? popup does not put its own keys first:\n%s", s)
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
	if !strings.Contains(s, "writing a review · 3") || strings.Contains(s, "↓ ") {
		t.Errorf("G did not scroll to the end:\n%s", s)
	}
	end := m.helpTop
	press(m, "j")
	if m.helpTop != end {
		t.Errorf("j scrolled past the end: %d, want %d", m.helpTop, end)
	}
	mouseAt(m, tea.MouseWheelMsg{X: 10, Y: 10, Button: tea.MouseWheelUp})
	if m.helpTop != end-wheelLines {
		t.Errorf("the wheel scrolled to %d, want %d", m.helpTop, end-wheelLines)
	}
	press(m, "?", "?")
	if !m.help || m.helpTop != 0 {
		t.Errorf("? again opens the popup at %d, want its top", m.helpTop)
	}
}

func ansiStrip(lines []string) string {
	return ansi.Strip(strings.Join(lines, "\n"))
}
