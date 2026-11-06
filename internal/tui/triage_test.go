package tui

import (
	"strings"
	"testing"

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

// esc while Claude thinks stops it and forgets the request.
func TestTriageStop(t *testing.T) {
	m, fc := startTriage(t, `{"reply":"","actions":[]}`)
	stopped := false
	tr := m.triage
	tr.turns = append(tr.turns, triageTurn{request: "slow"})
	tr.asking, tr.cancel = true, func() { stopped = true }
	press(m, "esc")
	if !stopped || tr.open || tr.asking || len(tr.turns) != 1 {
		t.Errorf("stopped %v, open %v, turns %+v", stopped, tr.open, tr.turns)
	}
	if tr.session != fc.triaged[0].Session {
		t.Error("a started session was dropped")
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
