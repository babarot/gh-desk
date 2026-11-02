package tui

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/babarot/gh-triage/internal/keymap"
	"github.com/babarot/gh-triage/internal/model"
	"github.com/babarot/gh-triage/internal/runner"
	"github.com/babarot/gh-triage/internal/tui/chat"
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
	press(m1, "i")
	press(m1, "i", "w", "h", "y", "?", "enter")
	if len(fc1.asked) != 2 {
		t.Fatalf("asked %d", len(fc1.asked))
	}
	k := m1.chatKey
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
	if s := screen(m2); !strings.Contains(s, "Answer 2") || !strings.Contains(s, "why?") {
		t.Errorf("the conversation kept is not shown:\n%s", s)
	}
	press(m2, "i", "m", "o", "r", "e", "enter")
	if len(fc2.asked) != 1 {
		t.Fatalf("asked %d", len(fc2.asked))
	}
	if r := fc2.asked[0]; r.First || r.Session != fc1.asked[0].Session || r.Prompt != "more" {
		t.Errorf("request %+v; want the session kept resumed", r)
	}

	// The first gh-triage reads what the second one added.
	press(m1, "esc", "i")
	if s := screen(m1); !strings.Contains(s, "more") {
		t.Errorf("the other gh-triage's question is not shown:\n%s", s)
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
	press(m, "i", "a", "g", "a", "i", "n", "enter")
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
	press(m, "i", "x", "enter")
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
	press(m, "i", "a", "n", "d", "?", "enter")
	if len(fc.asked) != 1 {
		t.Fatalf("asked %d", len(fc.asked))
	}
	r := fc.asked[0]
	if r.First || r.Session != "s1" || !strings.Contains(r.Prompt, "moved from 0000old to aaaaaaa1") || !strings.HasSuffix(r.Prompt, "and?") {
		t.Errorf("request %+v", r)
	}
	if s := screen(m); !strings.Contains(s, "head moved 0000old → aaaaaaa") {
		t.Errorf("no mark where the head moved:\n%s", s)
	}
}

func TestConversationStartedOver(t *testing.T) {
	chats := newFakeChats()
	m, fc := startKept(t, chats)
	press(m, "i")
	k := m.chatKey
	before := chats.recs[k].Rev
	press(m, "N")
	if len(fc.asked) != 1 {
		t.Fatalf("started over at the first press")
	}
	press(m, "N")
	if len(fc.asked) != 2 {
		t.Fatalf("asked %d", len(fc.asked))
	}
	r := chats.recs[k]
	if a := fc.asked[1]; !a.First || a.Session == fc.asked[0].Session {
		t.Errorf("request %+v; want a new session", a)
	}
	if r.Rev <= before || len(r.Turns) != 2 || r.Session != fc.asked[1].Session {
		t.Errorf("kept %+v; want the new conversation, its rev past %d", r, before)
	}
}

func TestConversationNotKeptGoesOn(t *testing.T) {
	chats := newFakeChats()
	chats.saveErr = errors.New("read-only file system")
	m, fc := startKept(t, chats)
	press(m, "i")
	press(m, "esc", "i")
	if len(fc.asked) != 1 || !strings.Contains(screen(m), "Answer 1") {
		t.Errorf("the conversation did not go on in memory; asked %d:\n%s", len(fc.asked), screen(m))
	}
}

func TestConversationRemovedByGC(t *testing.T) {
	chats := newFakeChats()
	m, fc := startKept(t, chats)
	press(m, "i")
	k := m.chatKey
	press(m, "esc")
	delete(chats.recs, k)
	press(m, "i")
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
	press(m1, "i")
	m2, _ := startKept(t, chats)
	press(m2, "i", "i", "o", "n", "e", "enter")
	press(m1, "i", "t", "w", "o", "enter")
	if len(fc1.asked) != 2 {
		t.Fatalf("asked %d", len(fc1.asked))
	}
	var asked []string
	for _, tr := range chats.recs[m1.chatKey].Turns {
		if tr.You {
			asked = append(asked, tr.Text)
		}
	}
	if strings.Join(asked, ",") != "Review this pull request.,one,two" {
		t.Errorf("questions kept %v", asked)
	}
}
