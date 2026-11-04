package plan

import (
	"slices"
	"testing"
	"time"

	"github.com/babarot/gh-desk/internal/model"
)

func checkRun(name string, run model.RunStatus, conclusion model.CheckState) model.Check {
	st := conclusion
	if run != model.RunCompleted {
		st = model.CheckPending
	}
	return model.Check{Name: name, State: st, Kind: model.KindCheckRun, Run: run}
}

func status(name string, st model.CheckState, required bool) model.Check {
	return model.Check{Name: name, State: st, Required: required, Kind: model.KindStatus}
}

func workflowRun(name string, st model.RunStatus, conclusion model.CheckState) model.WorkflowRun {
	return model.WorkflowRun{Name: name, Status: st, Conclusion: conclusion}
}

const (
	inProgress model.RunStatus = "IN_PROGRESS"
	queued     model.RunStatus = "QUEUED"
)

// Jobs are told from check runs and workflow runs, required or not; a
// commit status is not a job.
func TestJobs(t *testing.T) {
	now := time.Date(2026, 11, 5, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		name    string
		mi      model.MergeInfo
		trigger Trigger
		running bool
		names   []string
		failed  []string
	}{
		{
			// A repository with no required check, its tests still at
			// work, as gh-desk#47 was merged.
			name: "jobs at work, none required",
			mi: model.MergeInfo{RunsRead: true,
				Checks: []model.Check{checkRun("lint", model.RunCompleted, model.CheckSuccess), checkRun("test (ubuntu-latest)", inProgress, ""), checkRun("site", queued, "")},
				Runs:   []model.WorkflowRun{workflowRun("Test", inProgress, "")}},
			running: true, names: []string{"test (ubuntu-latest)", "site"},
		},
		{
			// Between a job and the one that needs it: every check run is
			// completed, the workflow run is not.
			name: "between a job and the one that needs it",
			mi: model.MergeInfo{RunsRead: true,
				Checks: []model.Check{checkRun("Plan", model.RunCompleted, model.CheckSuccess)},
				Runs:   []model.WorkflowRun{workflowRun("Plan", inProgress, ""), workflowRun("Lint", model.RunCompleted, model.CheckSuccess)}},
			running: true, names: []string{"Plan"},
		},
		{
			// A policy bot's pending status waits for the approval itself.
			name: "only a commit status pending",
			mi: model.MergeInfo{RunsRead: true,
				Checks: []model.Check{checkRun("test", model.RunCompleted, model.CheckSuccess), status("policy-bot: main", model.CheckPending, true)},
				Runs:   []model.WorkflowRun{workflowRun("Test", model.RunCompleted, model.CheckSuccess)}},
		},
		{
			name: "a job waiting for its environment",
			mi: model.MergeInfo{RunsRead: true,
				Checks: []model.Check{checkRun("deploy", model.RunWaiting, "")},
				Runs:   []model.WorkflowRun{workflowRun("Deploy", model.RunWaiting, "")}},
		},
		{
			// Every job is in a workflow run, all read.
			name: "checks not all read, runs all read",
			mi:   model.MergeInfo{RunsRead: true, ChecksPartial: true, Runs: []model.WorkflowRun{workflowRun("Test", model.RunCompleted, model.CheckSuccess)}},
		},
		{
			name:    "checks not all read, runs not all read",
			mi:      model.MergeInfo{RunsRead: true, RunsPartial: true, ChecksPartial: true},
			running: true,
		},
		{
			name:    "checks not all read, runs not read",
			mi:      model.MergeInfo{ChecksPartial: true},
			running: true,
		},
		{
			name: "runs not all read, checks all read",
			mi:   model.MergeInfo{RunsRead: true, RunsPartial: true, Checks: []model.Check{checkRun("test", model.RunCompleted, model.CheckSuccess)}},
		},
		{
			// Actions off, or the token may not read runs: the check runs
			// tell.
			name: "runs not read",
			mi:   model.MergeInfo{Checks: []model.Check{checkRun("test", model.RunCompleted, model.CheckSuccess)}},
		},
		{
			name:    "the user's yes, no run yet",
			mi:      model.MergeInfo{RunsRead: true},
			trigger: Trigger{At: now.Add(-10 * time.Second)},
			running: true,
		},
		{
			name:    "the user's yes, runs there",
			mi:      model.MergeInfo{RunsRead: true, Runs: []model.WorkflowRun{workflowRun("Test", model.RunCompleted, model.CheckSuccess)}},
			trigger: Trigger{At: now.Add(-10 * time.Second)},
		},
		{
			name:    "the user's yes, runs not read",
			mi:      model.MergeInfo{},
			trigger: Trigger{At: now.Add(-10 * time.Second)},
			running: true,
		},
		{
			// Marked ready by gh-desk: a workflow run for ready for review
			// is still to come, beside the runs of the opening.
			name:    "ready, older runs there",
			mi:      model.MergeInfo{RunsRead: true, Runs: []model.WorkflowRun{workflowRun("Test", model.RunCompleted, model.CheckSuccess)}},
			trigger: Trigger{At: now.Add(-10 * time.Second), Always: true},
			running: true,
		},
		{
			name:    "quiet time over",
			mi:      model.MergeInfo{RunsRead: true},
			trigger: Trigger{At: now.Add(-Quiet), Always: true},
		},
		{
			name: "a job failed",
			mi: model.MergeInfo{RunsRead: true,
				Checks: []model.Check{checkRun("lint", model.RunCompleted, model.CheckFailure), checkRun("skip", model.RunCompleted, model.CheckSkipped), checkRun("n", model.RunCompleted, model.CheckNeutral)},
				Runs:   []model.WorkflowRun{workflowRun("Test", model.RunCompleted, model.CheckFailure)}},
			failed: []string{"lint"},
		},
		{
			// A workflow that could not start has no job.
			name: "a workflow that could not start",
			mi: model.MergeInfo{RunsRead: true,
				Runs: []model.WorkflowRun{workflowRun("Broken", model.RunCompleted, "STARTUP_FAILURE")}},
			failed: []string{"Broken"},
		},
		{
			// A required status that failed is Failing's, not a job.
			name: "a failed commit status",
			mi:   model.MergeInfo{RunsRead: true, Checks: []model.Check{status("ci/jenkins", model.CheckFailure, true)}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := JobsRunning(tt.mi, tt.trigger, now); got != tt.running {
				t.Errorf("JobsRunning = %v; want %v", got, tt.running)
			}
			if got := RunningJobs(tt.mi); !slices.Equal(got, tt.names) {
				t.Errorf("RunningJobs = %q; want %q", got, tt.names)
			}
			if got := FailedJobs(tt.mi); !slices.Equal(got, tt.failed) {
				t.Errorf("FailedJobs = %q; want %q", got, tt.failed)
			}
		})
	}
}

// A merge waits for the jobs at work, required or not, and leaves a pull
// request whose job failed; GitHub's auto-merge, which waits only for the
// required ones, is not turned on while they are at work.
func TestForMergeWaitsForJobs(t *testing.T) {
	human := model.PR{Author: "alice"}
	mine := model.PR{Author: "me"}
	at := func(state model.MergeState, checks ...model.Check) model.MergeInfo {
		mi := info(state)
		mi.Checks, mi.RunsRead = checks, true
		mi.Runs = []model.WorkflowRun{workflowRun("CI", model.RunCompleted, model.CheckSuccess)}
		return mi
	}
	atWork := checkRun("test (ubuntu-latest)", inProgress, "")
	tests := []struct {
		name   string
		mi     model.MergeInfo
		pr     model.PR
		step   Step
		reason string
		admin  bool
	}{
		// gh-desk#47: no required check, the tests at work.
		{"unstable, a job at work", at(model.MergeUnstable, atWork), human, StepWait, "wait for checks, then merge", false},
		{"clean, a job at work", at(model.MergeClean, atWork), human, StepWait, "wait for checks, then merge", false},
		{"clean, jobs done", at(model.MergeClean, checkRun("test", model.RunCompleted, model.CheckSuccess)), human, StepMerge, "merge now", false},
		{"a job not required failed", at(model.MergeUnstable, checkRun("lint", model.RunCompleted, model.CheckFailure)), human, StepSkip, "a job failed: lint", false},
		{"blocked, auto-merge allowed, a job at work", func() model.MergeInfo {
			mi := at(model.MergeBlocked, atWork)
			mi.Repo.AutoMerge = true
			return mi
		}(), human, StepWait, "wait for checks, then merge", false},
		{"blocked, auto-merge allowed, jobs done", func() model.MergeInfo {
			mi := at(model.MergeBlocked)
			mi.Repo.AutoMerge = true
			return mi
		}(), human, StepAutoMerge, "turn on auto-merge", false},
		// A policy bot's status waits for the approval: not a job.
		{"only a status pending", at(model.MergeBlocked, status("policy-bot: main", model.CheckPending, true)), human, StepWait, "wait for checks, then merge", false},
		{"admin, a job at work", func() model.MergeInfo {
			mi := at(model.MergeBlocked, atWork)
			mi.AdminMerge = true
			return mi
		}(), mine, StepWait, "wait for checks, then merge as admin", false},
		// An admin merge goes past GitHub's wait for required checks, so a
		// required status that failed still stops it.
		{"admin, a required status failed", func() model.MergeInfo {
			mi := at(model.MergeBlocked, status("ci/jenkins", model.CheckFailure, true))
			mi.AdminMerge = true
			return mi
		}(), mine, StepSkip, "checks failing", false},
		{"admin, all done", func() model.MergeInfo {
			mi := at(model.MergeBlocked, checkRun("test", model.RunCompleted, model.CheckSuccess))
			mi.AdminMerge = true
			return mi
		}(), mine, StepMerge, "merge as admin", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := ForMerge(tt.mi, tt.pr, "me", set)
			if d.Step != tt.step || d.Reason != tt.reason || d.Admin != tt.admin {
				t.Errorf("ForMerge = %+v; want step %s, reason %q, admin %v", d, tt.step, tt.reason, tt.admin)
			}
		})
	}
}
