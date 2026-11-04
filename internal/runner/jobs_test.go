package runner

import (
	"slices"
	"strings"
	"testing"

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
