package runner

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/babarot/gh-desk/internal/logging"
	"github.com/babarot/gh-desk/internal/model"
)

// readsGH counts the reads, and fails them when told to.
type readsGH struct {
	*fakeGH
	mu    sync.Mutex
	reads [][]string
	fail  error
}

func (g *readsGH) MergeInfo(ctx context.Context, ids []string, full bool) (map[string]model.MergeInfo, error) {
	g.mu.Lock()
	g.reads = append(g.reads, slices.Clone(ids))
	err := g.fail
	g.mu.Unlock()
	if err != nil {
		return nil, err
	}
	return g.fakeGH.MergeInfo(ctx, ids, full)
}

// full is how many reads asked for all three pull requests.
func (g *readsGH) full() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	n := 0
	for _, ids := range g.reads {
		if len(ids) == 3 {
			n++
		}
	}
	return n
}

func (g *readsGH) snapshot() [][]string {
	g.mu.Lock()
	defer g.mu.Unlock()
	return slices.Clone(g.reads)
}

func (g *readsGH) count() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return len(g.reads)
}

func readsRunner(g GitHub) *Runner {
	return New(g, logging.Discard(), Options{Settings: settings, Viewer: "me"})
}

func atWork(id string) (model.MergeInfo, *waiter) {
	mi, _ := pr(id, "api", 1, "alice")
	mi.Rollup = model.CheckPending
	return mi, sleeper(mi, true)
}

func sleeper(mi model.MergeInfo, light bool) *waiter {
	now := time.Now()
	return &waiter{id: mi.ID, head: mi.HeadOID, light: light, every: time.Millisecond, due: now.Add(time.Millisecond),
		force: now.Add(time.Hour), wake: make(chan struct{})}
}

// await sleeps w in a batch of its own, and reports on the channel.
func await(t *testing.T, r *Runner, ctx context.Context, w *waiter) <-chan bool {
	t.Helper()
	r.active.Add(1)
	t.Cleanup(r.active.Done)
	woken := make(chan bool, 1)
	go func() { woken <- r.awaitRead(ctx, "acme/api", w) }()
	return woken
}

func wokenOf(t *testing.T, ch <-chan bool, want bool) {
	t.Helper()
	select {
	case got := <-ch:
		if got != want {
			t.Fatalf("awaitRead = %v, want %v", got, want)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("not woken")
	}
}

func TestTheWaitsOfARepositoryShareARead(t *testing.T) {
	a, wa := atWork("1")
	b, wb := atWork("2")
	c, wc := atWork("3")
	g := &readsGH{fakeGH: newFake(a, b, c)}
	r := readsRunner(g)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ca, cb, cc := await(t, r, ctx, wa), await(t, r, ctx, wb), await(t, r, ctx, wc)
	// The waits start one by one, and the first reads can come before the
	// last of them: once all three are there, every read has them all.
	deadline := time.Now().Add(3 * time.Second)
	for g.full() < 5 {
		if time.Now().After(deadline) {
			t.Fatalf("reads were not shared: %v", g.snapshot())
		}
		time.Sleep(time.Millisecond)
	}
	select {
	case <-ca:
		t.Fatal("woken while its jobs are at work")
	case <-cb:
		t.Fatal("woken while its jobs are at work")
	case <-cc:
		t.Fatal("woken while its jobs are at work")
	default:
	}
	reads := g.snapshot()
	first := slices.IndexFunc(reads, func(ids []string) bool { return len(ids) == 3 })
	for _, ids := range reads[first:] {
		if len(ids) != 3 {
			t.Errorf("a read after all three waited was not shared: %v", reads)
			break
		}
	}

	g.fakeGH.mu.Lock()
	g.prs["2"].Rollup = model.CheckSuccess
	g.fakeGH.mu.Unlock()
	wokenOf(t, cb, true)
	select {
	case <-ca:
		t.Fatal("woken for another pull request")
	default:
	}
	cancel()
	wokenOf(t, ca, false)
	wokenOf(t, cc, false)
	r.mu.Lock()
	defer r.mu.Unlock()
	for len(r.reads) > 0 {
		r.mu.Unlock()
		time.Sleep(time.Millisecond)
		r.mu.Lock()
	}
}

func TestAWaitTheLightReadCannotTellIsWokenUnread(t *testing.T) {
	mi, _ := pr("1", "api", 1, "alice")
	mi.Rollup = model.CheckPending
	g := &readsGH{fakeGH: newFake(mi)}
	r := readsRunner(g)
	wokenOf(t, await(t, r, context.Background(), sleeper(mi, false)), true)
	if g.count() != 0 {
		t.Errorf("read for a wait it cannot tell: %v", g.reads)
	}
}

func TestLightReadWakes(t *testing.T) {
	cases := map[string]func(*model.MergeInfo){
		"head moved": func(mi *model.MergeInfo) { mi.HeadOID = "other" },
		"merged":     func(mi *model.MergeInfo) { mi.State = model.StateMerged },
		"failed":     func(mi *model.MergeInfo) { mi.Rollup = model.CheckFailure },
		"no checks":  func(mi *model.MergeInfo) { mi.Rollup = "" },
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			mi, w := atWork("1")
			change(&mi)
			r := readsRunner(&readsGH{fakeGH: newFake(mi)})
			wokenOf(t, await(t, r, context.Background(), w), true)
		})
	}
}

func TestAFailedReadWakesThemAll(t *testing.T) {
	a, wa := atWork("1")
	b, wb := atWork("2")
	g := &readsGH{fakeGH: newFake(a, b), fail: errors.New("boom")}
	r := readsRunner(g)
	ctx := context.Background()
	ca, cb := await(t, r, ctx, wa), await(t, r, ctx, wb)
	wokenOf(t, ca, true)
	wokenOf(t, cb, true)
}

func TestAWaitIsWokenInTheEnd(t *testing.T) {
	mi, w := atWork("1")
	w.force = time.Now().Add(20 * time.Millisecond)
	r := readsRunner(&readsGH{fakeGH: newFake(mi)})
	wokenOf(t, await(t, r, context.Background(), w), true)
}

// A commit status that waits for an approval keeps the rollup pending after
// the jobs end, so such a pull request is not left to the light read.
func TestOnlyJobsAtWork(t *testing.T) {
	job := checkRun("test", "IN_PROGRESS", "")
	done := checkRun("lint", model.RunCompleted, model.CheckSuccess)
	waiting := checkRun("deploy", model.RunWaiting, "")
	status := model.Check{Name: "policy-bot", State: model.CheckPending, Kind: model.KindStatus}
	passed := model.Check{Name: "ci", State: model.CheckSuccess, Kind: model.KindStatus}
	cases := []struct {
		name string
		mi   model.MergeInfo
		want bool
	}{
		{"a job", model.MergeInfo{Checks: []model.Check{job, done, passed}}, true},
		{"nothing at work", model.MergeInfo{Checks: []model.Check{done}}, false},
		{"no checks", model.MergeInfo{}, false},
		{"a status waits", model.MergeInfo{Checks: []model.Check{job, status}}, false},
		{"an environment waits", model.MergeInfo{Checks: []model.Check{job, waiting}}, false},
		{"more than was read", model.MergeInfo{Checks: []model.Check{job}, ChecksPartial: true}, false},
	}
	for _, c := range cases {
		if got := onlyJobsAtWork(c.mi); got != c.want {
			t.Errorf("%s: %v", c.name, got)
		}
	}
}

// A merge whose job is at work leaves the reads to the light read, and goes
// on once the rollup turns.
func TestAMergeWaitsOnTheLightRead(t *testing.T) {
	mi, it := pr("1", "api", 1, "me")
	mi.MergeState, mi.Rollup = model.MergeUnstable, model.CheckPending
	mi.Checks = []model.Check{checkRun("test", "IN_PROGRESS", "")}
	f := newFake(mi)
	f.onRead = func(_ string, mi *model.MergeInfo, n int) {
		if n == 20 {
			mi.Rollup, mi.MergeState = model.CheckSuccess, model.MergeClean
			mi.Checks = []model.Check{checkRun("test", model.RunCompleted, model.CheckSuccess)}
		}
	}
	evs := runBatch(t, f, Merge, it)
	if e := last(evs[it.PR.Key]); e.Result != Merged {
		t.Errorf("last event = %+v (calls %v)", e, f.calls)
	}
}
