package runner

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/babarot/gh-desk/internal/logging"
	"github.com/babarot/gh-desk/internal/model"
)

func checkRun(name string, run model.RunStatus, conclusion model.CheckState) model.Check {
	st := conclusion
	if run != model.RunCompleted {
		st = model.CheckPending
	}
	return model.Check{Name: name, State: st, Kind: model.KindCheckRun, Run: run}
}

// As gh-desk#47 was: no check is required and the tests are at work. The
// merge waits for them, then merges.
func TestMergeWaitsForJobsNotRequired(t *testing.T) {
	mi, it := pr("1", "api", 1, "me")
	mi.MergeState = model.MergeUnstable
	mi.Checks = []model.Check{checkRun("test (ubuntu-latest)", "IN_PROGRESS", "")}
	f := newFake(mi)
	f.onRead = func(_ string, mi *model.MergeInfo, n int) {
		if n > 3 {
			mi.Checks, mi.MergeState = []model.Check{checkRun("test (ubuntu-latest)", model.RunCompleted, model.CheckSuccess)}, model.MergeClean
		}
	}
	events := runBatch(t, f, Merge, it)[it.PR.Key]
	if e := last(events); e.Result != Merged {
		t.Fatalf("last event = %+v (calls %v)", e, f.calls)
	}
	waited := slices.ContainsFunc(events, func(e Event) bool {
		return e.Step == StepWaitChecks && slices.Equal(e.Checks, []string{"test (ubuntu-latest)"})
	})
	if !waited {
		t.Errorf("did not say it waits for the job: %+v", events)
	}
	if f.reads["1"] <= 3 {
		t.Errorf("merged after %d reads, before the job ended", f.reads["1"])
	}
}

// A merge that has to approve approves once the jobs passed, not before:
// an approval is not taken back.
func TestMergeApprovesAfterTheJobs(t *testing.T) {
	mi, it := pr("1", "api", 1, "alice")
	mi.MergeState = model.MergeBlocked
	mi.Checks = []model.Check{checkRun("build", "IN_PROGRESS", "")}
	f := newFake(mi)
	var reviewedAt int
	f.onRead = func(_ string, mi *model.MergeInfo, n int) {
		if reviewedAt == 0 && slices.ContainsFunc(f.calls, func(c string) bool { return strings.HasPrefix(c, "review ") }) {
			reviewedAt = n
		}
		if n > 3 {
			mi.Checks = []model.Check{checkRun("build", model.RunCompleted, model.CheckSuccess)}
		}
		if reviewedAt > 0 {
			mi.MergeState = model.MergeClean
		}
	}
	if e := last(runBatch(t, f, Merge, it)[it.PR.Key]); e.Result != Merged || !e.Approved {
		t.Fatalf("last event = %+v (calls %v)", e, f.calls)
	}
	if reviewedAt <= 4 {
		t.Errorf("approved by read %d, before the job ended at read 4", reviewedAt)
	}
}

// A job that fails while the merge waits stops it before the approval.
func TestMergeWithAFailedJobDoesNotApprove(t *testing.T) {
	mi, it := pr("1", "api", 1, "alice")
	mi.MergeState = model.MergeBlocked
	mi.Checks = []model.Check{checkRun("build", "IN_PROGRESS", "")}
	f := newFake(mi)
	f.onRead = func(_ string, mi *model.MergeInfo, n int) {
		if n > 3 {
			mi.Checks = []model.Check{checkRun("build", model.RunCompleted, model.CheckFailure)}
		}
	}
	e := last(runBatch(t, f, Merge, it)[it.PR.Key])
	if e.Result != Skipped || e.Text != "a job failed: build" {
		t.Errorf("last event = %+v", e)
	}
	if len(f.calls) > 0 {
		t.Errorf("wrote %v", f.calls)
	}
}

// Marking a draft ready may start workflows of its own, on top of the runs
// of its opening: the merge waits the quiet time, sees the job that
// starts, and merges after it.
func TestMergeWaitsForJobsStartedByReady(t *testing.T) {
	mi, it := pr("1", "api", 1, "me")
	mi.Draft, mi.MergeState = true, model.MergeDraft
	f := newFake(mi)
	var readyAt int
	f.onRead = func(_ string, mi *model.MergeInfo, n int) {
		if mi.Draft {
			return
		}
		if readyAt == 0 {
			readyAt = n
			mi.MergeState = model.MergeClean
		}
		// The workflow run of ready for review shows a read later, and
		// its job ends two reads after.
		switch n - readyAt {
		case 1, 2:
			mi.Checks = []model.Check{checkRun("test", "QUEUED", "")}
		case 3:
			mi.Checks = []model.Check{checkRun("test", model.RunCompleted, model.CheckSuccess)}
		}
	}
	if e := last(runBatch(t, f, Merge, it)[it.PR.Key]); e.Result != Merged {
		t.Fatalf("last event = %+v (calls %v)", e, f.calls)
	}
	if n := f.reads["1"] - readyAt; n < 3 {
		t.Errorf("merged %d reads after ready, before the job it started ended", n)
	}
}

// A merge resumed at launch waits the quiet time even when the head has
// runs: gh-desk may have marked it ready just before it quit, and the
// workflows that starts may not show yet.
func TestResumeWaitsForJobsNotShownYet(t *testing.T) {
	mi, _ := pr("1", "api", 1, "me")
	f := newFake(mi)
	f.onRead = func(_ string, mi *model.MergeInfo, n int) {
		// Read 1 resumes and read 2 decides; the job shows after.
		switch n {
		case 3, 4:
			mi.Checks = []model.Check{checkRun("test", "QUEUED", "")}
		case 5:
			mi.Checks = []model.Check{checkRun("test", model.RunCompleted, model.CheckSuccess)}
		}
	}
	st := newStore()
	p := model.Pending{Key: mi.Key, ID: "1", Head: "h-1", Author: "me", Readied: true}
	if err := st.Put(p); err != nil {
		t.Fatal(err)
	}
	_, ch := newRunner(f, st).Resume(t.Context(), []model.Pending{p})
	if e := last(drain(t, ch)); e.Result != Merged {
		t.Fatalf("last event = %+v (calls %v)", e, f.calls)
	}
	if f.reads["1"] < 5 {
		t.Errorf("merged after %d reads, before the job not shown at first ended", f.reads["1"])
	}
}

func approveWait(it Item) Item {
	it.Wait = true
	return it
}

// An approval asked to wait is sent once the jobs at work on the head
// passed, and recorded while it waits.
func TestApprovalWaitsForJobs(t *testing.T) {
	mi, it := pr("1", "api", 1, "alice")
	mi.Checks = []model.Check{checkRun("test", "IN_PROGRESS", "")}
	f := newFake(mi)
	st := newStore()
	f.onRead = func(_ string, mi *model.MergeInfo, n int) {
		if n == 2 && len(st.now) == 0 {
			t.Error("not recorded while it waits")
		}
		if n > 3 {
			mi.Checks = []model.Check{checkRun("test", model.RunCompleted, model.CheckSuccess)}
		}
	}
	_, ch := newRunner(f, st).StartReview(t.Context(), ReviewText{Event: model.EventApprove}, []Item{approveWait(it)})
	es := drain(t, ch)
	if e := last(es); e.Result != Approved {
		t.Fatalf("last event = %+v (calls %v)", e, f.calls)
	}
	if f.reads["1"] <= 3 {
		t.Errorf("approved after %d reads, before the job ended", f.reads["1"])
	}
	if !slices.ContainsFunc(es, func(e Event) bool { return e.Step == StepWaitChecks && slices.Equal(e.Checks, []string{"test"}) }) {
		t.Errorf("did not say it waits for the job: %+v", es)
	}
	if len(st.now) != 0 {
		t.Errorf("record left: %v", st.now)
	}
}

// A job that fails while the approval waits stops it, unless the user let
// failing ones in.
func TestApprovalWithAFailedJob(t *testing.T) {
	for _, include := range []bool{false, true} {
		mi, it := pr("1", "api", 1, "alice")
		mi.Checks = []model.Check{checkRun("lint", "IN_PROGRESS", "")}
		f := newFake(mi)
		f.onRead = func(_ string, mi *model.MergeInfo, n int) {
			if n > 2 {
				mi.Checks = []model.Check{checkRun("lint", model.RunCompleted, model.CheckFailure)}
			}
		}
		it = approveWait(it)
		it.IncludeFailing = include
		e := last(runReview(t, f, ReviewText{Event: model.EventApprove}, it)[it.PR.Key])
		switch {
		case !include && (e.Result != Skipped || e.Text != "a job failed: lint" || len(f.calls) > 0):
			t.Errorf("failing left out: last event = %+v, calls %v", e, f.calls)
		case include && e.Result != Approved:
			t.Errorf("failing let in: last event = %+v", e)
		}
	}
}

// An approval with a body is sent at once: the body is not kept on disk
// while it would wait.
func TestApprovalWithABodyDoesNotWait(t *testing.T) {
	mi, it := pr("1", "api", 1, "alice")
	mi.Checks = []model.Check{checkRun("test", "IN_PROGRESS", "")}
	f := newFake(mi)
	e := last(runReview(t, f, ReviewText{Event: model.EventApprove, Body: "LGTM"}, approveWait(it))[it.PR.Key])
	if e.Result != Approved || f.reads["1"] != 1 {
		t.Errorf("last event = %+v after %d reads", e, f.reads["1"])
	}
}

// Another gh-desk holding the record of pending merges leaves no approval
// waiting here, since nothing would go on with it after quitting.
func TestApprovalThatWouldWaitIsLockedOut(t *testing.T) {
	mi, it := pr("1", "api", 1, "alice")
	mi.Checks = []model.Check{checkRun("test", "IN_PROGRESS", "")}
	f := newFake(mi)
	r := New(f, logging.Discard(), Options{Settings: settings, Viewer: "me", WaitTimeout: time.Second,
		PollFast: 5 * time.Millisecond, Quiet: 20 * time.Millisecond})
	_, ch := r.StartReview(t.Context(), ReviewText{Event: model.EventApprove}, []Item{approveWait(it)})
	if e := last(drain(t, ch)); e.Result != Skipped || !e.Locked || len(f.calls) > 0 {
		t.Errorf("last event = %+v, calls %v", e, f.calls)
	}
}

// An approval that waited goes on at the next launch, and approves once
// the jobs passed.
func TestResumeApproval(t *testing.T) {
	mi, _ := pr("1", "api", 1, "alice")
	mi.Checks = []model.Check{checkRun("test", "IN_PROGRESS", "")}
	f := newFake(mi)
	f.onRead = func(_ string, mi *model.MergeInfo, n int) {
		if n > 3 {
			mi.Checks = []model.Check{checkRun("test", model.RunCompleted, model.CheckSuccess)}
		}
	}
	st := newStore()
	p := model.Pending{Kind: model.PendingApprove, Key: mi.Key, ID: "1", Head: "h-1", Author: "alice"}
	if err := st.Put(p); err != nil {
		t.Fatal(err)
	}
	_, ch := newRunner(f, st).Resume(t.Context(), []model.Pending{p})
	if e := last(drain(t, ch)); e.Result != Approved || len(st.now) != 0 {
		t.Errorf("last event = %+v, left %v (calls %v)", e, st.now, f.calls)
	}
	if f.reads["1"] <= 3 {
		t.Errorf("approved after %d reads, before the job ended", f.reads["1"])
	}
}
