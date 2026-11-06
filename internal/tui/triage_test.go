package tui

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/babarot/gh-desk/internal/claude"
	"github.com/babarot/gh-desk/internal/model"
	"github.com/babarot/gh-desk/internal/runner"
	"github.com/babarot/gh-desk/internal/tui/askbox"
	"github.com/babarot/gh-desk/internal/tui/keymap"
)

// startTriage starts the TUI with Claude answering plan, and asks it on
// the tab pressed, or on the first.
func startTriage(t *testing.T, plan string, tab ...string) (*Model, *fakeClaude) {
	t.Helper()
	m, _ := startWith(t, fixture(), keymap.Default())
	fc := fakeClaudeOf(t, m)
	fc.plan = plan
	press(m, tab...)
	press(m, "C", "b", "o", "t", "s", "enter")
	if len(fc.triaged) != 1 {
		t.Fatalf("asked %d times", len(fc.triaged))
	}
	return m, fc
}

// A triage with nothing asked shows Clawd beside what the tab holds, with
// the questions tab fills in; the questions give way first on a low screen.
func TestTriageWelcome(t *testing.T) {
	m, _ := startWith(t, fixture(), keymap.Default())
	fakeClaudeOf(t, m)
	press(m, "C")
	golden(t, "triage-welcome", m)
	press(m, "tab")
	if got := m.triage.input.Value(); got != claude.TriageQuestions()[0] {
		t.Errorf("tab filled %q", got)
	}
	press(m, "tab")
	if got := m.triage.input.Value(); got != claude.TriageQuestions()[1] {
		t.Errorf("a second tab filled %q", got)
	}
	press(m, "x", "tab")
	if got := m.triage.input.Value(); got != claude.TriageQuestions()[1]+"x" {
		t.Errorf("tab changed the user's own text to %q", got)
	}
	m.Update(tea.WindowSizeMsg{Width: 130, Height: 18})
	if s := screen(m); !strings.Contains(s, "▐▛") || strings.Contains(s, "what can I approve") {
		t.Errorf("a low screen:\n%s", s)
	}
	m.Update(tea.WindowSizeMsg{Width: 130, Height: 12})
	if s := screen(m); strings.Contains(s, "▐▛") {
		t.Errorf("a screen too low for Clawd:\n%s", s)
	}
}

// A reply is drawn as the claude pane draws one, with its model and cost.
func TestTriageAnswerDrawnAsAChat(t *testing.T) {
	m, fc := startTriage(t, `{"reply":"**One** bot.","actions":[]}`)
	s := screen(m)
	if !strings.Contains(s, "> bots") || !strings.Contains(s, "⏺ One bot.") || strings.Contains(s, "**") {
		t.Errorf("reply:\n%s", s)
	}
	fc.triageCost, fc.triageModel = 0.25, "claude-opus-5-5"
	press(m, "i", "m", "o", "r", "e", "enter")
	s = screen(m)
	if !strings.Contains(s, "$0.25") || !strings.Contains(s, "claude-opus-5-5 · plan only") {
		t.Errorf("no model or cost:\n%s", s)
	}
}

func TestTriagePlanApplied(t *testing.T) {
	m, fc := startTriage(t, `{"reply":"Picked the bot update; api#99 is not in this tab.","actions":[`+
		`{"do":"unfold","group":"bots"},{"do":"select","keys":["acme/api#30","acme/api#99"]},`+
		`{"do":"cursor","key":"acme/api#30"},{"do":"approve","keys":["acme/api#30"]}]}`)
	r := fc.triaged[0]
	if !r.First || !strings.Contains(r.Prompt, "<request>\nbots\n</request>") || !strings.Contains(r.Prompt, "acme/api#30 head=ccccccc group=bots") {
		t.Errorf("request %+v", r)
	}
	golden(t, "triage-plan", m)
	if len(m.selected) != 0 || m.bulk != nil {
		t.Fatal("the plan was applied before enter")
	}

	press(m, "enter")
	if m.triage.open || m.selected[key("acme", "api", 30)] != byClaude || len(m.selected) != 1 || m.tab().folded["bots"] {
		t.Fatalf("not applied: open %v, selected %v", m.triage.open, m.selected)
	}
	if p, _, _ := m.currentPR(); p.Key != key("acme", "api", 30) {
		t.Errorf("cursor on %v", p.Key)
	}
	if m.bulk == nil || m.bulk.kind != runner.Review || len(m.bulk.items) != 1 || m.bulk.by != "bots" {
		t.Fatalf("bulk = %+v", m.bulk)
	}
	golden(t, "triage-confirm", m)

	// The next request goes on in the same session.
	press(m, "esc", "C", "m", "o", "r", "e", "enter")
	if len(fc.triaged) != 2 || fc.triaged[1].First || fc.triaged[1].Session != r.Session {
		t.Errorf("second request %+v after %+v", fc.triaged[1], r)
	}
}

// An approve whose pull requests are all left out is left out too: it
// does not fall back on the selection or the cursor.
func TestTriageWriteNeedsItsOwnPullRequests(t *testing.T) {
	m, _ := startWith(t, fixture(), keymap.Default())
	fc := fakeClaudeOf(t, m)
	fc.plan = `{"reply":"Approving.","actions":[{"do":"select","keys":["acme/api#99"]},{"do":"approve","keys":["acme/api#99"]}]}`
	press(m, "space", "C", "g", "o", "enter")
	if s := screen(m); strings.Count(s, "✗ acme/api#99 not in this tab") != 2 || strings.Contains(s, "enter apply") {
		t.Errorf("not shown as left out:\n%s", screen(m))
	}
	if m.applicable() {
		t.Fatal("a plan with nothing left applies")
	}
	press(m, "enter")
	if m.bulk != nil || !m.triage.open {
		t.Errorf("enter did something: bulk %+v", m.bulk)
	}
}

// A pull request that moved on after Claude read it is left out.
func TestTriageLeavesOutWhatChanged(t *testing.T) {
	m, _ := startTriage(t, `{"reply":"Selected.","actions":[{"do":"select","keys":["acme/api#12","acme/api#30"]}]}`)
	p, _ := m.knownPR(key("acme", "api", 12))
	p.HeadOID = "eeeeeee5"
	m.updatePR(p)
	if s := screen(m); !strings.Contains(s, "✗ acme/api#12 changed since Claude read it") {
		t.Errorf("not shown as changed:\n%s", s)
	}
	press(m, "enter")
	if m.isSelected(key("acme", "api", 12)) || !m.isSelected(key("acme", "api", 30)) {
		t.Errorf("selected = %v", m.selected)
	}
}

// What a row has not read is given as unknown, not as zero.
func TestTriageGivesUnreadFieldsAsUnknown(t *testing.T) {
	f := fixture()
	f.noHeavy = true
	m, _ := startWith(t, f, keymap.Default())
	fc := fakeClaudeOf(t, m)
	fc.plan = `{"reply":"","actions":[]}`
	press(m, "C", "x", "enter")
	if p := fc.triaged[0].Prompt; !strings.Contains(p, "ci=unknown review=unknown size=unknown") {
		t.Errorf("prompt:\n%s", p)
	}
}

// A plan Claude could not make leaves nothing to apply.
func TestTriageWithNothingToDo(t *testing.T) {
	m, _ := startTriage(t, `{"reply":"Sorting is not something I can do.","actions":[]}`)
	s := screen(m)
	if !strings.Contains(s, "Sorting is not something I can do.") || strings.Contains(s, "enter apply") {
		t.Errorf("screen:\n%s", s)
	}
	press(m, "enter")
	if !m.triage.open {
		t.Error("enter closed a plan with nothing to apply")
	}
	press(m, "esc")
	if m.triage.open {
		t.Error("esc did not close it")
	}
}

// Your own pull request cannot be approved, but can be merged.
func TestTriageOwnPullRequests(t *testing.T) {
	m, _ := startTriage(t, `{"reply":"","actions":[{"do":"approve","keys":["me/tool#3"]}]}`, "2")
	if !strings.Contains(screen(m), "✗ me/tool#3 your own") || m.applicable() {
		t.Errorf("approving your own:\n%s", screen(m))
	}
	m.triage.actions = []claude.Action{{Do: claude.DoMerge, Keys: []string{"me/tool#3"}}}
	press(m, "enter")
	if m.bulk == nil || m.bulk.kind != runner.Merge || len(m.bulk.items) != 1 {
		t.Errorf("bulk = %+v", m.bulk)
	}
}

// waitingTriage is a triage with a second request on its way, and whether
// it was stopped.
func waitingTriage(t *testing.T) (*Model, *fakeClaude, *bool) {
	t.Helper()
	m, fc := startTriage(t, `{"reply":"","actions":[]}`)
	stopped := new(bool)
	tr := m.triage
	tr.turns = append(tr.turns, triageTurn{request: "slow"})
	tr.asked++
	tr.asking, tr.cancel = true, func() { *stopped = true }
	return m, fc, stopped
}

// esc while Claude thinks stops it. The request stays in the conversation,
// and nothing can be sent until Claude Code has stopped.
func TestTriageStop(t *testing.T) {
	m, fc, stopped := waitingTriage(t)
	tr := m.triage
	press(m, "esc")
	if !*stopped || !tr.open || !tr.asking || !tr.stopping || tr.typing {
		t.Fatalf("stopped %v, open %v, asking %v, stopping %v, typing %v", *stopped, tr.open, tr.asking, tr.stopping, tr.typing)
	}
	if s := screen(m); !strings.Contains(s, "stopping…") {
		t.Errorf("not shown as stopping:\n%s", s)
	}
	m.Update(triageMsg{asked: tr.asked, err: context.Canceled})
	if tr.asking || tr.stopping || !tr.typing || !tr.turns[1].stopped {
		t.Fatalf("after its end: asking %v, stopping %v, typing %v, turns %+v", tr.asking, tr.stopping, tr.typing, tr.turns)
	}
	if s := screen(m); !strings.Contains(s, "⏺ stopped") || !strings.Contains(s, "> slow") {
		t.Errorf("the stopped request is not shown:\n%s", s)
	}
	if tr.session != fc.triaged[0].Session {
		t.Error("a started session was dropped")
	}
}

// A session Claude Code may have made before it stopped is not used again.
func TestTriageStopBeforeTheSessionStarted(t *testing.T) {
	m, fc, _ := waitingTriage(t)
	tr := m.triage
	tr.started = false
	press(m, "esc")
	m.Update(triageMsg{asked: tr.asked, err: context.Canceled})
	if tr.session == fc.triaged[0].Session {
		t.Error("the session was kept")
	}
}

// The popup can be closed while Claude goes on. Opened again it shows the
// request on its way, so nothing is sent twice, and then the plan.
func TestTriageClosedWhileAsking(t *testing.T) {
	m, fc, stopped := waitingTriage(t)
	tr := m.triage
	press(m, "backspace")
	if tr.open || *stopped || !tr.asking {
		t.Fatalf("open %v, stopped %v, asking %v", tr.open, *stopped, tr.asking)
	}
	press(m, "C")
	if !tr.open || tr.typing {
		t.Fatalf("opened again: open %v, typing %v", tr.open, tr.typing)
	}
	press(m, "h", "enter")
	if len(fc.triaged) != 1 {
		t.Fatalf("sent %d times", len(fc.triaged))
	}
	press(m, "backspace")
	plan := `{"reply":"Selected.","actions":[{"do":"select","keys":["acme/api#30"]}]}`
	m.Update(triageMsg{asked: tr.asked, answer: claude.Answer{Structured: []byte(plan)}})
	if tr.open || len(tr.actions) != 1 || !strings.Contains(status(m), "claude is done") {
		t.Fatalf("open %v, actions %+v, status %q", tr.open, tr.actions, status(m))
	}
	press(m, "C")
	if tr.typing || !strings.Contains(screen(m), "select api#30") {
		t.Fatalf("the plan is not shown:\n%s", screen(m))
	}
	press(m, "enter")
	if !m.isSelected(key("acme", "api", 30)) {
		t.Errorf("selected = %v", m.selected)
	}
}

// A plan is about the tab it was asked on: elsewhere it is not applied.
func TestTriagePlanAppliesOnItsTab(t *testing.T) {
	m, _ := startTriage(t, `{"reply":"Filtered.","actions":[{"do":"filter","text":"bump"},{"do":"approve","keys":["acme/api#30"]}]}`)
	tr := m.triage
	m.closeTriage()
	tr.actions = []claude.Action{{Do: claude.DoFilter, Text: "bump"}}
	press(m, "2", "C")
	s := screen(m)
	if !strings.Contains(s, "claude · review") || !strings.Contains(s, "applies there") {
		t.Errorf("on another tab:\n%s", s)
	}
	press(m, "enter")
	if m.filter != "" || !tr.open {
		t.Errorf("applied on another tab: filter %q, open %v", m.filter, tr.open)
	}
	press(m, "esc", "1")
	tr.actions = []claude.Action{{Do: claude.DoFilter, Text: "bump"}}
	press(m, "C", "enter")
	if m.filter != "bump" {
		t.Errorf("not applied on its tab: filter %q", m.filter)
	}
}

// chat_restart, pressed twice, starts a conversation over in a new session.
func TestTriageRestart(t *testing.T) {
	m, fc := startTriage(t, `{"reply":"","actions":[]}`)
	tr := m.triage
	press(m, "i", "ctrl+l")
	if len(tr.turns) != 1 {
		t.Fatal("a first press started over")
	}
	press(m, "ctrl+l")
	if len(tr.turns) != 0 || tr.session == fc.triaged[0].Session || tr.started {
		t.Fatalf("turns %+v, session %q, started %v", tr.turns, tr.session, tr.started)
	}
	press(m, "x", "enter")
	if r := fc.triaged[1]; !r.First || r.Session == fc.triaged[0].Session {
		t.Errorf("next request %+v", r)
	}
	press(m, "esc", "C", "ctrl+l")
	if !strings.Contains(status(m), "press") {
		t.Errorf("status %q", status(m))
	}
}

// Claude is given the tab shown only, and a pull request of another tab is
// left out of the plan.
func TestTriageIsAboutTheTabShown(t *testing.T) {
	m, fc := startTriage(t, `{"reply":"","actions":[{"do":"select","keys":["acme/api#12","me/tool#3"]}]}`)
	p := fc.triaged[0].Prompt
	if !strings.Contains(p, `tab "review"`) || !strings.Contains(p, "acme/api#12") || strings.Contains(p, "me/tool#3") || strings.Contains(p, `tab "mine"`) {
		t.Errorf("prompt:\n%s", p)
	}
	if s := screen(m); !strings.Contains(s, "✗ me/tool#3 not in this tab") || !strings.Contains(s, "claude · review") {
		t.Errorf("screen:\n%s", s)
	}
	press(m, "enter")
	if !m.isSelected(key("acme", "api", 12)) || m.isSelected(key("me", "tool", 3)) {
		t.Errorf("selected = %v", m.selected)
	}
}

// fakeClaudeOf is the fake Claude that startWith gave m.
func fakeClaudeOf(t *testing.T, m *Model) *fakeClaude {
	t.Helper()
	fc, ok := m.claude.(*fakeClaude)
	if !ok {
		t.Fatalf("m.claude is %T, not *fakeClaude", m.claude)
	}
	return fc
}

// memoryHistory is a history kept in memory.
type memoryHistory struct {
	entries []askbox.Entry
	replies map[string]string
	err     error
}

func (h *memoryHistory) Load() ([]askbox.Entry, error) { return h.entries, h.err }
func (h *memoryHistory) Add(e askbox.Entry) error {
	h.entries = append([]askbox.Entry{e}, h.entries...)
	return h.err
}
func (h *memoryHistory) Reply(id, text string) error {
	if h.replies == nil {
		h.replies = map[string]string{}
	}
	h.replies[id] = text
	return h.err
}

func withHistory(t *testing.T, h askbox.Store) *Model {
	t.Helper()
	m, _ := startWith(t, fixture(), keymap.Default())
	fakeClaudeOf(t, m).plan = `{"reply":"**Two** of them.\nMore.","actions":[]}`
	m.set.History = h
	return m
}

// up goes back through the questions kept, down comes back to what was
// typed, and a key that types still types.
func TestTriageRecall(t *testing.T) {
	now := time.Now()
	h := &memoryHistory{entries: []askbox.Entry{{Question: "newest", Tab: "mine", At: now.Add(-48 * time.Hour)}, {Question: "oldest"}}}
	m := withHistory(t, h)
	press(m, "C", "k", "j")
	tr := m.triage
	if got := tr.input.Value(); got != "kj" {
		t.Fatalf("typed %q", got)
	}
	if !strings.Contains(screen(m), "up history 2") {
		t.Errorf("no hint:\n%s", screen(m))
	}
	press(m, "up")
	if got := tr.input.Value(); got != "newest" {
		t.Fatalf("up gave %q", got)
	}
	if s := screen(m); !strings.Contains(s, "history 1/2") || !strings.Contains(s, "mine") {
		t.Errorf("no place:\n%s", s)
	}
	press(m, "ctrl+p", "ctrl+p")
	if got := tr.input.Value(); got != "oldest" {
		t.Fatalf("ctrl+p gave %q", got)
	}
	press(m, "ctrl+n", "down")
	if got := tr.input.Value(); got != "kj" || tr.recall.Active() {
		t.Fatalf("back to %q, active %v", got, tr.recall.Active())
	}
	press(m, "up", "x")
	if tr.recall.Active() || tr.input.Value() != "newestx" {
		t.Errorf("after editing: %q, active %v", tr.input.Value(), tr.recall.Active())
	}
	press(m, "down")
	if tr.input.Value() != "newestx" {
		t.Errorf("down left the questions and changed %q", tr.input.Value())
	}
}

// A question goes into the history as it is sent, and its first line of
// answer after; one stopped stays, and a later launch reads them.
func TestTriageKeepsQuestions(t *testing.T) {
	h := &memoryHistory{}
	m := withHistory(t, h)
	press(m, "C", "b", "o", "t", "s", "enter")
	if len(h.entries) != 1 || h.entries[0].Question != "bots" || h.entries[0].Tab != "review" {
		t.Fatalf("entries %+v", h.entries)
	}
	if got := h.replies[h.entries[0].ID]; got != "Two of them." {
		t.Errorf("reply %q", got)
	}
	if got := m.triage.history[0].Reply; got != "Two of them." {
		t.Errorf("reply in memory %q", got)
	}
	press(m, "i", "b", "o", "t", "s", "enter")
	if len(m.triage.history) != 1 {
		t.Errorf("the same question twice: %+v", m.triage.history)
	}
	n := withHistory(t, h)
	press(n, "C", "up")
	if got := n.triage.input.Value(); got != "bots" {
		t.Errorf("a later launch recalled %q", got)
	}
}

// The line of an answer is written after its question, however late that
// comes.
func TestTriageKeepsTheAnswerAfterItsQuestion(t *testing.T) {
	h := &slowHistory{added: make(chan struct{}), release: make(chan struct{}), replied: make(chan string, 1)}
	m := withHistory(t, h)
	press(m, "C", "a")
	tr := m.triage
	ask := m.keepQuestion("a")
	reply := m.keepReply("the answer")
	go reply()
	go ask()
	<-h.added
	select {
	case <-h.replied:
		t.Fatal("the answer went before its question")
	case <-time.After(20 * time.Millisecond):
	}
	close(h.release)
	if got := <-h.replied; got != tr.askedID {
		t.Errorf("replied to %q", got)
	}
}

// slowHistory holds Add until release is closed.
type slowHistory struct {
	added, release chan struct{}
	replied        chan string
}

func (h *slowHistory) Load() ([]askbox.Entry, error) { return nil, nil }
func (h *slowHistory) Add(askbox.Entry) error {
	close(h.added)
	<-h.release
	return nil
}
func (h *slowHistory) Reply(id, _ string) error {
	h.replied <- id
	return nil
}

// Questions asked while the history is read stay, before those read.
func TestTriageHistoryReadLate(t *testing.T) {
	m := withHistory(t, nil)
	press(m, "C", "n", "e", "w", "enter")
	m.Update(historyMsg{entries: []askbox.Entry{{Question: "old"}, {Question: "new"}}})
	h := m.triage.history
	if len(h) != 2 || h[0].Question != "new" || h[1].Question != "old" {
		t.Errorf("history %+v", h)
	}
}

// Starting over leaves the questions.
func TestChatRestartLeavesTheQuestions(t *testing.T) {
	m, _ := startWith(t, fixture(), keymap.Default())
	fakeClaudeOf(t, m)
	press(m, "i", "o", "n", "e", "enter")
	c := m.chat()
	press(m, "up", "ctrl+l", "ctrl+l")
	if c.recall.Active() {
		t.Error("still among the questions")
	}
	press(m, "down")
}

// A history that fails is told, and asking goes on.
func TestTriageHistoryFails(t *testing.T) {
	h := &memoryHistory{err: errors.New("disk full")}
	m := withHistory(t, h)
	press(m, "C")
	if !strings.Contains(status(m), "could not be read") {
		t.Errorf("status %q", status(m))
	}
	press(m, "a", "enter")
	if !strings.Contains(status(m), "could not be kept") || len(m.triage.turns) != 1 {
		t.Errorf("status %q, turns %d", status(m), len(m.triage.turns))
	}
}

// i recalls the questions of its pull request, but not a drafted review,
// whose prompt is not the text shown.
func TestChatRecall(t *testing.T) {
	m, _ := startWith(t, fixture(), keymap.Default())
	fakeClaudeOf(t, m)
	press(m, "i", "o", "n", "e", "enter")
	c := m.chat()
	press(m, "t", "w", "o", "enter", "ctrl+x")
	press(m, "esc")
	press(m, "i")
	press(m, "up")
	if got := c.input.Value(); got != "two" {
		t.Fatalf("up gave %q", got)
	}
	press(m, "up", "up")
	if got := c.input.Value(); got != "one" {
		t.Fatalf("then %q", got)
	}
}

// On a question over several rows, up and ctrl+p move the cursor until it
// is on the first.
func TestRecallOnlyFromTheEdges(t *testing.T) {
	long := strings.Repeat("a long question ", 20)
	h := &memoryHistory{entries: []askbox.Entry{{Question: "newest"}, {Question: "older"}}}
	m := withHistory(t, h)
	press(m, "C")
	tr := m.triage
	m.Update(tea.PasteMsg{Content: long})
	press(m, "up")
	if tr.recall.Active() {
		t.Fatal("up recalled from the last row")
	}
	for range 30 {
		press(m, "up")
		if tr.recall.Active() {
			break
		}
	}
	if !tr.recall.Active() || tr.input.Value() != "newest" {
		t.Errorf("value %q", tr.input.Value())
	}
	press(m, "down")
	if tr.input.Value() != long {
		t.Errorf("typed text not back: %q", tr.input.Value())
	}
}

// ctrl+r turns the text box into a search of the questions, in place: the
// popup gets no box of its own. enter fills in and sends nothing.
func TestTriageSearch(t *testing.T) {
	h := &memoryHistory{entries: []askbox.Entry{
		{Question: "bots with passing checks", Tab: "review", Reply: "2 bots"},
		{Question: "先週から動いていないものは？", Tab: "mine"},
		{Question: "what can I approve safely?"},
	}}
	m := withHistory(t, h)
	h.entries[0].At = m.set.Now().Add(-48 * time.Hour)
	press(m, "C", "d", "r", "a", "f", "t", "ctrl+r")
	tr := m.triage
	s := screen(m)
	for _, want := range []string{"Search the questions", "› 2d", "bots with passing checks", "⏺ 2 bots", "cancel"} {
		if !strings.Contains(s, want) {
			t.Errorf("no %q:\n%s", want, s)
		}
	}
	if strings.Count(s, "╭") != 2 {
		t.Errorf("a box over the popup:\n%s", s)
	}
	press(m, "動", "い")
	if s := screen(m); !strings.Contains(s, "先週から") || strings.Contains(s, "2 bots") {
		t.Errorf("not narrowed:\n%s", s)
	}
	press(m, "enter")
	if tr.search != nil || tr.input.Value() != "先週から動いていないものは？" || len(tr.turns) != 0 {
		t.Errorf("search %v, value %q, turns %d", tr.search, tr.input.Value(), len(tr.turns))
	}
	press(m, "ctrl+u")
	tr.input.SetValue("typed")
	press(m, "ctrl+r", "ctrl+n", "ctrl+p", "ctrl+p", "ctrl+r", "esc")
	if tr.search != nil || tr.input.Value() != "typed" || !tr.typing {
		t.Errorf("after esc: search %v, value %q, typing %v", tr.search, tr.input.Value(), tr.typing)
	}
}

// With nothing asked, there is nothing to search.
func TestTriageSearchOfNothing(t *testing.T) {
	m := withHistory(t, nil)
	press(m, "C", "ctrl+r")
	if m.triage.search != nil || !strings.Contains(status(m), "nothing has been asked") {
		t.Errorf("search %v, status %q", m.triage.search, status(m))
	}
}

// The chosen question stays among those shown when there are more than the
// rows, and no line of the search is wider than the screen.
func TestSearchFollowsTheChosen(t *testing.T) {
	h := &memoryHistory{}
	for i := range 12 {
		h.entries = append(h.entries, askbox.Entry{Question: "question " + string(rune('a'+i))})
	}
	m := withHistory(t, h)
	press(m, "C", "ctrl+r")
	for range 10 {
		press(m, "down")
	}
	if s := screen(m); !strings.Contains(s, "› ") || !strings.Contains(s, "question k") {
		t.Errorf("chosen not shown:\n%s", s)
	}
}

// In i the box of the pane becomes the search, and the draft has moved.
func TestChatSearch(t *testing.T) {
	m, _ := startWith(t, fixture(), keymap.Default())
	fc := fakeClaudeOf(t, m)
	press(m, "i", "o", "n", "e", "enter")
	c := m.chat()
	press(m, "ctrl+r")
	s := screen(m)
	if !strings.Contains(s, "history · acme/api#12") || strings.Contains(s, "ask claude") || !strings.Contains(s, "⏺ Answer 1") {
		t.Errorf("no search:\n%s", s)
	}
	press(m, "enter")
	if c.search != nil || c.input.Value() != "one" || len(fc.asked) != 1 {
		t.Errorf("search %v, value %q, asked %d", c.search, c.input.Value(), len(fc.asked))
	}
	press(m, "ctrl+x")
	if len(fc.asked) != 2 || !strings.Contains(fc.asked[1].Prompt, claude.DraftPrompt) {
		t.Errorf("ctrl+x did not draft: %d", len(fc.asked))
	}
}

// A text whose lower case is not as long in bytes marks and does not fall
// over, whichever side it is on.
func TestMarkHitByRune(t *testing.T) {
	m, _ := startWith(t, fixture(), keymap.Default())
	for _, tc := range []struct{ text, q, want string }{
		{"k", "\u212a", "k"},
		{"\u212a", "k", "\u212a"},
		{"Bots with checks", "with", "Bots with checks"},
		{"先週から動いていない", "動い", "先週から動いていない"},
		{"nothing", "zzz", "nothing"},
		{"nothing", "", "nothing"},
	} {
		if got := ansi.Strip(m.markHit(tc.text, tc.q, m.st.dim)); got != tc.want {
			t.Errorf("markHit(%q, %q) = %q", tc.text, tc.q, got)
		}
	}
}

// On a low screen the text box of a search stays in sight.
func TestSearchOnALowScreen(t *testing.T) {
	for _, height := range []int{12, 14, 16, 18, 24} {
		m, _ := startWith(t, fixture(), keymap.Default())
		fakeClaudeOf(t, m)
		m.Update(tea.WindowSizeMsg{Width: 130, Height: height})
		press(m, "i", "o", "n", "e", "enter", "t", "w", "o", "enter", "ctrl+r")
		if !m.claudeShown() {
			continue
		}
		s, cur := m.render()
		if cur == nil {
			t.Fatalf("height %d: no cursor", height)
		}
		lines := strings.Split(ansi.Strip(s), "\n")
		if !strings.Contains(lines[cur.Y], "Search the questions") {
			t.Errorf("height %d: the cursor is on %q", height, lines[cur.Y])
		}
	}
}

// What is typed is followed, faint, by the rest of a question, and tab or
// right fills it in. The cursor stays after the typed text.
func TestAskGhost(t *testing.T) {
	h := &memoryHistory{entries: []askbox.Entry{{Question: "先週から動いていないものは？"}}}
	m := withHistory(t, h)
	press(m, "C", "b", "o")
	tr := m.triage
	s, cur := m.render()
	if !strings.Contains(ansi.Strip(s), "┃ bots with passing checks") {
		t.Fatalf("no ghost:\n%s", ansi.Strip(s))
	}
	if !strings.Contains(ansi.Strip(s), "tab complete") {
		t.Errorf("no key told:\n%s", ansi.Strip(s))
	}
	lines := strings.Split(ansi.Strip(s), "\n")
	if got := ansi.Cut(lines[cur.Y], 0, cur.X); !strings.HasSuffix(got, "┃ bo") {
		t.Errorf("cursor after %q", got)
	}
	press(m, "tab")
	if tr.input.Value() != "bots with passing checks" {
		t.Errorf("tab gave %q", tr.input.Value())
	}
	tr.input.Reset()
	press(m, "先", "right")
	if tr.input.Value() != "先週から動いていないものは？" {
		t.Errorf("right gave %q", tr.input.Value())
	}
	tr.input.SetValue("bo")
	press(m, "left")
	if got := screen(m); strings.Contains(got, "┃ bots with") {
		t.Errorf("a ghost with the cursor inside the text:\n%s", got)
	}
	press(m, "right")
	if tr.input.Value() != "bo" {
		t.Errorf("right inside the text filled in %q", tr.input.Value())
	}
}

// The welcome counts what a triage is given, by group as a whole.
func TestTriageWelcomeCountsTheSameRows(t *testing.T) {
	m, _ := startWith(t, fixture(), keymap.Default())
	fakeClaudeOf(t, m)
	tab := m.tab()
	listed := m.listed(tab)
	tab.prs = append(tab.prs, tab.prs...)
	if len(m.listed(tab)) == 0 || len(listed) == 0 {
		t.Fatal("nothing listed")
	}
	press(m, "C")
	var prs []model.PR
	for _, r := range m.listed(tab) {
		prs = append(prs, r.pr)
	}
	s := screen(m)
	if !strings.Contains(s, pullRequests(len(prs))) || !strings.Contains(s, strings.Join(m.groupCounts(tab, prs), " · ")) {
		t.Errorf("counts differ:\n%s", s)
	}
}

// # and @ open a menu of what the tab holds, which fills in the word typed.
func TestTriageMenu(t *testing.T) {
	m := withHistory(t, nil)
	press(m, "C", "a", "space", "#")
	tr := m.triage
	s := screen(m)
	if !strings.Contains(s, "› api#12  Add retries to the client") || !strings.Contains(s, "close") {
		t.Fatalf("no menu:\n%s", s)
	}
	press(m, "3", "enter")
	if got := tr.input.Value(); got != "a api#30 " || len(tr.turns) != 0 {
		t.Fatalf("value %q, turns %d", got, len(tr.turns))
	}
	press(m, "@", "ctrl+n", "tab")
	if got := tr.input.Value(); !strings.HasPrefix(got, "a api#30 @") || !strings.HasSuffix(got, " ") || got == "a api#30 @" {
		t.Fatalf("value %q", got)
	}
	tr.input.SetValue("#")
	press(m, "esc")
	if !tr.open || !tr.typing || !tr.menu.Closed() || strings.Contains(screen(m), "› api#12") {
		t.Errorf("esc: open %v, typing %v, closed %v", tr.open, tr.typing, tr.menu.Closed())
	}
	press(m, "enter")
	if len(tr.turns) != 1 {
		t.Errorf("enter with the menu closed sent %d", len(tr.turns))
	}
}

// i has no menu: # is typed as any character.
func TestChatHasNoMenu(t *testing.T) {
	m, _ := startWith(t, fixture(), keymap.Default())
	fakeClaudeOf(t, m)
	press(m, "i", "#", "1")
	if s := screen(m); strings.Contains(s, "› api#12") {
		t.Errorf("a menu in i:\n%s", s)
	}
}

// A question of several lines is drawn as far as its first, and filled in
// whole.
func TestAskGhostOfSeveralLines(t *testing.T) {
	h := &memoryHistory{entries: []askbox.Entry{{Question: "foo\n\n\nbar"}}}
	m := withHistory(t, h)
	press(m, "C", "f")
	before := strings.Count(screen(m), "\n")
	lines := m.askLines(&m.triage.input, askGhost(&m.triage.input, &m.triage.recall, nil, m.triage.history))
	for _, l := range lines {
		if strings.Contains(l, "\n") {
			t.Fatalf("a line of the box holds a line break: %q", l)
		}
	}
	if !strings.Contains(screen(m), "┃ foo") || strings.Count(screen(m), "\n") != before {
		t.Errorf("screen:\n%s", screen(m))
	}
	press(m, "tab")
	if got := m.triage.input.Value(); got != "foo\n\n\nbar" {
		t.Errorf("tab gave %q", got)
	}
}
