package runner

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/babarot/gh-desk/internal/logging"
	"github.com/babarot/gh-desk/internal/model"
	"github.com/babarot/gh-desk/internal/plan"
)

// fakeGH is GitHub as a few pull requests whose state tests change as the
// runner acts on them.
type fakeGH struct {
	mu    sync.Mutex
	prs   map[string]*model.MergeInfo
	calls []string
	// onRead is called on every read, to move a pull request along.
	onRead       func(id string, mi *model.MergeInfo, reads int)
	reads        map[string]int
	autoMergeErr error
	commits      map[string]Commit
	// bodies are the bodies of the reviews sent, by pull request ID.
	bodies map[string]string
}

func (f *fakeGH) log(s string) { f.calls = append(f.calls, s) }

func (f *fakeGH) MergeInfo(ctx context.Context, ids []string, _ bool) (map[string]model.MergeInfo, error) {
	// As the real client does, a canceled read fails.
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	out := map[string]model.MergeInfo{}
	for _, id := range ids {
		mi := f.prs[id]
		f.reads[id]++
		if f.onRead != nil {
			f.onRead(id, mi, f.reads[id])
		}
		out[id] = *mi
	}
	return out, nil
}

func (f *fakeGH) Review(_ context.Context, pr model.PR, ev model.ReviewEvent, body string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.log(fmt.Sprintf("review %s %s %s", pr.Key, ev, pr.HeadOID))
	if f.bodies == nil {
		f.bodies = map[string]string{}
	}
	f.bodies[pr.ID] = body
	state := map[model.ReviewEvent]model.ReviewState{model.EventApprove: model.ReviewApproved,
		model.EventComment: model.ReviewCommented, model.EventRequestChanges: model.ReviewChangesRequested}[ev]
	mi := f.prs[pr.ID]
	mi.Reviews = append(mi.Reviews, model.Review{Author: "me", State: state})
	return nil
}

func (f *fakeGH) Merge(_ context.Context, k model.PRKey, id, head string, m model.MergeMethod) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.log(fmt.Sprintf("merge %s %s %s", k, head, m))
	if f.prs[id].HeadOID != head {
		return fmt.Errorf("head branch was modified")
	}
	f.prs[id].State = model.StateMerged
	return nil
}

func (f *fakeGH) EnableAutoMerge(_ context.Context, k model.PRKey, id, head string, m model.MergeMethod) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.log(fmt.Sprintf("auto-merge %s %s %s", k, head, m))
	if f.autoMergeErr != nil {
		return f.autoMergeErr
	}
	f.prs[id].AutoMerge = true
	return nil
}

func (f *fakeGH) UpdateBranch(_ context.Context, k model.PRKey, id, head string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.log(fmt.Sprintf("update %s %s", k, head))
	mi := f.prs[id]
	mi.HeadOID, mi.MergeState = head+"-updated", model.MergeBlocked
	return nil
}

func (f *fakeGH) MarkReady(_ context.Context, k model.PRKey, id, head string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.log(fmt.Sprintf("ready %s %s", k, head))
	f.prs[id].Draft = false
	return nil
}

func (f *fakeGH) CommitInfo(_ context.Context, _ model.PRKey, oid string) (Commit, error) {
	return f.commits[oid], nil
}

func newFake(mis ...model.MergeInfo) *fakeGH {
	f := &fakeGH{prs: map[string]*model.MergeInfo{}, reads: map[string]int{}, commits: map[string]Commit{}}
	for i := range mis {
		mi := mis[i]
		f.prs[mi.ID] = &mi
	}
	return f
}

var settings = plan.Settings{Methods: []model.MergeMethod{model.MethodMerge, model.MethodSquash}, AutoMerge: true,
	UpdateBranch: "humans", ApprovalChecks: []string{"policy-bot*"}}

func pr(id, repo string, n int, author string) (model.MergeInfo, Item) {
	k := model.PRKey{Owner: "acme", Repo: repo, Number: n}
	mi := model.MergeInfo{Key: k, ID: id, State: model.StateOpen, HeadOID: "h-" + id, BaseOID: "base", MergeState: model.MergeClean,
		Repo: model.RepoMerge{Methods: []model.MergeMethod{model.MethodMerge}}, Checks: []model.Check{}}
	return mi, Item{PR: model.PR{ID: id, Key: k, Author: author, HeadOID: "h-" + id}, Head: "h-" + id}
}

func runBatch(t *testing.T, f *fakeGH, kind Kind, items ...Item) map[model.PRKey][]Event {
	t.Helper()
	return runWith(t, f, func(ctx context.Context, r *Runner) <-chan Event { _, ch := r.Start(ctx, kind, items); return ch })
}

func runReview(t *testing.T, f *fakeGH, rv ReviewText, items ...Item) map[model.PRKey][]Event {
	t.Helper()
	return runWith(t, f, func(ctx context.Context, r *Runner) <-chan Event { _, ch := r.StartReview(ctx, rv, items); return ch })
}

func runWith(t *testing.T, f *fakeGH, start func(context.Context, *Runner) <-chan Event) map[model.PRKey][]Event {
	t.Helper()
	r := New(f, logging.Discard(), Options{Settings: settings, Viewer: "me", CanWait: true,
		ChecksTimeout: time.Second, BlockedTimeout: 40 * time.Millisecond, WatchTimeout: time.Second,
		PollFast: 5 * time.Millisecond, PollSlow: 5 * time.Millisecond, Recheck: 2 * time.Millisecond, RecheckFor: 50 * time.Millisecond})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	events := start(ctx, r)
	out := map[model.PRKey][]Event{}
	for e := range events {
		out[e.Key] = append(out[e.Key], e)
	}
	if ctx.Err() != nil {
		t.Fatal("the batch did not end")
	}
	return out
}

func last(es []Event) Event { return es[len(es)-1] }

func TestMergeNowApprovesFirst(t *testing.T) {
	mi, it := pr("1", "api", 1, "alice")
	f := newFake(mi)
	evs := runBatch(t, f, Merge, it)
	if e := last(evs[it.PR.Key]); e.Result != Merged || !e.Approved {
		t.Errorf("last event = %+v", e)
	}
	if want := "review acme/api#1 APPROVE h-1,merge acme/api#1 h-1 MERGE"; strings.Join(f.calls, ",") != want {
		t.Errorf("calls = %v", f.calls)
	}
}

func TestAutoMergeAndWatch(t *testing.T) {
	mi, it := pr("1", "api", 1, "renovate")
	it.AutoMerge = true
	mi.MergeState, mi.Repo.AutoMerge = model.MergeBlocked, true
	f := newFake(mi)
	f.onRead = func(_ string, mi *model.MergeInfo, n int) {
		if mi.AutoMerge && n > 6 {
			mi.State = model.StateMerged
		}
	}
	evs := runBatch(t, f, Merge, it)
	var auto, merged bool
	for _, e := range evs[it.PR.Key] {
		auto = auto || e.Result == AutoMergeOn
		merged = merged || (e.Watch && e.Result == Merged)
	}
	if !auto || !merged {
		t.Errorf("events = %+v", evs[it.PR.Key])
	}
}

func TestCleanStatusMergesDirectly(t *testing.T) {
	mi, it := pr("1", "api", 1, "renovate")
	it.AutoMerge = true
	mi.MergeState, mi.Repo.AutoMerge = model.MergeBlocked, true
	f := newFake(mi)
	f.autoMergeErr = fmt.Errorf("%w: GitHub said so", ErrCleanStatus)
	evs := runBatch(t, f, Merge, it)
	if e := last(evs[it.PR.Key]); e.Result != Merged {
		t.Errorf("last event = %+v (calls %v)", e, f.calls)
	}
}

func TestWaitForChecksThenMerge(t *testing.T) {
	mi, it := pr("1", "api", 1, "alice")
	mi.MergeState = model.MergeBlocked
	mi.Checks = []model.Check{{Name: "build", State: model.CheckPending, Required: true}}
	f := newFake(mi)
	f.onRead = func(_ string, mi *model.MergeInfo, n int) {
		if n > 5 {
			mi.Checks[0].State, mi.MergeState = model.CheckSuccess, model.MergeClean
		}
	}
	evs := runBatch(t, f, Merge, it)
	es := evs[it.PR.Key]
	if last(es).Result != Merged || !strings.Contains(fmt.Sprint(es), "waiting for checks") {
		t.Errorf("events = %+v", es)
	}
}

func TestHeadChangedStops(t *testing.T) {
	mi, it := pr("1", "api", 1, "alice")
	mi.HeadOID = "pushed-later"
	f := newFake(mi)
	evs := runBatch(t, f, Merge, it)
	if e := last(evs[it.PR.Key]); e.Result != Stopped || !strings.Contains(e.Text, "head changed") {
		t.Errorf("last event = %+v", e)
	}
	if len(f.calls) != 0 {
		t.Errorf("wrote to GitHub: %v", f.calls)
	}
}

func TestBlockedByAnotherRule(t *testing.T) {
	mi, it := pr("1", "api", 1, "alice")
	mi.MergeState = model.MergeBlocked
	mi.Checks = []model.Check{{Name: "build", State: model.CheckSuccess, Required: true}}
	f := newFake(mi)
	evs := runBatch(t, f, Merge, it)
	if e := last(evs[it.PR.Key]); e.Result != Stopped || e.Text != "blocked by another rule" {
		t.Errorf("last event = %+v", e)
	}
}

func TestOneRepositoryAtATime(t *testing.T) {
	a, ia := pr("1", "api", 1, "alice")
	b, ib := pr("2", "api", 2, "alice")
	c, ic := pr("3", "web", 3, "alice")
	f := newFake(a, b, c)
	evs := runBatch(t, f, Merge, ia, ib, ic)
	for _, k := range []model.PRKey{ia.PR.Key, ib.PR.Key, ic.PR.Key} {
		if last(evs[k]).Result != Merged {
			t.Errorf("%s: %+v", k, evs[k])
		}
	}
	if evs[ib.PR.Key][0].Text != "after #1" {
		t.Errorf("api#2 did not wait for api#1: %+v", evs[ib.PR.Key][0])
	}
	pos := func(s string) int {
		for i, c := range f.calls {
			if strings.HasPrefix(c, s) {
				return i
			}
		}
		return -1
	}
	if pos("merge acme/api#1") > pos("review acme/api#2") {
		t.Errorf("api#2 started before api#1 was merged: %v", f.calls)
	}
}

func TestSkips(t *testing.T) {
	dirty, idirty := pr("1", "api", 1, "alice")
	dirty.MergeState = model.MergeDirty
	behind, ibehind := pr("2", "web", 2, "renovate")
	behind.MergeState = model.MergeBehind
	ibehind.PR.AuthorIsBot = true
	f := newFake(dirty, behind)
	evs := runBatch(t, f, Merge, idirty, ibehind)
	if e := last(evs[idirty.PR.Key]); e.Result != Skipped || e.Text != "conflicts" {
		t.Errorf("dirty: %+v", e)
	}
	if e := last(evs[ibehind.PR.Key]); e.Result != Skipped || !strings.Contains(e.Text, "bot will rebase") {
		t.Errorf("behind bot: %+v", e)
	}
	if len(f.calls) != 0 {
		t.Errorf("wrote to GitHub: %v", f.calls)
	}
}

func TestUpdateBranchThenMerge(t *testing.T) {
	mi, it := pr("1", "api", 1, "alice")
	mi.MergeState = model.MergeBehind
	f := newFake(mi)
	f.commits["h-1-updated"] = Commit{Parents: []string{"h-1", "base"}, ViaWeb: true, Author: "me"}
	f.onRead = func(_ string, mi *model.MergeInfo, n int) {
		if mi.HeadOID == "h-1-updated" && n > 8 {
			mi.MergeState = model.MergeClean
		}
	}
	evs := runBatch(t, f, Merge, it)
	if e := last(evs[it.PR.Key]); e.Result != Merged {
		t.Errorf("last event = %+v (calls %v)", e, f.calls)
	}
	if !strings.Contains(strings.Join(f.calls, ","), "merge acme/api#1 h-1-updated") {
		t.Errorf("did not merge the updated head: %v", f.calls)
	}
}

func TestUpdatedHeadWithOtherCommitsStops(t *testing.T) {
	mi, it := pr("1", "api", 1, "alice")
	mi.MergeState = model.MergeBehind
	f := newFake(mi)
	// A push on top of the confirmed head, not GitHub's merge of the base.
	f.commits["h-1-updated"] = Commit{Parents: []string{"h-1"}, Author: "alice"}
	evs := runBatch(t, f, Merge, it)
	if e := last(evs[it.PR.Key]); e.Result != Stopped {
		t.Errorf("last event = %+v", e)
	}
}

func TestApproveOnly(t *testing.T) {
	a, ia := pr("1", "api", 1, "alice")
	b, ib := pr("2", "web", 2, "alice")
	b.Reviews = []model.Review{{Author: "me", State: model.ReviewApproved}}
	f := newFake(a, b)
	evs := runReview(t, f, ReviewText{Event: model.EventApprove, Body: "LGTM"}, ia, ib)
	if e := last(evs[ia.PR.Key]); e.Result != Approved {
		t.Errorf("api#1: %+v", e)
	}
	if e := last(evs[ib.PR.Key]); e.Result != Skipped || e.Text != "already approved" {
		t.Errorf("web#2: %+v", e)
	}
	if strings.Contains(strings.Join(f.calls, ","), "merge") {
		t.Errorf("approve merged: %v", f.calls)
	}
	if f.bodies["1"] != "LGTM" {
		t.Errorf("body sent = %q", f.bodies["1"])
	}
}

// A review of another kind is decided again before it is sent, and sent
// on the head the user confirmed.
func TestReview(t *testing.T) {
	a, ia := pr("1", "api", 1, "alice")
	b, ib := pr("2", "web", 2, "me")
	c, ic := pr("3", "cli", 3, "alice")
	c.State = model.StateMerged
	d, id := pr("4", "app", 4, "alice")
	d.HeadOID = "pushed"
	f := newFake(a, b, c, d)
	evs := runReview(t, f, ReviewText{Event: model.EventRequestChanges, Body: "please rebase"}, ia, ib, ic, id)
	if e := last(evs[ia.PR.Key]); e.Result != Reviewed || e.Approved {
		t.Errorf("api#1: %+v", e)
	}
	for _, it := range []Item{ib, ic} {
		if e := last(evs[it.PR.Key]); e.Result != Skipped {
			t.Errorf("%s: %+v", it.PR.Key, e)
		}
	}
	if e := last(evs[id.PR.Key]); e.Result != Stopped {
		t.Errorf("a pull request pushed to was not stopped: %+v", e)
	}
	if got := strings.Join(f.calls, ","); got != "review acme/api#1 REQUEST_CHANGES h-1" {
		t.Errorf("calls = %s", got)
	}

	// A comment goes to one's own and to a merged pull request.
	f = newFake(b, c)
	evs = runReview(t, f, ReviewText{Event: model.EventComment, Body: "thanks"}, ib, ic)
	for _, it := range []Item{ib, ic} {
		if e := last(evs[it.PR.Key]); e.Result != Reviewed {
			t.Errorf("comment on %s: %+v", it.PR.Key, e)
		}
	}
}

// A pull request confirmed as one to merge directly is not put on
// auto-merge when it turns out blocked when the runner gets to it.
func TestNoAutoMergeWithoutConsent(t *testing.T) {
	mi, it := pr("1", "api", 1, "renovate")
	mi.MergeState, mi.Repo.AutoMerge = model.MergeBlocked, true
	f := newFake(mi)
	f.onRead = func(_ string, mi *model.MergeInfo, n int) {
		if n > 4 {
			mi.MergeState = model.MergeClean
		}
	}
	evs := runBatch(t, f, Merge, it)
	if strings.Contains(strings.Join(f.calls, ","), "auto-merge") {
		t.Errorf("auto-merge turned on without consent: %v", f.calls)
	}
	if e := last(evs[it.PR.Key]); e.Result != Merged {
		t.Errorf("last event = %+v", e)
	}
}

// A watch whose reads keep failing still ends at its time limit.
func TestWatchEndsWhenReadsFail(t *testing.T) {
	mi, it := pr("1", "api", 1, "renovate")
	it.AutoMerge = true
	mi.MergeState, mi.Repo.AutoMerge = model.MergeBlocked, true
	f := newFake(mi)
	r := New(failingReads{f}, logging.Discard(), Options{Settings: settings, Viewer: "me", CanWait: true, ChecksTimeout: time.Second,
		BlockedTimeout: time.Second, WatchTimeout: 30 * time.Millisecond, PollFast: 5 * time.Millisecond, PollSlow: 5 * time.Millisecond,
		Recheck: 2 * time.Millisecond, RecheckFor: 50 * time.Millisecond})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, events := r.Start(ctx, Merge, []Item{it})
	for range events {
	}
	if ctx.Err() != nil {
		t.Fatal("the watch did not end")
	}
}

// failingReads fails every read after auto-merge is on.
type failingReads struct{ *fakeGH }

func (f failingReads) MergeInfo(ctx context.Context, ids []string, details bool) (map[string]model.MergeInfo, error) {
	f.mu.Lock()
	on := f.prs[ids[0]].AutoMerge
	f.mu.Unlock()
	if on {
		return nil, fmt.Errorf("401 Bad credentials")
	}
	return f.fakeGH.MergeInfo(ctx, ids, details)
}

// A merge commit of the confirmed head with something other than the base
// read before the update is not taken, even when GitHub made it.
func TestOldMergeCommitIsNotTaken(t *testing.T) {
	mi, it := pr("1", "api", 1, "alice")
	mi.MergeState = model.MergeBehind
	f := newFake(mi)
	f.commits["h-1-updated"] = Commit{Parents: []string{"h-1", "old-base"}, ViaWeb: true, Author: "me"}
	evs := runBatch(t, f, Merge, it)
	if e := last(evs[it.PR.Key]); e.Result != Stopped {
		t.Errorf("last event = %+v (calls %v)", e, f.calls)
	}
	if strings.Contains(strings.Join(f.calls, ","), "merge acme") {
		t.Errorf("merged: %v", f.calls)
	}
}

// memStore is a Store in memory that remembers whether it ever held a
// pull request.
type memStore struct {
	mu   sync.Mutex
	now  map[model.PRKey]model.Pending
	ever map[model.PRKey]bool
}

func newStore() *memStore {
	return &memStore{now: map[model.PRKey]model.Pending{}, ever: map[model.PRKey]bool{}}
}

func (s *memStore) Put(p model.Pending) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.now[p.Key], s.ever[p.Key] = p, true
	return nil
}

func (s *memStore) Remove(k model.PRKey) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.now, k)
	return nil
}

func newRunner(f GitHub, st Store) *Runner {
	return New(f, logging.Discard(), Options{Settings: settings, Viewer: "me", Store: st, CanWait: true,
		ChecksTimeout: time.Second, BlockedTimeout: 40 * time.Millisecond, WatchTimeout: time.Second,
		PollFast: 5 * time.Millisecond, PollSlow: 5 * time.Millisecond, Recheck: 2 * time.Millisecond, RecheckFor: 50 * time.Millisecond})
}

func drain(t *testing.T, ch <-chan Event) []Event {
	t.Helper()
	var out []Event
	done := time.After(3 * time.Second)
	for {
		select {
		case e, ok := <-ch:
			if !ok {
				return out
			}
			out = append(out, e)
		case <-done:
			t.Fatal("the batch did not end")
		}
	}
}

func TestRecordedWhileWaitingForgottenAfter(t *testing.T) {
	mi, it := pr("1", "api", 1, "alice")
	mi.MergeState = model.MergeBlocked
	mi.Checks = []model.Check{{Name: "build", State: model.CheckPending, Required: true}}
	f := newFake(mi)
	f.onRead = func(_ string, mi *model.MergeInfo, n int) {
		if n > 5 {
			mi.Checks[0].State, mi.MergeState = model.CheckSuccess, model.MergeClean
		}
	}
	st := newStore()
	_, ch := newRunner(f, st).Start(context.Background(), Merge, []Item{it})
	es := drain(t, ch)
	if last(es).Result != Merged || !st.ever[it.PR.Key] || len(st.now) != 0 {
		t.Errorf("events %+v, recorded %v, left %v", es, st.ever, st.now)
	}
}

// A job made before the runner adopts the record keeps going without it:
// it skips the wait rather than wait unrecorded. One made after waits and
// records.
func TestAdoptReachesOnlyLaterJobs(t *testing.T) {
	mi, it := pr("1", "api", 1, "alice")
	mi.MergeState = model.MergeBlocked
	mi.Checks = []model.Check{{Name: "build", State: model.CheckPending, Required: true}}
	f := newFake(mi)
	r := newRunner(f, nil)
	r.canWait = false
	st := newStore()
	f.onRead = func(_ string, mi *model.MergeInfo, n int) {
		if n == 1 {
			// The other gh-desk quits while the job reads.
			r.Adopt(st)
		}
		if n > 5 {
			mi.Checks[0].State, mi.MergeState = model.CheckSuccess, model.MergeClean
		}
	}
	_, ch := r.Start(context.Background(), Merge, []Item{it})
	es := drain(t, ch)
	if e := last(es); e.Result != Skipped || !e.Locked || st.ever[it.PR.Key] {
		t.Fatalf("before adopting: events %+v, recorded %v", es, st.ever)
	}
	if len(f.calls) != 0 {
		t.Fatalf("wrote to GitHub: %v", f.calls)
	}
	_, ch = r.Start(context.Background(), Merge, []Item{it})
	es = drain(t, ch)
	if last(es).Result != Merged || !st.ever[it.PR.Key] || len(st.now) != 0 {
		t.Errorf("after adopting: events %+v, recorded %v, left %v", es, st.ever, st.now)
	}
}

func TestResumeWatching(t *testing.T) {
	mi, _ := pr("1", "api", 1, "renovate")
	mi.MergeState, mi.AutoMerge = model.MergeBlocked, true
	f := newFake(mi)
	f.onRead = func(_ string, mi *model.MergeInfo, n int) {
		if n > 3 {
			mi.State = model.StateMerged
		}
	}
	st := newStore()
	p := model.Pending{Key: mi.Key, ID: "1", Head: "h-1", Author: "renovate", AuthorIsBot: true, AutoMerge: true, Watching: true}
	if err := st.Put(p); err != nil {
		t.Fatal(err)
	}
	_, ch := newRunner(f, st).Resume(context.Background(), []model.Pending{p})
	es := drain(t, ch)
	if e := last(es); !e.Watch || e.Result != Merged || len(st.now) != 0 {
		t.Errorf("events %+v, left %v", es, st.now)
	}
	if len(f.calls) != 0 {
		t.Errorf("wrote to GitHub: %v", f.calls)
	}
}

func TestResumeStops(t *testing.T) {
	moved, _ := pr("1", "api", 1, "alice")
	moved.HeadOID = "pushed-while-closed"
	dismissed, _ := pr("2", "web", 2, "alice")
	dismissed.MergeState = model.MergeBlocked
	f := newFake(moved, dismissed)
	ps := []model.Pending{
		{Key: moved.Key, ID: "1", Head: "h-1", Author: "alice"},
		{Key: dismissed.Key, ID: "2", Head: "h-2", Author: "alice", Approved: true},
	}
	_, ch := newRunner(f, newStore()).Resume(context.Background(), ps)
	byKey := map[model.PRKey]Event{}
	for _, e := range drain(t, ch) {
		byKey[e.Key] = e
	}
	if e := byKey[moved.Key]; e.Result != Stopped || !strings.Contains(e.Text, "head changed") {
		t.Errorf("moved: %+v", e)
	}
	if e := byKey[dismissed.Key]; e.Result != Stopped || !strings.Contains(e.Text, "dismissed") {
		t.Errorf("dismissed: %+v", e)
	}
	if len(f.calls) != 0 {
		t.Errorf("wrote to GitHub: %v", f.calls)
	}
}

func TestResumeKeepsTimeWaited(t *testing.T) {
	mi, _ := pr("1", "api", 1, "alice")
	mi.MergeState = model.MergeBlocked
	mi.Reviews = []model.Review{{Author: "me", State: model.ReviewApproved}}
	mi.Checks = []model.Check{{Name: "build", State: model.CheckSuccess, Required: true}}
	f := newFake(mi)
	// Blocked 39ms of the 40ms limit before the last run ended: it stops
	// at once instead of waiting 40ms more.
	p := model.Pending{Key: mi.Key, ID: "1", Head: "h-1", Author: "alice", Approved: true, BlockedWaited: 39 * time.Millisecond}
	start := time.Now()
	_, ch := newRunner(f, newStore()).Resume(context.Background(), []model.Pending{p})
	es := drain(t, ch)
	if e := last(es); e.Result != Stopped || e.Text != "blocked by another rule" {
		t.Errorf("last = %+v", e)
	}
	if time.Since(start) > 35*time.Millisecond {
		t.Errorf("waited %v; the time already waited was not kept", time.Since(start))
	}
}

// A failed read keeps the record, so the next launch goes on.
func TestFailedReadKeepsTheRecord(t *testing.T) {
	mi, it := pr("1", "api", 1, "alice")
	mi.MergeState = model.MergeBlocked
	mi.Checks = []model.Check{{Name: "build", State: model.CheckPending, Required: true}}
	f := newFake(mi)
	st := newStore()
	_, ch := newRunner(missingAfter{f, 3}, st).Start(context.Background(), Merge, []Item{it})
	drain(t, ch)
	if _, ok := st.now[it.PR.Key]; !ok {
		t.Errorf("the record went with a failed read: %v", st.now)
	}
}

// missingAfter stops returning the pull request after n reads.
type missingAfter struct {
	*fakeGH
	n int
}

func (f missingAfter) MergeInfo(ctx context.Context, ids []string, details bool) (map[string]model.MergeInfo, error) {
	got, err := f.fakeGH.MergeInfo(ctx, ids, details)
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.reads[ids[0]] > f.n {
		return map[string]model.MergeInfo{}, err
	}
	return got, err
}

// A record that cannot be written stops the job before any write.
func TestNoWriteWithoutARecord(t *testing.T) {
	mi, it := pr("1", "api", 1, "alice")
	f := newFake(mi)
	_, ch := newRunner(f, failingStore{}).Start(context.Background(), Merge, []Item{it})
	es := drain(t, ch)
	if e := last(es); e.Result != Failed || len(f.calls) != 0 {
		t.Errorf("last %+v, calls %v", e, f.calls)
	}
}

type failingStore struct{}

func (failingStore) Put(model.Pending) error  { return fmt.Errorf("disk full") }
func (failingStore) Remove(model.PRKey) error { return nil }

// Without the record's lock, a merge that turns out to wait is skipped.
func TestNoWaitWithoutTheLock(t *testing.T) {
	mi, it := pr("1", "api", 1, "alice")
	mi.MergeState = model.MergeBlocked
	f := newFake(mi)
	r := New(f, logging.Discard(), Options{Settings: settings, Viewer: "me", PollFast: time.Millisecond,
		Recheck: time.Millisecond, RecheckFor: 10 * time.Millisecond})
	_, ch := r.Start(context.Background(), Merge, []Item{it})
	es := drain(t, ch)
	if e := last(es); e.Result != Skipped || len(f.calls) != 0 {
		t.Errorf("last %+v, calls %v", e, f.calls)
	}
}

// An approval whose result was not recorded is not sent again on resume.
func TestResumeUnknownApproval(t *testing.T) {
	mi, _ := pr("1", "api", 1, "alice")
	mi.MergeState = model.MergeBlocked
	f := newFake(mi)
	p := model.Pending{Key: mi.Key, ID: "1", Head: "h-1", Author: "alice", Approving: true}
	_, ch := newRunner(f, newStore()).Resume(context.Background(), []model.Pending{p})
	es := drain(t, ch)
	if e := last(es); e.Result != Stopped || !strings.Contains(e.Text, "not known") || len(f.calls) != 0 {
		t.Errorf("last %+v, calls %v", e, f.calls)
	}
}

// Quitting keeps the record, and Wait returns once the batch has ended.
func TestWaitAfterCancel(t *testing.T) {
	mi, it := pr("1", "api", 1, "alice")
	mi.MergeState = model.MergeBlocked
	mi.Checks = []model.Check{{Name: "build", State: model.CheckPending, Required: true}}
	f := newFake(mi)
	st := newStore()
	r := newRunner(f, st)
	ctx, cancel := context.WithCancel(context.Background())
	_, ch := r.Start(ctx, Merge, []Item{it})
	go func() {
		for range ch {
		}
	}()
	time.Sleep(20 * time.Millisecond)
	cancel()
	done := make(chan struct{})
	go func() { r.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Wait did not return")
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	if _, ok := st.now[it.PR.Key]; !ok {
		t.Errorf("quitting dropped the record: %v", st.now)
	}
}

// A resumed watch does not hold back the next pull request of its
// repository.
func TestResumedWatchDoesNotBlock(t *testing.T) {
	a, _ := pr("1", "api", 1, "renovate")
	a.MergeState, a.AutoMerge = model.MergeBlocked, true
	b, _ := pr("2", "api", 2, "alice")
	f := newFake(a, b)
	ps := []model.Pending{
		{Key: a.Key, ID: "1", Head: "h-1", Author: "renovate", AuthorIsBot: true, AutoMerge: true, Watching: true},
		{Key: b.Key, ID: "2", Head: "h-2", Author: "alice"},
	}
	r := New(f, logging.Discard(), Options{Settings: settings, Viewer: "me", Store: newStore(), CanWait: true,
		ChecksTimeout: time.Second, BlockedTimeout: time.Second, WatchTimeout: 300 * time.Millisecond,
		PollFast: 5 * time.Millisecond, PollSlow: 5 * time.Millisecond, Recheck: 2 * time.Millisecond, RecheckFor: 50 * time.Millisecond})
	start := time.Now()
	_, ch := r.Resume(context.Background(), ps)
	var mergedAt time.Duration
	for e := range ch {
		if e.Key == b.Key && e.Result == Merged {
			mergedAt = time.Since(start)
		}
	}
	if mergedAt == 0 || mergedAt > 200*time.Millisecond {
		t.Errorf("api#2 merged after %v; it waited for api#1's watch", mergedAt)
	}
}

// An approval the user gave before the merge is recorded, so a dismissal
// while gh-desk was closed stops it instead of approving again.
func TestApprovalBeforeMergeIsRecorded(t *testing.T) {
	mi, it := pr("1", "api", 1, "alice")
	mi.MergeState = model.MergeBlocked
	mi.Reviews = []model.Review{{Author: "me", State: model.ReviewApproved}}
	mi.Checks = []model.Check{{Name: "build", State: model.CheckPending, Required: true}}
	f := newFake(mi)
	st := newStore()
	ctx, cancel := context.WithCancel(context.Background())
	r := newRunner(f, st)
	_, first := r.Start(ctx, Merge, []Item{it})
	go func() {
		for range first {
		}
	}()
	time.Sleep(20 * time.Millisecond)
	cancel()
	r.Wait()
	st.mu.Lock()
	p, ok := st.now[it.PR.Key]
	st.mu.Unlock()
	if !ok || !p.Approved {
		t.Fatalf("record %+v, kept %v", p, ok)
	}

	f.prs["1"].Reviews = nil
	_, ch := newRunner(f, st).Resume(context.Background(), []model.Pending{p})
	es := drain(t, ch)
	if e := last(es); e.Result != Stopped || !strings.Contains(e.Text, "dismissed") {
		t.Errorf("last %+v", e)
	}
	for _, c := range f.calls {
		if strings.HasPrefix(c, "review") {
			t.Errorf("approved again: %v", f.calls)
		}
	}
}

// A pull request already watched is not taken by a second batch, whose
// record would be overwritten and removed by the watch.
func TestOneJobPerPullRequest(t *testing.T) {
	mi, it := pr("1", "api", 1, "renovate")
	mi.MergeState, mi.Repo.AutoMerge = model.MergeBlocked, true
	it.AutoMerge = true
	f := newFake(mi)
	st := newStore()
	r := newRunner(f, st)
	ctx := t.Context()
	_, first := r.Start(ctx, Merge, []Item{it})
	for e := range first {
		if e.Result == AutoMergeOn {
			break
		}
	}
	go func() {
		for range first {
		}
	}()
	_, second := r.Start(ctx, Merge, []Item{it})
	es := drain(t, second)
	if e := last(es); e.Result != Skipped || !strings.Contains(e.Text, "already") {
		t.Errorf("second batch: %+v", es)
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	if p, ok := st.now[it.PR.Key]; !ok || !p.Watching {
		t.Errorf("record %+v, kept %v", p, ok)
	}
}

// An approval dismissed between the reads of a resumed merge stops it too.
func TestDismissedBetweenReads(t *testing.T) {
	mi, _ := pr("1", "api", 1, "alice")
	mi.MergeState = model.MergeBlocked
	mi.Reviews = []model.Review{{Author: "me", State: model.ReviewApproved}}
	f := newFake(mi)
	f.onRead = func(_ string, mi *model.MergeInfo, n int) {
		if n > 1 {
			mi.Reviews = nil
		}
	}
	p := model.Pending{Key: mi.Key, ID: "1", Head: "h-1", Author: "alice", Approved: true}
	_, ch := newRunner(f, newStore()).Resume(context.Background(), []model.Pending{p})
	es := drain(t, ch)
	if e := last(es); e.Result != Stopped || !strings.Contains(e.Text, "dismissed") || len(f.calls) != 0 {
		t.Errorf("last %+v, calls %v", e, f.calls)
	}
}

// An approval dismissed while waiting stops the merge even if the pull
// request turns mergeable.
func TestDismissedWhileWaiting(t *testing.T) {
	mi, it := pr("1", "api", 1, "alice")
	mi.MergeState = model.MergeBlocked
	mi.Reviews = []model.Review{{Author: "me", State: model.ReviewApproved}}
	mi.Checks = []model.Check{{Name: "build", State: model.CheckPending, Required: true}}
	f := newFake(mi)
	f.onRead = func(_ string, mi *model.MergeInfo, n int) {
		if n > 4 {
			mi.Reviews, mi.Checks[0].State, mi.MergeState = nil, model.CheckSuccess, model.MergeClean
		}
	}
	_, ch := newRunner(f, newStore()).Start(context.Background(), Merge, []Item{it})
	es := drain(t, ch)
	if e := last(es); e.Result != Stopped || !strings.Contains(e.Text, "dismissed") || len(f.calls) != 0 {
		t.Errorf("last %+v, calls %v", e, f.calls)
	}
}

func TestReadyOnly(t *testing.T) {
	draft, it := pr("1", "api", 1, "me")
	draft.Draft, draft.MergeState = true, model.MergeDraft
	ready, it2 := pr("2", "web", 2, "me")
	moved, it3 := pr("3", "cli", 3, "me")
	moved.Draft, moved.HeadOID = true, "pushed-later"
	f := newFake(draft, ready, moved)
	evs := runBatch(t, f, Ready, it, it2, it3)
	if e := last(evs[it.PR.Key]); e.Result != Readied {
		t.Errorf("draft: last event = %+v", e)
	}
	if e := last(evs[it2.PR.Key]); e.Result != Skipped || e.Text != "not a draft" {
		t.Errorf("not a draft: last event = %+v", e)
	}
	if e := last(evs[it3.PR.Key]); e.Result != Stopped || !strings.Contains(e.Text, "head changed") {
		t.Errorf("moved: last event = %+v", e)
	}
	if want := "ready acme/api#1 h-1"; strings.Join(f.calls, ",") != want {
		t.Errorf("calls = %v", f.calls)
	}
}

// A draft goes to its merge marked ready first, then approved when it is
// not the viewer's.
func TestMergeDraftReadiesFirst(t *testing.T) {
	mine, it := pr("1", "api", 1, "me")
	bots, it2 := pr("2", "web", 2, "renovate")
	it2.PR.AuthorIsBot = true
	for _, mi := range []*model.MergeInfo{&mine, &bots} {
		mi.Draft, mi.MergeState = true, model.MergeDraft
	}
	f := newFake(mine, bots)
	f.onRead = func(_ string, mi *model.MergeInfo, _ int) {
		if !mi.Draft && mi.MergeState == model.MergeDraft {
			mi.MergeState = model.MergeClean
		}
	}
	evs := runBatch(t, f, Merge, it, it2)
	if e := last(evs[it.PR.Key]); e.Result != Merged || !e.Readied || e.Approved {
		t.Errorf("own: last event = %+v", e)
	}
	if e := last(evs[it2.PR.Key]); e.Result != Merged || !e.Readied || !e.Approved {
		t.Errorf("bot's: last event = %+v", e)
	}
	// The two repositories run side by side, so only the order within
	// each pull request is fixed.
	for pr, want := range map[string]string{
		"acme/api#1": "ready acme/api#1 h-1,merge acme/api#1 h-1 MERGE",
		"acme/web#2": "ready acme/web#2 h-2,review acme/web#2 APPROVE h-2,merge acme/web#2 h-2 MERGE",
	} {
		var got []string
		for _, c := range f.calls {
			if strings.Contains(c, " "+pr+" ") {
				got = append(got, c)
			}
		}
		if strings.Join(got, ",") != want {
			t.Errorf("%s calls = %v", pr, got)
		}
	}
}

// A draft gh-desk marked ready and found a draft again was turned back by
// someone: it is not marked ready again.
func TestRedraftedStops(t *testing.T) {
	mi, it := pr("1", "api", 1, "me")
	mi.Draft, mi.MergeState = true, model.MergeDraft
	f := newFake(mi)
	f.onRead = func(_ string, mi *model.MergeInfo, _ int) {
		// Turned back at once, as if by the author.
		mi.Draft = true
	}
	_, ch := newRunner(f, newStore()).Start(context.Background(), Merge, []Item{it})
	es := drain(t, ch)
	if e := last(es); e.Result != Stopped || !strings.Contains(e.Text, "draft again") {
		t.Errorf("last %+v", e)
	}
	if want := "ready acme/api#1 h-1"; strings.Join(f.calls, ",") != want {
		t.Errorf("calls = %v", f.calls)
	}

	p := model.Pending{Key: mi.Key, ID: "1", Head: "h-1", Author: "me", Readied: true}
	f.calls = nil
	_, ch = newRunner(f, newStore()).Resume(context.Background(), []model.Pending{p})
	if e := last(drain(t, ch)); e.Result != Stopped || len(f.calls) != 0 {
		t.Errorf("resumed: last %+v, calls %v", e, f.calls)
	}
}
