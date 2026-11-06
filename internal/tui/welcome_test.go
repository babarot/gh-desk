package tui

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/babarot/gh-desk/internal/claude"
	"github.com/babarot/gh-desk/internal/tui/keymap"
)

// The claude pane with nothing asked shows the welcome, with the
// questions tab puts in the text box one after another; asking ends it.
func TestClaudeWelcome(t *testing.T) {
	m, _ := startWith(t, fixture(), keymap.Default())
	m.Update(tea.WindowSizeMsg{Width: 130, Height: 60})
	m.pane.height = 30
	press(m, "i")
	s := screen(m)
	for _, want := range []string{"▐▛███▜▌", "acme/api#12  Add retries to the client", "alice wants to merge", "worktree ready at", "try"} {
		if !strings.Contains(s, want) {
			t.Errorf("welcome lacks %q:\n%s", want, s)
		}
	}
	qs := claude.Questions()
	for _, q := range qs {
		if !strings.Contains(s, q) {
			t.Errorf("welcome lacks the question %q", q)
		}
	}
	c := m.chat()
	for i := range len(qs) + 1 {
		press(m, "tab")
		if got, want := c.input.Value(), qs[(i)%len(qs)]; got != want {
			t.Fatalf("tab %d put %q, want %q", i+1, got, want)
		}
	}
	if !strings.Contains(screen(m), "› "+qs[0]) {
		t.Errorf("the question in the text box is not marked:\n%s", screen(m))
	}
	// Text of the user's own is not replaced.
	c.input.SetValue("why?")
	press(m, "tab")
	if c.input.Value() != "why?" {
		t.Errorf("tab replaced the user's text with %q", c.input.Value())
	}
	press(m, "enter")
	if s := screen(m); strings.Contains(s, "▐▛███▜▌") || strings.Contains(s, qs[2]) {
		t.Errorf("the welcome stays after a question:\n%s", s)
	}
}

// The welcome gives way to fit the pane: the questions first, and all of
// it when even the pull request does not fit.
func TestClaudeWelcomeFits(t *testing.T) {
	m, _ := startWith(t, fixture(), keymap.Default())
	press(m, "i")
	c := m.chat()
	full := m.welcomeLines(c, 100)
	for h := 0; h <= len(full); h++ {
		L := m.welcomeLines(c, h)
		if len(L) > h {
			t.Errorf("height %d: %d lines", h, len(L))
		}
		if h < 4 && L != nil {
			t.Errorf("height %d: shown without room for the pull request", h)
		}
	}
	c.preparing = true
	if s := strings.Join(m.welcomeLines(c, 100), "\n"); !strings.Contains(s, "making the worktree") {
		t.Errorf("no worktree being made:\n%s", s)
	}
}

// A worktree that could not be made ahead is told in the welcome, and
// opening the conversation again tries again.
func TestClaudeWelcomeWorktreeFailed(t *testing.T) {
	m, _ := startWith(t, fixture(), keymap.Default())
	fc := fakeClaudeOf(t, m)
	fc.prepareErr = errors.New("git fetch: could not read from remote\nmore")
	press(m, "i")
	s := screen(m)
	if !strings.Contains(s, "✗ worktree not made: git fetch: could not read from remote · the first question tries again") || strings.Contains(s, "more") {
		t.Errorf("the failure is not told on one line:\n%s", s)
	}
	fc.prepareErr = nil
	press(m, "esc", "i")
	if s := screen(m); !strings.Contains(s, "worktree ready at") || strings.Contains(s, "not made") {
		t.Errorf("opening again does not try again:\n%s", s)
	}
}

// Clawd is drawn in Claude's color on every theme (ADR-081).
func TestMascotKeepsClaudesColor(t *testing.T) {
	for name, p := range themes {
		if got := newStyles(p).mascot.GetForeground(); got != claudeOrange() {
			t.Errorf("theme %s draws Clawd in %v", name, got)
		}
	}
}
