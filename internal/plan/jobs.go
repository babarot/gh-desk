package plan

import (
	"slices"
	"time"

	"github.com/babarot/gh-desk/internal/model"
)

// Quiet is how long after something that may start workflows on a head
// what is seen is not taken as all there is, unless a Trigger says
// otherwise: GitHub makes a workflow run some seconds after the push, the
// opening or the ready for review that starts it.
const Quiet = 30 * time.Second

// Trigger is the last thing that may have started workflows on the head
// being decided on.
type Trigger struct {
	At time.Time
	// Always is set for what gh-desk wrote itself, marking the pull
	// request ready or updating its branch: workflows it starts come on
	// top of runs the head may already have, so runs being there says
	// nothing. The user's yes waits only while the head has no run, or its
	// runs could not be read.
	Always bool
	// For is how long the quiet time lasts; Quiet when zero.
	For time.Duration
}

// quiet reports whether now is still in the quiet time after t.
func (t Trigger) quiet(mi model.MergeInfo, now time.Time) bool {
	quiet := t.For
	if quiet == 0 {
		quiet = Quiet
	}
	if t.At.IsZero() || now.Sub(t.At) >= quiet {
		return false
	}
	// Runs that could not be read may be none yet.
	return t.Always || !mi.RunsRead || len(mi.Runs) == 0
}

// JobsRunning reports whether jobs of the head are still to finish: a
// check run or a workflow run not completed, other than one waiting for
// someone to approve its environment, which would not finish while gh-desk
// waits. So does the quiet time after t. Commit statuses are not jobs, and
// are not waited for here.
//
// A workflow run holds every job of its workflow, so runs all read tell
// for the check runs not read; with neither all read, what runs is not
// known, and counts as running. Workflow runs that could not be read leave
// the check runs to tell; a job that waits for another has no check run
// until that one ends, so then there is a moment when nothing seems to
// run.
func JobsRunning(mi model.MergeInfo, t Trigger, now time.Time) bool {
	runsKnown := mi.RunsRead && !mi.RunsPartial
	return mi.ChecksPartial && !runsKnown || t.quiet(mi, now) || len(RunningJobs(mi)) > 0
}

// RunningJobs names the jobs being waited for: the check runs not
// completed, or when none is, the workflow runs not completed (between a
// job and the one that needs it, only its workflow run is at work). A
// workflow run is not named beside its own jobs: which run a check run
// belongs to is not read.
func RunningJobs(mi model.MergeInfo) []string {
	var out []string
	for _, c := range mi.Checks {
		if c.Kind == model.KindCheckRun && running(c.Run) {
			out = append(out, c.Name)
		}
	}
	if len(out) > 0 {
		return out
	}
	for _, r := range mi.Runs {
		if running(r.Status) && !slices.Contains(out, r.Name) {
			out = append(out, r.Name)
		}
	}
	return out
}

func running(s model.RunStatus) bool {
	return s != model.RunCompleted && s != model.RunWaiting
}

// FailedJobs names the jobs of the head that ended without passing, whether
// required or not: the check runs, or when none failed, the workflow runs
// that did (a workflow that could not start has no job to fail). A skipped
// or neutral job has not failed. Commit statuses are left to Failing.
func FailedJobs(mi model.MergeInfo) []string {
	var out []string
	for _, c := range mi.Checks {
		if c.Kind == model.KindCheckRun && c.Run == model.RunCompleted && failed(c.State) {
			out = append(out, c.Name)
		}
	}
	if len(out) > 0 {
		return out
	}
	for _, r := range mi.Runs {
		if r.Status == model.RunCompleted && failed(r.Conclusion) && !slices.Contains(out, r.Name) {
			out = append(out, r.Name)
		}
	}
	return out
}
