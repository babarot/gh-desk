package tui

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/babarot/gh-triage/internal/model"
	"github.com/babarot/gh-triage/internal/runner"
	"github.com/babarot/gh-triage/internal/tui/chat"
	"github.com/babarot/gh-triage/internal/tui/keymap"
)

// fakeChats keeps conversations in memory. Two Models given one stand for
// two gh-triage sharing the state directory.
type fakeChats struct {
	recs map[model.PRKey]chat.Record
	// held names who holds each lock.
	held    map[model.PRKey]string
	saveErr error
	saves   int
}

func newFakeChats() *fakeChats {
	return &fakeChats{recs: map[model.PRKey]chat.Record{}, held: map[model.PRKey]string{}}
}

func (f *fakeChats) Load(k model.PRKey) (chat.Record, bool, error) {
	r, ok := f.recs[k]
	return r, ok, nil
}

func (f *fakeChats) Save(r chat.Record) error {
	if f.saveErr != nil {
		return f.saveErr
	}
	f.saves++
	f.recs[r.Key] = r
	return nil
}

func (f *fakeChats) Keys() ([]model.PRKey, error) {
	var out []model.PRKey
	for k := range f.recs {
		out = append(out, k)
	}
	return out, nil
}

func (f *fakeChats) Lock(k model.PRKey) (func(), bool, string, error) {
	if who := f.held[k]; who != "" {
		return nil, false, who, nil
	}
	f.held[k] = "pid 1 in /here"
	return func() { delete(f.held, k) }, true, "", nil
}

// chatText is the whole conversation in the claude pane, unstyled, as it
// scrolls.
func chatText(m *Model) string { return stripped(m.chatLines(m.chat(), m.paneInner())) }

func startKept(t *testing.T, chats *fakeChats) (*Model, *fakeClaude) {
	t.Helper()
	set := testSettings()
	set.Chats = chats
	fc := &fakeClaude{}
	m := New(fixture(), &fakeRunner{results: map[model.PRKey]runner.Event{}}, fc, keymap.Default(), nil, set)
	m.Update(tea.WindowSizeMsg{Width: 130, Height: 34})
	run(m, m.Init())
	return m, fc
}

func TestConversationGoesOnInAnotherLaunch(t *testing.T) {
	chats := newFakeChats()
	m1, fc1 := startKept(t, chats)
	press(m1, "i", "tab", "enter")
	press(m1, "w", "h", "y", "?", "enter")
	if len(fc1.asked) != 2 {
		t.Fatalf("asked %d", len(fc1.asked))
	}
	k := m1.chat().PR().Key
	if r := chats.recs[k]; len(r.Turns) != 4 || !r.Started || r.Session != fc1.asked[0].Session || r.Rev == 0 {
		t.Fatalf("kept %+v", r)
	}
	if len(chats.held) != 0 {
		t.Fatalf("locks left held: %v", chats.held)
	}

	m2, fc2 := startKept(t, chats)
	press(m2, "i")
	if len(fc2.asked) != 0 {
		t.Fatalf("a kept conversation was asked again: %+v", fc2.asked)
	}
	if s := chatText(m2); !strings.Contains(s, "Answer 2") || !strings.Contains(s, "why?") {
		t.Errorf("the conversation kept is not shown:\n%s", s)
	}
	press(m2, "m", "o", "r", "e", "enter")
	if len(fc2.asked) != 1 {
		t.Fatalf("asked %d", len(fc2.asked))
	}
	if r := fc2.asked[0]; r.First || r.Session != fc1.asked[0].Session || r.Prompt != "more" {
		t.Errorf("request %+v; want the session kept resumed", r)
	}

	// The first gh-triage reads what the second one added.
	press(m1, "esc", "i")
	if s := chatText(m1); !strings.Contains(s, "more") {
		t.Errorf("the other gh-triage's question is not shown:\n%s", s)
	}
}

// Opened with nothing asked, a conversation makes its worktree under its
// lock and frees it; the first question typed carries the pull request.
func TestConversationOpensWithNothingAsked(t *testing.T) {
	chats := newFakeChats()
	m, fc := startKept(t, chats)
	press(m, "i")
	if len(fc.asked) != 0 || len(fc.prepared) != 1 || len(chats.held) != 0 {
		t.Fatalf("asked %d, prepared %d, held %v", len(fc.asked), len(fc.prepared), chats.held)
	}
	press(m, "w", "h", "y", "?", "enter")
	if len(fc.asked) != 1 {
		t.Fatalf("asked %d", len(fc.asked))
	}
	if r := fc.asked[0]; !r.First || !strings.Contains(r.Prompt, "<diff>") || !strings.HasSuffix(r.Prompt, "\nwhy?") || strings.Contains(r.Prompt, "Review this pull request.") {
		t.Errorf("first request %+v", r)
	}
	if r := chats.recs[m.chat().PR().Key]; len(r.Turns) != 2 || r.Turns[0].Text != "why?" {
		t.Errorf("kept %+v", r)
	}
}

// Claude Code in the terminal, refused while the worktree is made, leaves
// the lock to it.
func TestConversationLockKeptWhilePrepared(t *testing.T) {
	chats := newFakeChats()
	m, _ := startKept(t, chats)
	press(m, "i")
	k := m.chat().PR().Key
	c := m.chat()
	// The worktree is still being made: its message has not come.
	if !m.chatState.Lock(c.Conversation) {
		t.Fatal("could not take the lock")
	}
	c.preparing = true
	press(m, "ctrl+o")
	if chats.held[k] == "" {
		t.Fatal("the lock was freed while the worktree is made")
	}
	m.Update(chatReadyMsg{key: k, dir: "/cache/worktrees/" + k.String()})
	if chats.held[k] != "" {
		t.Error("the lock is held after the worktree was made")
	}
}

func TestConversationCutShort(t *testing.T) {
	chats := newFakeChats()
	k := key("acme", "api", 12)
	chats.recs[k] = chat.Record{Key: k, Session: "taken", Dir: "/cache/worktrees/acme/api#12", Head: "aaaaaaa1", Rev: 1,
		Turns: []chat.Turn{{You: true, Text: "Review this pull request.", Head: "aaaaaaa1"}}}
	m, fc := startKept(t, chats)
	press(m, "i")
	if len(fc.asked) != 0 {
		t.Fatalf("asked %d", len(fc.asked))
	}
	if s := screen(m); !strings.Contains(s, "interrupted: gh-triage was closed before the answer") {
		t.Errorf("no interrupted:\n%s", s)
	}
	// A session that never answered is not used again: Claude Code may
	// have taken its ID.
	press(m, "a", "g", "a", "i", "n", "enter")
	if len(fc.asked) != 1 {
		t.Fatalf("asked %d", len(fc.asked))
	}
	if r := fc.asked[0]; !r.First || r.Session == "taken" || r.Session == "" {
		t.Errorf("request %+v; want a new session", r)
	}
}

func TestConversationWorkedOnElsewhere(t *testing.T) {
	chats := newFakeChats()
	k := key("acme", "api", 12)
	chats.recs[k] = chat.Record{Key: k, Session: "s1", Dir: "/cache/worktrees/acme/api#12", Head: "aaaaaaa1", Rev: 1,
		Turns: []chat.Turn{{You: true, Text: "Review this pull request.", Head: "aaaaaaa1"}}}
	chats.held[k] = "pid 42 in /there"
	m, fc := startKept(t, chats)
	press(m, "i")
	if len(fc.asked) != 0 {
		t.Fatalf("asked %d", len(fc.asked))
	}
	if s := screen(m); !strings.Contains(s, "answering in another gh-triage (pid 42 in /there)") || strings.Contains(s, "interrupted") {
		t.Errorf("not shown as answered elsewhere:\n%s", s)
	}
	press(m, "x", "enter")
	if len(fc.asked) != 0 {
		t.Fatalf("asked while another gh-triage answers")
	}

	// The other one answers and lets go; the screen reads it again.
	r := chats.recs[k]
	r.Started, r.Rev = true, 2
	r.Turns = append(r.Turns, chat.Turn{Text: "Answered over there."})
	chats.recs[k] = r
	delete(chats.held, k)
	// A second later, as the spinner turns.
	m.set.Now = func() time.Time { return now.Add(watchEvery) }
	m.watchChat()
	if s := screen(m); !strings.Contains(s, "Answered over there.") || strings.Contains(s, "answering in another") {
		t.Errorf("the answer is not shown:\n%s", s)
	}
}

func TestConversationAfterTheHeadMoved(t *testing.T) {
	chats := newFakeChats()
	k := key("acme", "api", 12)
	chats.recs[k] = chat.Record{Key: k, Session: "s1", Started: true, Dir: "/cache/worktrees/acme/api#12", Head: "0000old", Rev: 2,
		Turns: []chat.Turn{{You: true, Text: "Review this pull request.", Head: "0000old"}, {Text: "Fine."}}}
	m, fc := startKept(t, chats)
	press(m, "i")
	if s := screen(m); !strings.Contains(s, "(was 0000old)") {
		t.Errorf("the head kept is not shown:\n%s", s)
	}
	press(m, "a", "n", "d", "?", "enter")
	if len(fc.asked) != 1 {
		t.Fatalf("asked %d", len(fc.asked))
	}
	r := fc.asked[0]
	if r.First || r.Session != "s1" || !strings.Contains(r.Prompt, "moved from 0000old to aaaaaaa1") || !strings.HasSuffix(r.Prompt, "and?") {
		t.Errorf("request %+v", r)
	}
	if s := chatText(m); !strings.Contains(s, "head moved 0000old → aaaaaaa") {
		t.Errorf("no mark where the head moved:\n%s", s)
	}
}

func TestConversationStartedOver(t *testing.T) {
	chats := newFakeChats()
	m, fc := startKept(t, chats)
	press(m, "i", "tab", "enter")
	k := m.chat().PR().Key
	before := chats.recs[k].Rev
	press(m, "ctrl+l")
	if len(chats.recs[k].Turns) == 0 {
		t.Fatalf("started over at the first press")
	}
	// Started over, nothing is asked, and the lock is free.
	press(m, "ctrl+l")
	r := chats.recs[k]
	if len(fc.asked) != 1 || r.Rev <= before || len(r.Turns) != 0 || r.Session != "" || len(chats.held) != 0 {
		t.Fatalf("asked %d, kept %+v, held %v; want an empty conversation, its rev past %d", len(fc.asked), r, chats.held, before)
	}
	press(m, "tab", "enter")
	if a := fc.asked[1]; !a.First || a.Session == fc.asked[0].Session {
		t.Errorf("request %+v; want a new session", a)
	}
}

func TestConversationNotKeptGoesOn(t *testing.T) {
	chats := newFakeChats()
	chats.saveErr = errors.New("read-only file system")
	m, fc := startKept(t, chats)
	press(m, "i", "tab", "enter")
	press(m, "esc", "i")
	if len(fc.asked) != 1 || !strings.Contains(screen(m), "Answer 1") {
		t.Errorf("the conversation did not go on in memory; asked %d:\n%s", len(fc.asked), screen(m))
	}
}

func TestConversationRemovedByGC(t *testing.T) {
	chats := newFakeChats()
	m, fc := startKept(t, chats)
	press(m, "i", "tab", "enter")
	k := m.chat().PR().Key
	press(m, "esc")
	delete(chats.recs, k)
	press(m, "i", "tab", "enter")
	if len(fc.asked) != 2 || !fc.asked[1].First {
		t.Errorf("a conversation gc removed was not started anew: %+v", fc.asked)
	}
}

func TestConversationKeptInThePane(t *testing.T) {
	chats := newFakeChats()
	k := key("acme", "api", 12)
	chats.recs[k] = chat.Record{Key: k, Session: "s1", Started: true, Head: "aaaaaaa1", Rev: 2,
		Turns: []chat.Turn{{You: true, Text: "Review this pull request."}, {Text: "Fine."}}}
	m, _ := startKept(t, chats)
	if s := screen(m); !strings.Contains(s, "1 question · at aaaaaaa") {
		t.Errorf("the pane does not say a conversation is kept:\n%s", s)
	}
}

// A question asked in a conversation left open reads first what another
// gh-triage added meanwhile, and writes after it.
func TestConversationAskedAfterAnotherAdded(t *testing.T) {
	chats := newFakeChats()
	m1, fc1 := startKept(t, chats)
	press(m1, "i", "tab", "enter")
	m2, _ := startKept(t, chats)
	press(m2, "i", "o", "n", "e", "enter")
	press(m1, "t", "w", "o", "enter")
	if len(fc1.asked) != 2 {
		t.Fatalf("asked %d", len(fc1.asked))
	}
	var asked []string
	for _, tr := range chats.recs[m1.chat().PR().Key].Turns {
		if tr.You {
			asked = append(asked, tr.Text)
		}
	}
	if strings.Join(asked, ",") != "Review this pull request.,one,two" {
		t.Errorf("questions kept %v", asked)
	}
}
