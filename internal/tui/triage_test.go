package tui

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/babarot/gh-desk/internal/claude"
	"github.com/babarot/gh-desk/internal/runner"
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
