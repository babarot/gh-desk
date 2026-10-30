package tui

import (
	"strings"
	"testing"

	"github.com/babarot/gh-triage/internal/claude"
	"github.com/babarot/gh-triage/internal/keymap"
	"github.com/babarot/gh-triage/internal/runner"
)

// startTriage starts the TUI with Claude answering plan, and asks it.
func startTriage(t *testing.T, plan string) (*Model, *fakeClaude) {
	t.Helper()
	m, _ := startWith(t, fixture(), keymap.Default())
	fc := m.claude.(*fakeClaude)
	fc.plan = plan
	press(m, "C", "b", "o", "t", "s", "enter")
	if len(fc.triaged) != 1 {
		t.Fatalf("asked %d times", len(fc.triaged))
	}
	return m, fc
}

func TestTriagePlanApplied(t *testing.T) {
	m, fc := startTriage(t, `{"reply":"Picked the bot update; api#99 is not in the lists.","actions":[`+
		`{"do":"unfold","group":"bots"},{"do":"select","keys":["acme/api#30","acme/api#99"]},`+
		`{"do":"cursor","key":"acme/api#30"},{"do":"approve","keys":["acme/api#30"]}]}`)
	r := fc.triaged[0]
	if !r.First || !strings.Contains(r.Prompt, "<request>\nbots\n</request>") || !strings.Contains(r.Prompt, "acme/api#30 head=ccccccc group=bots") {
		t.Errorf("request %+v", r)
	}
	golden(t, "triage-plan", screen(m))
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
	if m.bulk == nil || m.bulk.kind != runner.Approve || len(m.bulk.items) != 1 || m.bulk.by != "bots" {
		t.Fatalf("bulk = %+v", m.bulk)
	}
	golden(t, "triage-confirm", screen(m))

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
	fc := m.claude.(*fakeClaude)
	fc.plan = `{"reply":"Approving.","actions":[{"do":"select","keys":["acme/api#99"]},{"do":"approve","keys":["acme/api#99"]}]}`
	press(m, "space", "C", "g", "o", "enter")
	if s := screen(m); strings.Count(s, "✗ acme/api#99 not in the lists") != 2 || strings.Contains(s, "enter apply") {
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
	fc := m.claude.(*fakeClaude)
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
	m, _ := startTriage(t, `{"reply":"","actions":[{"do":"approve","keys":["me/tool#3"]}]}`)
	if !strings.Contains(screen(m), "✗ me/tool#3 your own") || m.applicable() {
		t.Errorf("approving your own:\n%s", screen(m))
	}
	m.triage.actions = []claude.Action{{Do: claude.DoMerge, Keys: []string{"me/tool#3"}}}
	press(m, "enter")
	if m.bulk == nil || m.bulk.kind != runner.Merge || len(m.bulk.items) != 1 {
		t.Errorf("bulk = %+v", m.bulk)
	}
}
