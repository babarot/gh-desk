package runner

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

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

func TestAStoppedBaseLetsItsChildGoOn(t *testing.T) {
	a, ia := pr("1", "api", 1, "me")
	b, ib := pr("2", "api", 2, "me")
	branches(&a, "feature", "main")
	branches(&b, "child", "feature")
	a.MergeState = model.MergeDirty
	f := newFake(a, b)
	evs := runBatch(t, f, Merge, ia, ib)
	if e := last(evs[ia.PR.Key]); e.Result != Skipped {
		t.Errorf("api#1: %+v", e)
	}
	merged(t, evs, ib)
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
