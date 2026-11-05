package tui

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/babarot/gh-desk/internal/claude"
	"github.com/babarot/gh-desk/internal/tui/keymap"
)

// notesWith counts the notifications whose title has s.
func notesWith(m *Model, s string) int {
	n := 0
	for _, x := range m.notes {
		if strings.Contains(x.title, s) {
			n++
		}
	}
	return n
}

// What Claude leaves while the pane does not show it is kept in the
// notifications (ADR-007); shown, the conversation says it, and nothing is
// kept.
func TestClaudeElsewhereNoted(t *testing.T) {
	m, _ := startWith(t, fixture(), keymap.Default())
	press(m, "i")
	k := m.chat().PR().Key
	m.Update(chatAnswerMsg{key: k, answer: claude.Answer{Text: "Seen here."}})
	m.Update(chatAnswerMsg{key: k, err: errors.New("claude exited 1")})
	if len(m.notes) != 0 {
		t.Fatalf("what the pane showed was kept: %+v", m.notes)
	}
	press(m, "esc")
	m.Update(chatAnswerMsg{key: k, err: errors.New("claude exited 1")})
	if len(m.notes) != 1 || m.notes[0].icon != "✗" || !strings.Contains(m.notes[0].title, "Claude failed on "+k.String()) ||
		m.notes[0].items[0].key != k {
		t.Fatalf("a failure elsewhere is not kept: %+v", m.notes)
	}
	m.Update(chatAnswerMsg{key: k, answer: claude.Answer{Text: "{}"}, draft: true})
	if notesWith(m, "Claude failed on") != 2 {
		t.Errorf("a draft that could not be read is not kept: %+v", m.notes)
	}
}

// A conversation that cannot be kept says so in the notifications, as
// keeping it fails when an answer comes; the toast's timer is sent.
func TestConversationNotKeptNoted(t *testing.T) {
	chats := newFakeChats()
	chats.saveErr = errors.New("read-only file system")
	m, _ := startKept(t, chats)
	press(m, "i", "tab", "enter")
	if notesWith(m, "could not keep the conversation of acme/api#12") != 1 || m.notes[0].items[0].key != key("acme", "api", 12) {
		t.Errorf("notes = %+v", m.notes)
	}
	if len(m.noted) != 0 {
		t.Errorf("the toast's timer was not sent: %d left", len(m.noted))
	}
}

// A comment sent to every pull request is kept, as its failure is.
func TestCommentSentNoted(t *testing.T) {
	m := start(t, fixture(), keymap.Default())
	press(m, "c", "tab")
	run(m, m.paste(tea.PasteMsg{Content: "Does this retry on a 429?"}))
	press(m, "ctrl+s", "y")
	if notesWith(m, "comment sent to") != 1 || m.notes[0].icon != "✓" || m.notes[0].items[0].key != key("acme", "api", 12) {
		t.Errorf("notes = %+v", m.notes)
	}
}

// Descriptions that keep failing to be read are kept once, and again only
// after a reading that went through, as a tab's reading is.
func TestBodiesFailureNotedOnce(t *testing.T) {
	m := start(t, fixture(), keymap.Default())
	fail := bodiesMsg{err: errString("secondary rate limit"), tab: m.cur}
	m.Update(fail)
	m.Update(fail)
	if notesWith(m, "could not read every description in review") != 1 {
		t.Fatalf("notes = %+v, want 1", m.notes)
	}
	m.Update(bodiesMsg{tab: m.cur})
	m.Update(fail)
	if notesWith(m, "could not read every description") != 2 {
		t.Errorf("a failure after a reading that went through was not kept: %+v", m.notes)
	}
}
