package runner

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/babarot/gh-desk/internal/model"
)

// gateGH holds a write until the test lets it go, and says when one has
// started.
type gateGH struct {
	*fakeGH
	// on is the write held: "merge", "auto-merge" or "review".
	on      string
	started chan struct{}
	release chan struct{}
}

func newGate(f *fakeGH, on string) *gateGH {
	return &gateGH{fakeGH: f, on: on, started: make(chan struct{}, 8), release: make(chan struct{})}
}

func (g *gateGH) hold(what string) {
	if what == g.on {
		g.started <- struct{}{}
		<-g.release
	}
}

func (g *gateGH) Merge(ctx context.Context, k model.PRKey, id, head string, m model.MergeMethod) error {
	g.hold("merge")
	return g.fakeGH.Merge(ctx, k, id, head, m)
}

func (g *gateGH) EnableAutoMerge(ctx context.Context, k model.PRKey, id, head string, m model.MergeMethod) error {
	g.hold("auto-merge")
	return g.fakeGH.EnableAutoMerge(ctx, k, id, head, m)
}

func (g *gateGH) Review(ctx context.Context, pr model.PR, ev model.ReviewEvent, body string) error {
	g.hold("review")
	return g.fakeGH.Review(ctx, pr, ev, body)
}

func finals(es []Event) []Event {
	var out []Event
	for _, e := range es {
		if e.Result.Final() {
			out = append(out, e)
		}
	}
	return out
}

func (f *fakeGH) wrote() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.calls)
}

func (s *memStore) has(k model.PRKey) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.now[k]
	return ok
}

func (s *memStore) get(k model.PRKey) (model.Pending, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.now[k]
	return p, ok
}

func readsOf(f *fakeGH, id string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.reads[id]
}

// A merge waiting for checks stops at once, writes nothing, says the user
// stopped it, and forgets its record.
func TestStopAWaitingMerge(t *testing.T) {
	mi, it := pr("1", "api", 1, "me")
	f := newFake(mi)
	pending(f, f.prs["1"], 1<<30)
	st := newStore()
	r := newRunner(f, st)
	_, ch := r.Start(context.Background(), Merge, []Item{it})
	waitFor(t, func() bool { return readsOf(f, "1") >= 2 && st.has(it.PR.Key) })
	if got := r.Stop([]model.PRKey{it.PR.Key}); !slices.Equal(got, []model.PRKey{it.PR.Key}) {
		t.Fatalf("Stop = %v", got)
	}
	es := drain(t, ch)
	fs := finals(es)
	if len(fs) != 1 || fs[0].Result != Stopped || !fs[0].StoppedByUser || fs[0].Text != "stopped by you" {
		t.Errorf("final events %+v", fs)
	}
	if len(f.wrote()) != 0 {
		t.Errorf("wrote %v", f.wrote())
	}
	if st.has(it.PR.Key) {
		t.Errorf("the record is kept: %v", st.now)
	}
}

// A stop asked before the job's turn in its batch came ends it without a
// write, and the job before it is not touched.
func TestStopBeforeItsTurn(t *testing.T) {
	a, ia := pr("1", "api", 1, "alice")
	b, ib := pr("2", "api", 2, "alice")
	g := newGate(newFake(a, b), "review")
	r := newRunner(g, newStore())
	_, ch := r.StartReview(context.Background(), ReviewText{Event: model.EventApprove}, []Item{ia, ib})
	<-g.started
	r.Stop([]model.PRKey{ib.PR.Key})
	close(g.release)
	evs := map[model.PRKey][]Event{}
	for _, e := range drain(t, ch) {
		evs[e.Key] = append(evs[e.Key], e)
	}
	if e := last(evs[ia.PR.Key]); e.Result != Approved {
		t.Errorf("api#1: %+v", e)
	}
	if e := last(evs[ib.PR.Key]); e.Result != Stopped || !e.StoppedByUser {
		t.Errorf("api#2: %+v", e)
	}
	if calls := g.wrote(); len(calls) != 1 || !strings.HasPrefix(calls[0], "review acme/api#1") {
		t.Errorf("calls %v", calls)
	}
}

// A write already on its way is sent to the end: the merge is merged, and
// its record is gone as it is after any merge.
func TestStopDuringAWriteLetsItFinish(t *testing.T) {
	mi, it := pr("1", "api", 1, "me")
	g := newGate(newFake(mi), "merge")
	st := newStore()
	r := newRunner(g, st)
	_, ch := r.Start(context.Background(), Merge, []Item{it})
	<-g.started
	if got := r.Stop([]model.PRKey{it.PR.Key}); len(got) != 1 {
		t.Fatalf("Stop = %v", got)
	}
	close(g.release)
	fs := finals(drain(t, ch))
	if len(fs) != 1 || fs[0].Result != Merged || fs[0].StoppedByUser {
		t.Errorf("final events %+v", fs)
	}
	if st.has(it.PR.Key) {
		t.Errorf("record left: %v", st.now)
	}
}

// A stop that comes while the job reads is seen before the write the read
// decided on.
func TestStopDuringAReadStopsBeforeTheWrite(t *testing.T) {
	mi, it := pr("1", "api", 1, "me")
	f := newFake(mi)
	r := newRunner(f, newStore())
	f.onRead = func(_ string, _ *model.MergeInfo, n int) {
		if n == 1 {
			r.Stop([]model.PRKey{it.PR.Key})
		}
	}
	_, ch := r.Start(context.Background(), Merge, []Item{it})
	fs := finals(drain(t, ch))
	if len(fs) != 1 || fs[0].Result != Stopped || !fs[0].StoppedByUser {
		t.Errorf("final events %+v", fs)
	}
	if len(f.wrote()) != 0 {
		t.Errorf("wrote %v", f.wrote())
	}
}

// A read that fails after a stop ends the job without its record: the next
// launch does not go on with what the user stopped.
func TestStopThenAFailedReadForgetsTheRecord(t *testing.T) {
	mi, it := pr("1", "api", 1, "me")
	f := newFake(mi)
	pending(f, f.prs["1"], 1<<30)
	st := newStore()
	r := newRunner(missingAfter{f, 2}, st)
	prev := f.onRead
	f.onRead = func(id string, mi *model.MergeInfo, n int) {
		prev(id, mi, n)
		if n == 3 {
			// This read fails (missingAfter), with the stop already in.
			r.Stop([]model.PRKey{it.PR.Key})
		}
	}
	_, ch := r.Start(context.Background(), Merge, []Item{it})
	fs := finals(drain(t, ch))
	if len(fs) != 1 || fs[0].Result != Failed || !strings.HasSuffix(fs[0].Text, "stopped by you") {
		t.Errorf("final events %+v", fs)
	}
	if st.has(it.PR.Key) {
		t.Errorf("the record is kept: %v", st.now)
	}
}

// A merge waiting for its turn in the lane gives it up when stopped, and
// the merge at work goes on.
func TestStopInTheLaneGivesItUp(t *testing.T) {
	a, ia := pr("1", "api", 1, "me")
	b, ib := pr("2", "api", 2, "me")
	g := newGate(newFake(a, b), "merge")
	r := newRunner(g, newStore())
	_, ch := r.Start(context.Background(), Merge, []Item{ia, ib})
	<-g.started
	waitFor(t, func() bool {
		r.mu.Lock()
		defer r.mu.Unlock()
		return len(r.lanes["acme/api"]) == 2
	})
	r.mu.Lock()
	second := r.lanes["acme/api"][1].key
	r.mu.Unlock()
	r.Stop([]model.PRKey{second})
	waitFor(t, func() bool {
		r.mu.Lock()
		defer r.mu.Unlock()
		return len(r.lanes["acme/api"]) == 1
	})
	close(g.release)
	evs := map[model.PRKey][]Event{}
	for _, e := range drain(t, ch) {
		evs[e.Key] = append(evs[e.Key], e)
	}
	if e := last(evs[second]); e.Result != Stopped || !e.StoppedByUser {
		t.Errorf("%s: %+v", second, e)
	}
	first := ia.PR.Key
	if second == first {
		first = ib.PR.Key
	}
	if e := last(evs[first]); e.Result != Merged {
		t.Errorf("%s: %+v", first, e)
	}
}

// The merge a stopped one was based on no longer holds its child back.
func TestAStoppedParentLetsItsChildGoOn(t *testing.T) {
	a, ia := pr("1", "api", 1, "me")
	b, ib := pr("2", "api", 2, "me")
	branches(&a, "feature", "main")
	branches(&b, "child", "feature")
	f := newFake(a, b)
	pending(f, f.prs["1"], 1<<30)
	r := newRunner(f, newStore())
	_, ch := r.Start(context.Background(), Merge, []Item{ia, ib})
	waitFor(t, func() bool { return readsOf(f, "1") >= 2 })
	r.Stop([]model.PRKey{ia.PR.Key})
	evs := map[model.PRKey][]Event{}
	for _, e := range drain(t, ch) {
		evs[e.Key] = append(evs[e.Key], e)
	}
	if e := last(evs[ia.PR.Key]); e.Result != Stopped || !e.StoppedByUser {
		t.Errorf("api#1: %+v", e)
	}
	merged(t, evs, ib)
}

// An approval waiting for the jobs is ended once, by the goroutine it was
// handed to, and sends nothing.
func TestStopAWaitingApproval(t *testing.T) {
	mi, it := pr("1", "api", 1, "alice")
	mi.Checks = []model.Check{checkRun("test", "IN_PROGRESS", "")}
	f := newFake(mi)
	st := newStore()
	r := newRunner(f, st)
	_, ch := r.StartReview(context.Background(), ReviewText{Event: model.EventApprove}, []Item{approveWait(it)})
	waitFor(t, func() bool { return readsOf(f, "1") >= 3 && st.has(it.PR.Key) })
	r.Stop([]model.PRKey{it.PR.Key})
	fs := finals(drain(t, ch))
	if len(fs) != 1 || fs[0].Result != Stopped || !fs[0].StoppedByUser {
		t.Errorf("final events %+v", fs)
	}
	if len(f.wrote()) != 0 || st.has(it.PR.Key) {
		t.Errorf("wrote %v, record %v", f.wrote(), st.now)
	}
}

// A pull request on auto-merge is GitHub's: a stop leaves its watch alone.
func TestStopLeavesAWatchAlone(t *testing.T) {
	mi, it := pr("1", "api", 1, "renovate")
	mi.MergeState = model.MergeBlocked
	mi.Reviews = []model.Review{{Author: "me", State: model.ReviewApproved}}
	mi.Repo.AutoMerge = true
	it.AutoMerge = true
	f := newFake(mi)
	st := newStore()
	r := newRunner(f, st)
	_, ch := r.Start(context.Background(), Merge, []Item{it})
	waitFor(t, func() bool { p, ok := st.get(it.PR.Key); return ok && p.Watching })
	if got := r.Stop([]model.PRKey{it.PR.Key}); len(got) != 0 {
		t.Errorf("Stop = %v", got)
	}
	f.mu.Lock()
	f.prs["1"].State = model.StateMerged
	f.mu.Unlock()
	if e := watchEnd(t, drain(t, ch)); e.Result != Merged {
		t.Errorf("watch end %+v", e)
	}
}

// A stop that comes while auto-merge is being turned on is too late once
// GitHub took it: the watch goes on, and its record is kept, so quitting
// before the first read of the watch leaves it for the next launch.
func TestStopWhileHandingOver(t *testing.T) {
	mi, it := pr("1", "api", 1, "renovate")
	mi.MergeState = model.MergeBlocked
	mi.Reviews = []model.Review{{Author: "me", State: model.ReviewApproved}}
	mi.Repo.AutoMerge = true
	it.AutoMerge = true
	g := newGate(newFake(mi), "auto-merge")
	st := newStore()
	r := newRunner(g, st)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, ch := r.Start(ctx, Merge, []Item{it})
	<-g.started
	if got := r.Stop([]model.PRKey{it.PR.Key}); len(got) != 1 {
		t.Fatalf("Stop = %v", got)
	}
	close(g.release)
	var es []Event
	for e := range ch {
		es = append(es, e)
		if e.Result == AutoMergeOn {
			break
		}
	}
	if e := last(es); e.Result != AutoMergeOn || e.StoppedByUser {
		t.Fatalf("events %+v", es)
	}
	cancel()
	go func() {
		for range ch {
		}
	}()
	r.Wait()
	if p, ok := st.get(it.PR.Key); !ok || !p.Watching {
		t.Errorf("record %+v, %v", p, ok)
	}
}

// Quitting is not a stop: no event says the user stopped it, and the
// record stays for the next launch.
func TestQuitIsNotAStop(t *testing.T) {
	mi, it := pr("1", "api", 1, "me")
	f := newFake(mi)
	pending(f, f.prs["1"], 1<<30)
	st := newStore()
	r := newRunner(f, st)
	ctx, cancel := context.WithCancel(context.Background())
	_, ch := r.Start(ctx, Merge, []Item{it})
	var es []Event
	done := make(chan struct{})
	go func() {
		for e := range ch {
			es = append(es, e)
		}
		close(done)
	}()
	waitFor(t, func() bool { return readsOf(f, "1") >= 2 })
	cancel()
	r.Wait()
	<-done
	if slices.ContainsFunc(es, func(e Event) bool { return e.StoppedByUser }) {
		t.Errorf("events %+v", es)
	}
	if !st.has(it.PR.Key) {
		t.Errorf("quitting dropped the record")
	}
}

// Stop leaves alone what no job of the runner is on.
func TestStopWhatIsNotAtWork(t *testing.T) {
	r := newRunner(newFake(), newStore())
	if got := r.Stop([]model.PRKey{{Owner: "acme", Repo: "api", Number: 9}}); len(got) != 0 {
		t.Errorf("Stop = %v", got)
	}
}

// A watch resumed from the record is GitHub's from its claim: a stop asked
// before its job began leaves it, and its record, alone.
func TestStopLeavesAResumedWatchAlone(t *testing.T) {
	mi, _ := pr("1", "api", 1, "renovate")
	mi.MergeState, mi.AutoMerge = model.MergeBlocked, true
	f := newFake(mi)
	st := newStore()
	p := model.Pending{Key: mi.Key, ID: "1", Head: "h-1", Author: "renovate", AuthorIsBot: true, AutoMerge: true, Watching: true}
	if err := st.Put(p); err != nil {
		t.Fatal(err)
	}
	r := newRunner(f, st)
	ctx, cancel := context.WithCancel(context.Background())
	_, ch := r.Resume(ctx, []model.Pending{p})
	if got := r.Stop([]model.PRKey{mi.Key}); len(got) != 0 {
		t.Errorf("Stop = %v", got)
	}
	if !st.has(mi.Key) {
		t.Errorf("the record of the watch is gone")
	}
	cancel()
	go func() {
		for range ch {
		}
	}()
	r.Wait()
	if !st.has(mi.Key) {
		t.Errorf("the record of the watch is gone after quitting")
	}
}

// stopOnPut stops a job while its record is written, the moment between
// the last check before a write and the write.
type stopOnPut struct {
	*memStore
	stop func(model.Pending)
}

func (s stopOnPut) Put(p model.Pending) error {
	err := s.memStore.Put(p)
	s.stop(p)
	return err
}

// A stop that comes while the approval is recorded, just before it is
// sent, keeps it from being sent.
func TestStopWhileRecordingTheApproval(t *testing.T) {
	mi, it := pr("1", "api", 1, "alice")
	f := newFake(mi)
	var r *Runner
	st := stopOnPut{memStore: newStore(), stop: func(p model.Pending) {
		if !p.Approving {
			return
		}
		// Stop as it does, from inside save, which holds the stopper's
		// lock: the stop lands between the record and the write.
		r.mu.Lock()
		s := r.stops[p.Key]
		r.mu.Unlock()
		s.stopped = true
		s.cancel()
	}}
	r = newRunner(f, st)
	_, ch := r.Start(context.Background(), Merge, []Item{it})
	fs := finals(drain(t, ch))
	if len(fs) != 1 || fs[0].Result != Stopped || !fs[0].StoppedByUser {
		t.Errorf("final events %+v", fs)
	}
	if len(f.wrote()) != 0 {
		t.Errorf("wrote %v", f.wrote())
	}
}
