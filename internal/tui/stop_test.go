package tui

import (
	"slices"
	"strings"
	"testing"

	"github.com/babarot/gh-desk/internal/model"
	"github.com/babarot/gh-desk/internal/runner"
	"github.com/babarot/gh-desk/internal/tui/keymap"
)

// running puts a job at work on k, as a batch's events would.
func running(m *Model, k model.PRKey) {
	m.progress[k] = progress{text: "waiting for checks", result: runner.Running}
	m.addBatch("b1", &batchState{kind: runner.Merge, goes: map[model.PRKey]bool{k: true}, order: []model.PRKey{k}, done: map[model.PRKey]progress{}})
}

// X on a row at work asks the runner to stop it, and says so.
func TestStopTheRowAtWork(t *testing.T) {
	m, r := startWith(t, fixture(), keymap.Default())
	p, _, _ := m.currentPR()
	running(m, p.Key)
	if !strings.Contains(menuText(m), "stop waiting; nothing more is written") {
		t.Errorf("the menu does not offer stop:\n%s", menuText(m))
	}
	press(m, "X")
	if !slices.EqualFunc(r.stopped, [][]model.PRKey{{p.Key}}, slices.Equal) {
		t.Fatalf("stopped = %v", r.stopped)
	}
	if m.toast == nil || m.toast.title != "stop: asked to stop 1" {
		t.Errorf("toast = %+v", m.toast)
	}
}

// menuText is the menu of enter as drawn, closed again after.
func menuText(m *Model) string {
	press(m, "enter")
	defer press(m, "esc")
	return screen(m)
}

// X on a selection stops what is at work in it, counts the rest, and
// spends the selection as a confirmed action does (ADR-094).
func TestStopSpendsTheSelection(t *testing.T) {
	m, r := startWith(t, fixture(), keymap.Default())
	press(m, "space", "space")
	sel := slices.Collect(func(yield func(model.PRKey) bool) {
		for k := range m.selected {
			if !yield(k) {
				return
			}
		}
	})
	if len(sel) != 2 {
		t.Fatalf("selected %v", sel)
	}
	running(m, sel[0])
	press(m, "X")
	if !slices.EqualFunc(r.stopped, [][]model.PRKey{{sel[0]}}, slices.Equal) {
		t.Errorf("stopped = %v", r.stopped)
	}
	if len(m.selected) != 0 || len(m.lastSelected) != 2 {
		t.Errorf("selected %v, kept for u %v", m.selected, m.lastSelected)
	}
	if m.toast == nil || m.toast.title != "stop: asked to stop 1 · 1 not at work" {
		t.Errorf("toast = %+v", m.toast)
	}
}

// X where nothing is at work stops nothing and says why; the selection
// stays, as no action ran.
func TestStopWhatIsNotAtWork(t *testing.T) {
	m, r := startWith(t, fixture(), keymap.Default())
	press(m, "X")
	if len(r.stopped) != 0 || !strings.Contains(status(m), "stop: not at work") {
		t.Errorf("stopped %v, status %q", r.stopped, status(m))
	}
}

// A pull request on auto-merge is GitHub's to merge: X leaves it alone and
// says where to turn it off.
func TestStopLeavesAutoMergeToGitHub(t *testing.T) {
	m, r := startWith(t, fixture(), keymap.Default())
	p, _, _ := m.currentPR()
	m.progress[p.Key] = progress{text: "auto-merge on", result: runner.AutoMergeOn}
	press(m, "X")
	if len(r.stopped) != 0 || !strings.Contains(status(m), "auto-merge is GitHub's now") {
		t.Errorf("stopped %v, status %q", r.stopped, status(m))
	}
}

// The status line tells the key that stops, while something is at work,
// both under a running batch and on the row of one at work.
func TestStatusTellsTheStopKey(t *testing.T) {
	m := start(t, fixture(), keymap.Default())
	p, _, _ := m.currentPR()
	if strings.Contains(status(m), "X stop") {
		t.Fatalf("told with nothing at work: %q", status(m))
	}
	running(m, p.Key)
	if got := status(m); !strings.Contains(got, "merging… 0/1") || !strings.HasSuffix(got, " · X stop") {
		t.Errorf("under a batch: %q", got)
	}
	m.movedOnto()
	if got := status(m); got != p.Key.String()+": waiting for checks · X stop" {
		t.Errorf("on the row: %q", got)
	}
}

// What the user stopped is a decision: the batch's note counts it apart,
// without the mark of a failure, and . does not offer it again (ADR-122).
func TestStoppedByYouIsNotTriedAgain(t *testing.T) {
	m := start(t, fixture(), keymap.Default())
	mine, moved := key("acme", "api", 12), key("acme", "web", 7)
	bs := &batchState{kind: runner.Merge, goes: map[model.PRKey]bool{mine: true, moved: true}, order: []model.PRKey{mine, moved},
		done: map[model.PRKey]progress{
			mine:  {result: runner.Stopped, text: "stopped by you", byUser: true},
			moved: {result: runner.Stopped, text: "head changed · confirm again"},
		}}
	run(m, m.noteBatch(bs))
	if m.retry == nil {
		t.Fatal("the pull request whose head moved is not left to try again")
	}
	if n := m.notes[0]; !strings.Contains(n.title, "1 stopped, 1 stopped by you") || n.icon != "✗" {
		t.Errorf("note %q %q", n.icon, n.title)
	}

	m = start(t, fixture(), keymap.Default())
	bs = &batchState{kind: runner.Merge, goes: map[model.PRKey]bool{mine: true}, order: []model.PRKey{mine},
		done: map[model.PRKey]progress{mine: {result: runner.Stopped, text: "stopped by you", byUser: true}}}
	run(m, m.noteBatch(bs))
	if m.retry != nil {
		t.Errorf("retry = %+v", m.retry)
	}
	if n := m.notes[0]; n.title != "merge done: merged 0, 1 stopped by you" || n.icon != "✓" {
		t.Errorf("note %q %q", n.icon, n.title)
	}
}
