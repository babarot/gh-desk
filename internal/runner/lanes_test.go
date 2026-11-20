package runner

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/babarot/gh-desk/internal/logging"
	"github.com/babarot/gh-desk/internal/model"
)

// overlapGH notes when two merges of a repository are written at once.
type overlapGH struct {
	*fakeGH
	mu      sync.Mutex
	writing map[string]int
	overlap bool
}

func (g *overlapGH) Merge(ctx context.Context, k model.PRKey, id, head string, m model.MergeMethod) error {
	g.mu.Lock()
	if g.writing == nil {
		g.writing = map[string]int{}
	}
	g.writing[k.Repository()]++
	g.overlap = g.overlap || g.writing[k.Repository()] > 1
	g.mu.Unlock()
	time.Sleep(2 * time.Millisecond)
	err := g.fakeGH.Merge(ctx, k, id, head, m)
	g.mu.Lock()
	g.writing[k.Repository()]--
	g.mu.Unlock()
	return err
}

func pos(f *fakeGH, s string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.IndexFunc(f.calls, func(c string) bool { return strings.HasPrefix(c, s) })
}

func merged(t *testing.T, evs map[model.PRKey][]Event, items ...Item) {
	t.Helper()
	for _, it := range items {
		if e := last(evs[it.PR.Key]); e.Result != Merged {
			t.Errorf("%s: %+v", it.PR.Key, e)
		}
	}
}

func branches(mi *model.MergeInfo, head, base string) {
	mi.Head.Name, mi.BaseRef = head, base
}

func drainAll(t *testing.T, chs ...<-chan Event) map[model.PRKey][]Event {
	t.Helper()
	var mu sync.Mutex
	var wg sync.WaitGroup
	evs := map[model.PRKey][]Event{}
	for _, ch := range chs {
		wg.Go(func() {
			for _, ev := range drain(t, ch) {
				mu.Lock()
				evs[ev.Key] = append(evs[ev.Key], ev)
				mu.Unlock()
			}
		})
	}
	wg.Wait()
	return evs
}

func waitFor(t *testing.T, ok func() bool) {
	t.Helper()
	for range 3000 {
		if ok() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("it never happened")
}

// Merges of a repository go side by side up to the write, which goes one at
// a time; another repository does not wait for them.
func TestMergesOfARepositoryDoNotOverlap(t *testing.T) {
	var mis []model.MergeInfo
	var items []Item
	for i, id := range []string{"1", "2", "3", "4", "5"} {
		mi, it := pr(id, "api", i+1, "me")
		mis, items = append(mis, mi), append(items, it)
	}
	g := &overlapGH{fakeGH: newFake(mis...)}
	r := newRunner(g, newStore())
	_, ch := r.Start(context.Background(), Merge, items)
	evs := drainAll(t, ch)
	merged(t, evs, items...)
	if g.overlap {
		t.Errorf("merges overlapped: %v", g.calls)
	}
}

func TestAReadyMergePassesOneAtWork(t *testing.T) {
	a, ia := pr("1", "api", 1, "me")
	b, ib := pr("2", "api", 2, "me")
	f := newFake(a, b)
	pending(f, f.prs["1"], 20)
	evs := runBatch(t, f, Merge, ia, ib)
	merged(t, evs, ia, ib)
	if pos(f, "merge acme/api#2") > pos(f, "merge acme/api#1") {
		t.Errorf("api#2 waited for api#1: %v", f.calls)
	}
}

func TestAReadyMergePassesOneAtWorkAcrossBatches(t *testing.T) {
	a, ia := pr("1", "api", 1, "me")
	b, ib := pr("2", "api", 2, "me")
	f := newFake(a, b)
	pending(f, f.prs["1"], 20)
	r := newRunner(f, newStore())
	ctx := context.Background()
	_, first := r.Start(ctx, Merge, []Item{ia})
	_, second := r.Start(ctx, Merge, []Item{ib})
	merged(t, drainAll(t, first, second), ia, ib)
	if pos(f, "merge acme/api#2") > pos(f, "merge acme/api#1") {
		t.Errorf("api#2 waited for api#1: %v", f.calls)
	}
}

// The jobs that end first merge first, whatever the order of the batch.
func TestMergesGoInTheOrderTheirJobsEnd(t *testing.T) {
	a, ia := pr("1", "api", 1, "me")
	b, ib := pr("2", "api", 2, "me")
	f := newFake(a, b)
	pending(f, f.prs["1"], 40)
	pending(f, f.prs["2"], 10)
	evs := runBatch(t, f, Merge, ia, ib)
	merged(t, evs, ia, ib)
	if pos(f, "merge acme/api#2") > pos(f, "merge acme/api#1") {
		t.Errorf("api#2 waited for api#1: %v", f.calls)
	}
}

// Approvals and other repositories go aside from merges that wait.
func TestAWaitingMergeHoldsNothingUp(t *testing.T) {
	a, ia := pr("1", "api", 1, "me")
	b, ib := pr("2", "web", 2, "me")
	c, ic := pr("3", "api", 3, "alice")
	f := newFake(a, b, c)
	pending(f, f.prs["1"], 1<<30)
	r := newRunner(f, newStore())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, first := r.Start(ctx, Merge, []Item{ia})
	_, second := r.Start(context.Background(), Merge, []Item{ib})
	_, third := r.StartReview(context.Background(), ReviewText{Event: model.EventApprove}, []Item{ic})
	evs := drainAll(t, second, third)
	merged(t, evs, ib)
	if e := last(evs[ic.PR.Key]); e.Result != Approved {
		t.Errorf("api#3: %+v", e)
	}
	cancel()
	drainAll(t, first)
}

// Jobs found in its turn send a merge back to wait, off the lane.
func TestJobsFoundInItsTurnGiveUpTheLane(t *testing.T) {
	a, ia := pr("1", "api", 1, "me")
	f := newFake(a)
	f.onRead = func(_ string, mi *model.MergeInfo, n int) {
		switch n {
		case 3:
			mi.Checks = []model.Check{checkRun("late", "IN_PROGRESS", "")}
		case 8:
			mi.Checks = []model.Check{checkRun("late", model.RunCompleted, model.CheckSuccess)}
		}
	}
	r := newRunner(f, newStore())
	_, ch := r.Start(context.Background(), Merge, []Item{ia})
	waitFor(t, func() bool {
		f.mu.Lock()
		defer f.mu.Unlock()
		return f.reads["1"] >= 5
	})
	r.mu.Lock()
	held := len(r.lanes)
	r.mu.Unlock()
	if held != 0 {
		t.Errorf("the lane is held while jobs are at work")
	}
	merged(t, drainAll(t, ch), ia)
	if f.reads["1"] < 8 {
		t.Errorf("merged with a job at work: %v", f.calls)
	}
}

// A merge that updated its branch keeps the lane, so another merge does not
// leave it behind again.
func TestAnUpdatedBranchKeepsTheLane(t *testing.T) {
	a, ia := pr("1", "api", 1, "me")
	b, ib := pr("2", "api", 2, "me")
	a.MergeState = model.MergeBehind
	f := newFake(a, b)
	f.commits["h-1-updated"] = Commit{Parents: []string{"h-1", "base"}, ViaWeb: true, Author: "me"}
	var updated time.Time
	f.onRead = func(id string, mi *model.MergeInfo, _ int) {
		if id != "1" || mi.HeadOID != "h-1-updated" {
			return
		}
		if updated.IsZero() {
			updated = time.Now()
		}
		if time.Since(updated) < 60*time.Millisecond {
			mi.MergeState = model.MergeUnstable
			mi.Checks = []model.Check{checkRun("test", "IN_PROGRESS", "")}
			return
		}
		mi.MergeState = model.MergeClean
		mi.Checks = []model.Check{checkRun("test", model.RunCompleted, model.CheckSuccess)}
	}
	r := newRunner(f, newStore())
	ctx := context.Background()
	_, first := r.Start(ctx, Merge, []Item{ia})
	waitFor(t, func() bool { return pos(f, "update acme/api#1") >= 0 })
	_, second := r.Start(ctx, Merge, []Item{ib})
	merged(t, drainAll(t, first, second), ia, ib)
	if pos(f, "merge acme/api#1") > pos(f, "merge acme/api#2") {
		t.Errorf("api#2 passed an updated branch: %v", f.calls)
	}
}

func TestAMergeWaitsForTheMergeItIsBasedOn(t *testing.T) {
	a, ia := pr("1", "api", 1, "me")
	b, ib := pr("2", "api", 2, "me")
	branches(&a, "feature", "main")
	branches(&b, "child", "feature")
	f := newFake(a, b)
	pending(f, f.prs["1"], 15)
	evs := runBatch(t, f, Merge, ib, ia)
	merged(t, evs, ia, ib)
	if pos(f, "merge acme/api#1") > pos(f, "merge acme/api#2") {
		t.Errorf("the child went first: %v", f.calls)
	}
}

func TestAMergeWaitsForItsBaseAcrossBatches(t *testing.T) {
	a, ia := pr("1", "api", 1, "me")
	b, ib := pr("2", "api", 2, "me")
	branches(&a, "feature", "main")
	branches(&b, "child", "feature")
	f := newFake(a, b)
	pending(f, f.prs["1"], 15)
	r := newRunner(f, newStore())
	ctx := context.Background()
	_, first := r.Start(ctx, Merge, []Item{ia})
	_, second := r.Start(ctx, Merge, []Item{ib})
	merged(t, drainAll(t, first, second), ia, ib)
	if pos(f, "merge acme/api#1") > pos(f, "merge acme/api#2") {
		t.Errorf("the child went first: %v", f.calls)
	}
}

// A resume keeps no branch names: it reads them.
func TestAResumedMergeWaitsForItsBase(t *testing.T) {
	a, _ := pr("1", "api", 1, "me")
	b, _ := pr("2", "api", 2, "me")
	branches(&a, "feature", "main")
	branches(&b, "child", "feature")
	f := newFake(a, b)
	pending(f, f.prs["1"], 15)
	st := newStore()
	ps := []model.Pending{{Key: b.Key, ID: "2", Head: "h-2", Author: "me"}, {Key: a.Key, ID: "1", Head: "h-1", Author: "me"}}
	r := newRunner(f, st)
	_, ch := r.Resume(context.Background(), ps)
	es := drain(t, ch)
	if len(es) == 0 || pos(f, "merge acme/api#1") < 0 || pos(f, "merge acme/api#1") > pos(f, "merge acme/api#2") {
		t.Errorf("the child went first: %v", f.calls)
	}
}

// A base that ended without a merge stops its child before it is merged
// into the base's branch, even when the base ended while the child still
// waited for its jobs (ADR-131).
func TestAStoppedBaseStopsItsChild(t *testing.T) {
	a, ia := pr("1", "api", 1, "me")
	b, ib := pr("2", "api", 2, "me")
	branches(&a, "feature", "main")
	branches(&b, "child", "feature")
	a.MergeState = model.MergeDirty
	f := newFake(a, b)
	pending(f, f.prs["2"], 5)
	evs := runBatch(t, f, Merge, ia, ib)
	if e := last(evs[ia.PR.Key]); e.Result != Skipped {
		t.Errorf("api#1: %+v", e)
	}
	if e := last(evs[ib.PR.Key]); e.Result != Stopped || !strings.Contains(e.Text, "#1 did not merge") {
		t.Errorf("api#2: %+v", e)
	}
	if pos(f, "merge acme/api#2") >= 0 {
		t.Errorf("the child was merged: %v", f.calls)
	}
}

// A base whose merge GitHub refused stops its child too.
func TestAFailedBaseStopsItsChild(t *testing.T) {
	a, ia := pr("1", "api", 1, "me")
	b, ib := pr("2", "api", 2, "me")
	branches(&a, "feature", "main")
	branches(&b, "child", "feature")
	f := newFake(a, b)
	f.mergeErrs = []error{errors.New("GitHub said no")}
	evs := runBatch(t, f, Merge, ia, ib)
	if e := last(evs[ia.PR.Key]); e.Result != Failed {
		t.Errorf("api#1: %+v", e)
	}
	if e := last(evs[ib.PR.Key]); e.Result != Stopped || !strings.Contains(e.Text, "#1 did not merge") {
		t.Errorf("api#2: %+v (calls %v)", e, f.calls)
	}
}

// A base merged whose branch stays, so GitHub does not retarget the child,
// stops the child: merged, it would land on a branch nobody merges.
func TestAMergedBaseNotRetargetedStopsItsChild(t *testing.T) {
	a, ia := pr("1", "api", 1, "me")
	b, ib := pr("2", "api", 2, "me")
	branches(&a, "feature", "main")
	branches(&b, "child", "feature")
	f := newFake(a, b)
	f.keepBranches = true
	evs := runBatch(t, f, Merge, ia, ib)
	merged(t, evs, ia)
	if e := last(evs[ib.PR.Key]); e.Result != Stopped || !strings.Contains(e.Text, "still targets") {
		t.Errorf("api#2: %+v (calls %v)", e, f.calls)
	}
}

// GitHub retargets a child a moment after its base is merged: a read
// before that does not stop it.
func TestAChildRetargetedLateIsMerged(t *testing.T) {
	a, ia := pr("1", "api", 1, "me")
	b, ib := pr("2", "api", 2, "me")
	branches(&a, "feature", "main")
	branches(&b, "child", "feature")
	f := newFake(a, b)
	f.keepBranches = true
	late := 0
	f.onRead = func(id string, mi *model.MergeInfo, _ int) {
		if id == "2" && f.prs["1"].State == model.StateMerged {
			if late++; late >= 3 {
				mi.BaseRef = "main"
			}
		}
	}
	merged(t, runBatch(t, f, Merge, ia, ib), ia, ib)
}

// A child whose base ended unmerged while its jobs ran is not approved:
// the stop comes before any write (ADR-131).
func TestNoApprovalForAChildOfAFailedBase(t *testing.T) {
	a, ia := pr("1", "api", 1, "me")
	b, ib := pr("2", "api", 2, "alice")
	branches(&a, "feature", "main")
	branches(&b, "child", "feature")
	b.MergeState = model.MergeBlocked
	b.Checks = []model.Check{checkRun("build", "IN_PROGRESS", "")}
	f := newFake(a, b)
	f.onRead = func(id string, mi *model.MergeInfo, n int) {
		if id == "2" && n > 15 {
			mi.Checks = []model.Check{checkRun("build", model.RunCompleted, model.CheckSuccess)}
		}
	}
	pending(f, f.prs["1"], 2)
	f.mergeErrs = []error{errors.New("GitHub said no")}
	evs := runBatch(t, f, Merge, ia, ib)
	if e := last(evs[ib.PR.Key]); e.Result != Stopped || !strings.Contains(e.Text, "#1 did not merge") {
		t.Errorf("api#2: %+v (calls %v)", e, f.calls)
	}
	if pos(f, "review acme/api#2") >= 0 {
		t.Errorf("the child was approved: %v", f.calls)
	}
}

// A child whose auto-merge waits only for the approval is not approved
// while its base is being merged: the approval would merge it into the
// base's branch at once, whatever the base's merge comes to (ADR-131).
func TestNoApprovalWhileTheBaseIsAtWork(t *testing.T) {
	for _, tt := range []struct {
		name   string
		fails  bool
		result Result
	}{
		{"base fails", true, Stopped},
		{"base merges", false, Merged},
	} {
		t.Run(tt.name, func(t *testing.T) {
			a, ia := pr("1", "api", 1, "me")
			b, ib := pr("2", "api", 2, "alice")
			branches(&a, "feature", "main")
			branches(&b, "child", "feature")
			b.AutoMerge = true
			f := newFake(a, b)
			f.autoMergeOnApproval = true
			pending(f, f.prs["1"], 8)
			if tt.fails {
				f.mergeErrs = []error{errors.New("GitHub said no")}
			}
			evs := runBatch(t, f, Merge, ia, ib)
			if e := last(evs[ib.PR.Key]); e.Result != tt.result {
				t.Errorf("api#2: %+v (calls %v)", e, f.calls)
			}
			if pos(f, "auto-merged acme/api#2 into feature") >= 0 {
				t.Errorf("the child was merged into its base's branch: %v", f.calls)
			}
			if r := pos(f, "review acme/api#2"); r >= 0 && r < pos(f, "merge acme/api#1") {
				t.Errorf("the child was approved before its base's merge ended: %v", f.calls)
			}
		})
	}
}

// Marking a draft child ready waits for its base too: with an approval in
// already, it could let auto-merge merge it.
func TestNoReadyWhileTheBaseIsAtWork(t *testing.T) {
	a, ia := pr("1", "api", 1, "me")
	b, ib := pr("2", "api", 2, "me")
	branches(&a, "feature", "main")
	branches(&b, "child", "feature")
	b.Draft = true
	f := newFake(a, b)
	pending(f, f.prs["1"], 8)
	f.mergeErrs = []error{errors.New("GitHub said no")}
	evs := runBatch(t, f, Merge, ia, ib)
	if e := last(evs[ib.PR.Key]); e.Result != Stopped || !strings.Contains(e.Text, "#1 did not merge") {
		t.Errorf("api#2: %+v (calls %v)", e, f.calls)
	}
	if pos(f, "ready acme/api#2") >= 0 {
		t.Errorf("the child was marked ready: %v", f.calls)
	}
}

// A child confirmed alone, its base merged by no merge of gh-desk's, is
// merged into its base's branch as before.
func TestAChildAloneMergesIntoItsBase(t *testing.T) {
	a, _ := pr("1", "api", 1, "me")
	b, ib := pr("2", "api", 2, "me")
	branches(&a, "feature", "main")
	branches(&b, "child", "feature")
	f := newFake(a, b)
	merged(t, runBatch(t, f, Merge, ib), ib)
}

// stacked makes mi a pull request blocked on its base, the head of the
// open pull request n, its required checks done.
func stacked(mi *model.MergeInfo, n int) {
	mi.BasePR, mi.MergeState = n, model.MergeBlocked
	mi.Checks = []model.Check{{Name: "build", State: model.CheckSuccess, Required: true}}
}

// A resumed merge blocked on a base no merge is at work on stops at once,
// and its record goes: waiting would not unblock it.
func TestAResumedMergeBlockedOnAnOpenBaseStops(t *testing.T) {
	b, _ := pr("2", "api", 2, "me")
	branches(&b, "child", "feature")
	stacked(&b, 1)
	f := newFake(b)
	st := newStore()
	st.now[b.Key] = model.Pending{Key: b.Key, ID: "2", Head: "h-2", Author: "me"}
	r := New(f, logging.Discard(), Options{Settings: settings, Viewer: "me", Store: st, CanWait: true,
		WaitTimeout: time.Second, BlockedTimeout: time.Minute, WatchTimeout: time.Second,
		PollFast: 5 * time.Millisecond, PollSlow: 5 * time.Millisecond, Recheck: 2 * time.Millisecond, RecheckFor: 50 * time.Millisecond, Quiet: 20 * time.Millisecond})
	_, ch := r.Resume(context.Background(), []model.Pending{st.now[b.Key]})
	es := drain(t, ch)
	if e := last(es); e.Result != Stopped || e.Text != "its base #1 is not merged · merge that one first" {
		t.Errorf("last event = %+v", e)
	}
	if len(f.calls) != 0 {
		t.Errorf("wrote to GitHub: %v", f.calls)
	}
	if _, ok := st.now[b.Key]; ok {
		t.Error("the record stayed")
	}
}

// A base that ended without a merge in the same batch stops its stacked
// child, which would otherwise wait out merge.blocked_timeout, as one left
// behind (ADR-131).
func TestABaseThatDidNotMergeStopsItsBlockedChild(t *testing.T) {
	a, ia := pr("1", "api", 1, "me")
	b, ib := pr("2", "api", 2, "me")
	branches(&a, "feature", "main")
	branches(&b, "child", "feature")
	a.MergeState = model.MergeDirty
	stacked(&b, 1)
	f := newFake(a, b)
	evs := runBatch(t, f, Merge, ia, ib)
	if e := last(evs[ia.PR.Key]); e.Result != Skipped {
		t.Errorf("api#1: %+v", e)
	}
	if e := last(evs[ib.PR.Key]); e.Result != Stopped || !strings.Contains(e.Text, "#1 did not merge") {
		t.Errorf("api#2: %+v", e)
	}
}

// While its base is being merged, a blocked stacked child waits, and goes
// on once the merge retargets it.
func TestABlockedChildWaitsWhileItsBaseIsAtWork(t *testing.T) {
	a, ia := pr("1", "api", 1, "me")
	b, ib := pr("2", "api", 2, "me")
	branches(&a, "feature", "main")
	branches(&b, "child", "feature")
	stacked(&b, 1)
	f := newFake(a, b)
	pending(f, f.prs["1"], 6)
	prev := f.onRead
	f.onRead = func(id string, mi *model.MergeInfo, n int) {
		prev(id, mi, n)
		if id == "2" && f.prs["1"].State == model.StateMerged {
			mi.BasePR, mi.BaseRef, mi.MergeState = 0, "main", model.MergeClean
		}
	}
	r := New(f, logging.Discard(), Options{Settings: settings, Viewer: "me", Store: newStore(), CanWait: true,
		WaitTimeout: time.Second, BlockedTimeout: time.Minute, WatchTimeout: time.Second,
		PollFast: 5 * time.Millisecond, PollSlow: 5 * time.Millisecond, Recheck: 2 * time.Millisecond, RecheckFor: 50 * time.Millisecond, Quiet: 20 * time.Millisecond})
	_, ch := r.Start(context.Background(), Merge, []Item{ib, ia})
	merged(t, drainAll(t, ch), ia, ib)
}

func TestAMergeWaitsForABaseOnAutoMerge(t *testing.T) {
	a, ia := pr("1", "api", 1, "me")
	b, ib := pr("2", "api", 2, "me")
	branches(&a, "feature", "main")
	branches(&b, "child", "feature")
	a.MergeState = model.MergeBlocked
	a.Repo.AutoMerge = true
	ia.AutoMerge = true
	f := newFake(a, b)
	f.onRead = func(id string, mi *model.MergeInfo, n int) {
		if id == "1" && mi.AutoMerge && n > 12 {
			mi.State = model.StateMerged
		}
	}
	evs := runBatch(t, f, Merge, ib, ia)
	merged(t, evs, ib)
	if pos(f, "auto-merge acme/api#1") < 0 || pos(f, "auto-merge acme/api#1") > pos(f, "merge acme/api#2") {
		t.Errorf("the child did not wait: %v", f.calls)
	}
	if f.prs["1"].State != model.StateMerged {
		t.Errorf("the child went on before its base was merged: %v", f.calls)
	}
}

func TestBasesThatLeadBackStop(t *testing.T) {
	a, ia := pr("1", "api", 1, "me")
	b, ib := pr("2", "api", 2, "me")
	branches(&a, "one", "two")
	branches(&b, "two", "one")
	f := newFake(a, b)
	evs := runBatch(t, f, Merge, ia, ib)
	stopped := 0
	for _, it := range []Item{ia, ib} {
		if last(evs[it.PR.Key]).Result == Stopped {
			stopped++
		}
	}
	// The one that stops leaves the other free.
	if stopped == 0 || len(f.calls) > 1 {
		t.Errorf("events %+v, calls %v", evs, f.calls)
	}
}

// A branch of a fork is not the branch of that name here.
func TestABaseIsNotAForkBranchOfThatName(t *testing.T) {
	a, ia := pr("1", "api", 1, "me")
	b, ib := pr("2", "api", 2, "me")
	branches(&a, "feature", "main")
	branches(&b, "child", "feature")
	a.HeadInFork = true
	f := newFake(a, b)
	pending(f, f.prs["1"], 20)
	evs := runBatch(t, f, Merge, ia, ib)
	merged(t, evs, ia, ib)
	if pos(f, "merge acme/api#2") > pos(f, "merge acme/api#1") {
		t.Errorf("api#2 waited for a fork's branch: %v", f.calls)
	}
}

// failFirstGH fails the first read of a pull request.
type failFirstGH struct {
	*fakeGH
	id   string
	once sync.Once
}

func (g *failFirstGH) MergeInfo(ctx context.Context, ids []string, full bool) (map[string]model.MergeInfo, error) {
	var err error
	if slices.Contains(ids, g.id) {
		g.once.Do(func() { err = errors.New("boom") })
	}
	if err != nil {
		return nil, err
	}
	return g.fakeGH.MergeInfo(ctx, ids, full)
}

// Branches that cannot be read stop the merge unwritten, its record kept,
// and do not hold up another batch.
func TestUnreadBranchesWriteNothing(t *testing.T) {
	a, _ := pr("1", "api", 1, "me")
	b, ib := pr("2", "api", 2, "me")
	f := newFake(a, b)
	st := newStore()
	p := model.Pending{Key: a.Key, ID: "1", Head: "h-1", Author: "me"}
	if err := st.Put(p); err != nil {
		t.Fatal(err)
	}
	r := newRunner(&failFirstGH{fakeGH: f, id: "1"}, st)
	_, resumed := r.Resume(context.Background(), []model.Pending{p})
	_, later := r.Start(context.Background(), Merge, []Item{ib})
	evs := drainAll(t, resumed, later)
	if e := last(evs[a.Key]); e.Result != Failed || !strings.Contains(e.Text, "next launch") {
		t.Errorf("api#1: %+v", e)
	}
	if st.now[a.Key].Key != a.Key {
		t.Errorf("the record was lost: %v", st.now)
	}
	if pos(f, "merge acme/api#1") >= 0 {
		t.Errorf("wrote to GitHub: %v", f.calls)
	}
	merged(t, evs, ib)
}

// A merge confirmed while its child waits in the lane still goes first.
func TestABaseConfirmedWhileItsChildWaitsInTheLane(t *testing.T) {
	x, ix := pr("9", "api", 9, "me")
	a, ia := pr("1", "api", 1, "me")
	b, ib := pr("2", "api", 2, "me")
	branches(&a, "feature", "main")
	branches(&b, "child", "feature")
	x.MergeState = model.MergeBehind
	f := newFake(x, a, b)
	f.commits["h-9-updated"] = Commit{Parents: []string{"h-9", "base"}, ViaWeb: true, Author: "me"}
	var updated time.Time
	f.onRead = func(id string, mi *model.MergeInfo, _ int) {
		if id != "9" || mi.HeadOID != "h-9-updated" {
			return
		}
		if updated.IsZero() {
			updated = time.Now()
		}
		if time.Since(updated) < 80*time.Millisecond {
			mi.MergeState = model.MergeUnstable
			mi.Checks = []model.Check{checkRun("test", "IN_PROGRESS", "")}
			return
		}
		mi.MergeState = model.MergeClean
		mi.Checks = []model.Check{checkRun("test", model.RunCompleted, model.CheckSuccess)}
	}
	pending(f, f.prs["1"], 30)
	r := newRunner(f, newStore())
	ctx := context.Background()
	_, holder := r.Start(ctx, Merge, []Item{ix})
	waitFor(t, func() bool { return pos(f, "update acme/api#9") >= 0 })
	_, child := r.Start(ctx, Merge, []Item{ib})
	waitFor(t, func() bool {
		r.mu.Lock()
		defer r.mu.Unlock()
		return len(r.lanes["acme/api"]) == 2
	})
	_, parent := r.Start(ctx, Merge, []Item{ia})
	merged(t, drainAll(t, holder, child, parent), ix, ia, ib)
	if pos(f, "merge acme/api#1") > pos(f, "merge acme/api#2") {
		t.Errorf("the child went first: %v", f.calls)
	}
}
