package runner

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/babarot/gh-desk/internal/model"
)

// stack puts mi in a stack; based on the open pull request base, or the
// lowest of its stack when base is 0.
func stack(mi *model.MergeInfo, base int) {
	mi.Stacked, mi.BasePR = true, base
}

func called(f *fakeGH, prefix string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, c := range f.calls {
		if strings.HasPrefix(c, prefix) {
			n++
		}
	}
	return n
}

// The lowest pull request of a stack is merged through the asynchronous
// merge API, followed until GitHub says it merged; the synchronous merge,
// which GitHub refuses for a stack, is not tried.
func TestAStackedPullRequestIsMergedAsync(t *testing.T) {
	mi, it := pr("1", "api", 1, "alice")
	stack(&mi, 0)
	f := newFake(mi)
	f.async.polls = 3
	st := newStore()
	r := newRunner(f, st)
	_, ch := r.Start(context.Background(), Merge, []Item{it})
	es := drain(t, ch)
	if e := last(es); e.Result != Merged {
		t.Errorf("last event = %+v; calls %v", e, f.calls)
	}
	if called(f, "merge acme") != 0 || called(f, "merge-async acme/api#1 h-1 queue=false bypass=false") != 1 || called(f, "merge-status") != 3 {
		t.Errorf("calls = %v", f.calls)
	}
	if _, ok := st.now[mi.Key]; ok {
		t.Error("the record stayed")
	}
}

func TestAStackedMergeThatFailsSaysWhy(t *testing.T) {
	mi, it := pr("1", "api", 1, "alice")
	stack(&mi, 0)
	f := newFake(mi)
	f.async.polls, f.async.end, f.async.message = 1, model.MergeFailed, "required review missing"
	evs := runBatch(t, f, Merge, it)
	if e := last(evs[it.PR.Key]); e.Result != Failed || e.Text != "merge failed: required review missing" {
		t.Errorf("last event = %+v", e)
	}
}

// Where the base has a merge queue, the asynchronous merge puts it there,
// and it is watched in the queue as any other.
func TestAStackedPullRequestGoesToTheMergeQueueAsync(t *testing.T) {
	mi, it := pr("1", "api", 1, "alice")
	stack(&mi, 0)
	mi.Queue = true
	f := newFake(mi)
	f.async.end = model.MergeEnqueued
	f.onRead = func(_ string, mi *model.MergeInfo, _ int) {
		if mi.InQueue {
			mi.State = model.StateMerged
		}
	}
	evs := runBatch(t, f, Merge, it)
	if !slices.ContainsFunc(evs[it.PR.Key], func(e Event) bool { return e.Result == Queued }) {
		t.Errorf("never queued: %+v", evs[it.PR.Key])
	}
	if called(f, "enqueue") != 0 || called(f, "merge-async acme/api#1 h-1 queue=true") != 1 {
		t.Errorf("calls = %v", f.calls)
	}
}

// A request asked before and still pending is asked again once it ends.
func TestAPendingStackedMergeIsAskedAgain(t *testing.T) {
	mi, it := pr("1", "api", 1, "alice")
	stack(&mi, 0)
	f := newFake(mi)
	f.async.busy = 2
	evs := runBatch(t, f, Merge, it)
	if e := last(evs[it.PR.Key]); e.Result != Merged {
		t.Errorf("last event = %+v", e)
	}
	if n := called(f, "merge-async"); n != 3 {
		t.Errorf("asked %d times: %v", n, f.calls)
	}
}

// One higher up a stack is not merged on its own, even when GitHub would,
// since that would merge the ones below it too; nor as an admin.
func TestAPullRequestHigherUpAStackIsNotMergedAlone(t *testing.T) {
	clean, iclean := pr("2", "api", 2, "alice")
	stack(&clean, 1)
	admin, iadmin := pr("3", "web", 3, "me")
	stack(&admin, 1)
	admin.MergeState, admin.AdminMerge = model.MergeBlocked, true
	admin.Checks = []model.Check{{Name: "build", State: model.CheckSuccess, Required: true}}
	f := newFake(clean, admin)
	evs := runBatch(t, f, Merge, iclean, iadmin)
	for _, k := range []model.PRKey{iclean.PR.Key, iadmin.PR.Key} {
		if e := last(evs[k]); e.Result != Stopped || !strings.Contains(e.Text, "#1 is not merged") {
			t.Errorf("%s: %+v", k, e)
		}
	}
	if called(f, "merge") != 0 {
		t.Errorf("wrote a merge: %v", f.calls)
	}
}

// A merge asked before gh-desk quit is followed by its ID, not asked again.
func TestAResumedStackedMergeIsFollowed(t *testing.T) {
	mi, _ := pr("1", "api", 1, "alice")
	stack(&mi, 0)
	f := newFake(mi)
	f.async.polls = 2
	st := newStore()
	r := newRunner(f, st)
	_, ch := r.Resume(context.Background(), []model.Pending{{Key: mi.Key, ID: "1", Head: "h-1", Author: "alice", MergeRequest: "req-acme/api#1"}})
	if e := last(drain(t, ch)); e.Result != Merged {
		t.Errorf("last event = %+v", e)
	}
	if called(f, "merge-async") != 0 || called(f, "merge-status acme/api#1 req-acme/api#1") == 0 {
		t.Errorf("calls = %v", f.calls)
	}
}

// Quitting while GitHub merges keeps the request in the record.
func TestQuittingKeepsAPendingStackedMerge(t *testing.T) {
	mi, it := pr("1", "api", 1, "alice")
	stack(&mi, 0)
	f := newFake(mi)
	f.async.polls = 1 << 20
	st := newStore()
	r := newRunner(f, st)
	ctx, cancel := context.WithCancel(context.Background())
	_, ch := r.Start(ctx, Merge, []Item{it})
	waitFor(t, func() bool { return called(f, "merge-status") > 0 })
	cancel()
	drain(t, ch)
	st.mu.Lock()
	defer st.mu.Unlock()
	if p := st.now[mi.Key]; p.MergeRequest != "req-acme/api#1" {
		t.Errorf("record = %+v", p)
	}
}

// A stop does not end a merge GitHub was asked for: it is followed to its
// result.
func TestAStopLeavesAStackedMergeToGitHub(t *testing.T) {
	mi, it := pr("1", "api", 1, "alice")
	stack(&mi, 0)
	f := newFake(mi)
	f.async.polls = 40
	r := newRunner(f, newStore())
	_, ch := r.Start(context.Background(), Merge, []Item{it})
	waitFor(t, func() bool { return called(f, "merge-status") > 0 })
	if got := r.Stop([]model.PRKey{mi.Key}); len(got) != 0 {
		t.Errorf("stopped %v", got)
	}
	if e := last(drain(t, ch)); e.Result != Merged {
		t.Errorf("last event = %+v", e)
	}
}

// GitHub has no auto-merge for a stack: it waits as without auto-merge.
func TestAStackedPullRequestIsNotPutOnAutoMerge(t *testing.T) {
	mi, it := pr("1", "api", 1, "alice")
	stack(&mi, 0)
	mi.MergeState, mi.Repo.AutoMerge = model.MergeBlocked, true
	mi.Checks = []model.Check{{Name: "build", State: model.CheckSuccess, Required: true}}
	it.AutoMerge = true
	f := newFake(mi)
	evs := runBatch(t, f, Merge, it)
	if e := last(evs[it.PR.Key]); e.Result != Stopped || e.Text != "blocked by another rule" {
		t.Errorf("last event = %+v", e)
	}
	if called(f, "auto-merge") != 0 {
		t.Errorf("calls = %v", f.calls)
	}
}

// The user's own stacked pull request, which gh-desk merges as an admin,
// is merged past the rules the same way through the asynchronous merge.
func TestAStackedAdminMergeBypassesTheRules(t *testing.T) {
	mi, it := pr("1", "api", 1, "me")
	stack(&mi, 0)
	mi.MergeState, mi.AdminMerge = model.MergeBlocked, true
	mi.Checks = []model.Check{{Name: "build", State: model.CheckSuccess, Required: true}}
	f := newFake(mi)
	evs := runBatch(t, f, Merge, it)
	if e := last(evs[it.PR.Key]); e.Result != Merged {
		t.Errorf("last event = %+v", e)
	}
	if called(f, "merge-async acme/api#1 h-1 queue=false bypass=true") != 1 {
		t.Errorf("calls = %v", f.calls)
	}
}
