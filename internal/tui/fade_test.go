package tui

import (
	"slices"
	"testing"
	"time"

	"github.com/babarot/gh-desk/internal/model"
	"github.com/babarot/gh-desk/internal/runner"
	"github.com/babarot/gh-desk/internal/tui/keymap"
)

// An approval's result is put away by the first automatic reading that
// comes at least an interval after it, however close to a reading it came,
// and the row leaves a tab that no longer has the pull request. A merge's
// result stays until r (ADR-007, ADR-084).
func TestAReviewResultFadesAfterAnInterval(t *testing.T) {
	f := fixture()
	m := start(t, f, keymap.Default())
	m.set.Refresh = 3 * time.Minute
	clock := now
	m.set.Now = func() time.Time { return clock }
	review := m.tabs[0]
	approved, merged := key("acme", "api", 12), key("acme", "api", 30)

	m.event(eventMsg{ok: true, ev: runner.Event{Key: approved, Result: runner.Approved, Text: "approved", Approved: true}})
	m.event(eventMsg{ok: true, ev: runner.Event{Key: merged, Result: runner.Merged, Text: "merged"}})
	// GitHub no longer has them as asking for the viewer's review.
	f.searches["review"] = slices.DeleteFunc(slices.Clone(f.searches["review"]), func(p model.PR) bool {
		return p.Key == approved || p.Key == merged
	})

	listed := func(k model.PRKey) bool {
		return slices.ContainsFunc(m.allOf(review), func(p model.PR) bool { return p.Key == k })
	}
	tick := func(after time.Duration) {
		clock = now.Add(after)
		_, cmd := m.Update(tickMsg{})
		run(m, cmd)
	}

	tick(10 * time.Second)
	if _, shown := m.progress[approved]; !shown || !listed(approved) {
		t.Fatal("the approval was put away by a reading 10 seconds after it")
	}
	tick(3*time.Minute + 10*time.Second)
	if _, shown := m.progress[approved]; shown || listed(approved) {
		t.Error("the approval stayed after a reading an interval after it")
	}
	if _, shown := m.progress[merged]; !shown || !listed(merged) {
		t.Error("the merge was put away by the automatic refresh")
	}
}
