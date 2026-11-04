package tui

import (
	"testing"
	"time"

	"github.com/babarot/gh-desk/internal/model"
	"github.com/babarot/gh-desk/internal/runner"
	"github.com/babarot/gh-desk/internal/tui/keymap"
)

// A draft made ready by a batch stays where it was in a tab that sorts
// drafts last, while its row shows the result; putting the results away
// sorts it as it is.
func TestARowShowingAResultKeepsItsPlace(t *testing.T) {
	f := fixture()
	draft := model.PR{ID: "5", Key: key("me", "tool", 4), Title: "Try a new parser", Author: "me", State: model.StateOpen, Draft: true,
		HeadOID: "eeeeeee5", Updated: now.Add(-time.Hour)}
	f.searches["mine"] = append(f.searches["mine"], draft)
	m := start(t, f, keymap.Default())
	mine := m.tabs[1]

	order := func() []model.PRKey {
		var ks []model.PRKey
		for _, r := range m.rows(mine) {
			if r.kind == rowPR {
				ks = append(ks, r.pr.Key)
			}
		}
		return ks
	}
	if got := order(); len(got) != 2 || got[1] != draft.Key {
		t.Fatalf("before: %v, want the draft last", got)
	}

	m.event(eventMsg{ok: true, ev: runner.Event{Key: draft.Key, Result: runner.Readied, Text: "ready for review"}})
	if p, _ := m.knownPR(draft.Key); p.Draft {
		t.Fatal("the pull request is still a draft")
	}
	if got := order(); got[1] != draft.Key {
		t.Errorf("after ready: %v, want the row where it was", got)
	}

	m.clearResults()
	if got := order(); got[0] != draft.Key {
		t.Errorf("after the results are put away: %v, want it first, as the newest ready one", got)
	}
}
