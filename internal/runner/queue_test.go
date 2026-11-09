package runner

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/babarot/gh-desk/internal/model"
)

// queuePR is a pull request whose base branch merges through a merge
// queue.
func queuePR(author string) (model.MergeInfo, Item) {
	mi, it := pr("1", "api", 1, author)
	mi.Queue = true
	return mi, it
}

// watchEnd is the event that ended the watch of a pull request.
func watchEnd(t *testing.T, es []Event) Event {
	t.Helper()
	for _, e := range es {
		if e.Watch {
			return e
		}
	}
	t.Fatalf("no watch event in %+v", es)
	return Event{}
}

func hasResult(es []Event, r Result) bool {
	for _, e := range es {
		if e.Result == r && !e.Watch {
			return true
		}
	}
	return false
}

// The merge queue merges a pull request: gh-desk puts it in the queue,
// never merges it itself, and watches it until the queue has merged it.
func TestCleanGoesIntoTheQueue(t *testing.T) {
	mi, it := queuePR("alice")
	f := newFake(mi)
	f.onRead = func(_ string, mi *model.MergeInfo, n int) {
		if mi.InQueue && n > 4 {
			mi.State, mi.InQueue = model.StateMerged, false
		}
	}
	es := runBatch(t, f, Merge, it)[it.PR.Key]
	if want := "review acme/api#1 APPROVE h-1,enqueue acme/api#1 h-1"; strings.Join(f.calls, ",") != want {
		t.Errorf("calls = %v", f.calls)
	}
	if !hasResult(es, Queued) {
		t.Errorf("no queued event in %+v", es)
	}
	if e := watchEnd(t, es); e.Result != Merged || !e.Queued || e.Text != "merged by the merge queue" {
		t.Errorf("watch ended with %+v", e)
	}
}

// One already in the queue is only followed: GitHub says UNKNOWN of it
// while it is there, which is not a reason to read it again and again.
func TestInTheQueueIsOnlyWatched(t *testing.T) {
	mi, it := queuePR("alice")
	mi.MergeState, mi.InQueue = model.MergeUnknown, true
	f := newFake(mi)
	f.onRead = func(_ string, mi *model.MergeInfo, n int) {
		if n > 4 {
			mi.State, mi.InQueue = model.StateMerged, false
		}
	}
	es := runBatch(t, f, Merge, it)[it.PR.Key]
	if len(f.calls) != 0 {
		t.Errorf("calls = %v; want none", f.calls)
	}
	if e := watchEnd(t, es); e.Result != Merged {
		t.Errorf("watch ended with %+v", e)
	}
}

// On a branch with a merge queue, auto-merge puts the pull request in the
// queue once it can be merged and is off from then on: the watch goes on
// while it is in the queue.
func TestAutoMergeIntoTheQueueIsWatchedThere(t *testing.T) {
	mi, it := queuePR("renovate")
	it.AutoMerge = true
	mi.MergeState, mi.Repo.AutoMerge = model.MergeBlocked, true
	f := newFake(mi)
	f.onRead = func(_ string, mi *model.MergeInfo, n int) {
		switch {
		case mi.InQueue && n > 10:
			mi.State, mi.InQueue = model.StateMerged, false
		case mi.AutoMerge && n > 5:
			mi.AutoMerge, mi.InQueue, mi.MergeState = false, true, model.MergeUnknown
		}
	}
	es := runBatch(t, f, Merge, it)[it.PR.Key]
	if !hasResult(es, AutoMergeOn) {
		t.Errorf("no auto-merge event in %+v", es)
	}
	if e := watchEnd(t, es); e.Result != Merged || e.Text != "merged by the merge queue" {
		t.Errorf("watch ended with %+v", e)
	}
}

// Auto-merge cannot be turned on for one that can be merged now; on a
// branch with a merge queue it goes into the queue instead of a merge the
// branch's rules refuse.
func TestCleanStatusGoesIntoTheQueue(t *testing.T) {
	mi, it := queuePR("renovate")
	it.AutoMerge = true
	mi.MergeState, mi.Repo.AutoMerge = model.MergeBlocked, true
	f := newFake(mi)
	f.autoMergeErr = fmt.Errorf("%w: GitHub said so", ErrCleanStatus)
	f.onRead = func(_ string, mi *model.MergeInfo, n int) {
		if mi.InQueue && n > 6 {
			mi.State, mi.InQueue = model.StateMerged, false
		}
	}
	es := runBatch(t, f, Merge, it)[it.PR.Key]
	if !slicesHasPrefix(f.calls, "enqueue ") || slicesHasPrefix(f.calls, "merge ") {
		t.Errorf("calls = %v", f.calls)
	}
	if e := watchEnd(t, es); e.Result != Merged {
		t.Errorf("watch ended with %+v", e)
	}
}

// A refused enqueue is read again: one someone else put in the queue is
// watched there.
func TestRefusedEnqueueOfOneInTheQueue(t *testing.T) {
	mi, it := queuePR("me")
	f := newFake(mi)
	f.enqueueErr = errors.New("Pull Request is in the merge queue")
	f.onRead = func(_ string, mi *model.MergeInfo, n int) {
		switch {
		case n > 6:
			mi.State, mi.InQueue = model.StateMerged, false
		case strings.Contains(strings.Join(f.calls, ","), "enqueue"):
			mi.InQueue = true
		}
	}
	es := runBatch(t, f, Merge, it)[it.PR.Key]
	if !hasResult(es, Queued) {
		t.Errorf("no queued event in %+v", es)
	}
}

// One that leaves the queue unmerged (a check of the queue failed, or
// someone took it out) ends the watch.
func TestLeavingTheQueueStopsTheWatch(t *testing.T) {
	mi, it := queuePR("me")
	f := newFake(mi)
	f.onRead = func(_ string, mi *model.MergeInfo, n int) {
		if mi.InQueue && n > 4 {
			mi.InQueue = false
		}
	}
	es := runBatch(t, f, Merge, it)[it.PR.Key]
	if e := watchEnd(t, es); e.Result != Stopped || e.Text != "left the merge queue unmerged" || !e.Queued {
		t.Errorf("watch ended with %+v", e)
	}
}

func slicesHasPrefix(calls []string, prefix string) bool {
	for _, c := range calls {
		if strings.HasPrefix(c, prefix) {
			return true
		}
	}
	return false
}
