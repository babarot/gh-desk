package plan

import (
	"slices"

	"github.com/babarot/gh-triage/internal/model"
)

// Step is what merging a pull request takes next.
type Step string

const (
	// StepMerge merges it now.
	StepMerge Step = "merge"
	// StepAutoMerge turns on GitHub's auto-merge.
	StepAutoMerge Step = "auto-merge"
	// StepWait waits for its checks and rules, then merges.
	StepWait Step = "wait"
	// StepUpdate updates its branch with the base, then waits.
	StepUpdate Step = "update-branch"
	// StepRecheck reads it again: GitHub is still working out whether it
	// can be merged.
	StepRecheck Step = "recheck"
	// StepSkip leaves it.
	StepSkip Step = "skip"
)

// Settings are what deciding a merge takes from the config file.
type Settings struct {
	// Methods is the order of preference among the methods a repository
	// allows.
	Methods []model.MergeMethod
	// AutoMerge uses GitHub's auto-merge where the repository allows it.
	AutoMerge bool
	// UpdateBranch is whose branch is updated when behind: "humans", "all"
	// or "never".
	UpdateBranch string
	// ApprovalChecks are glob patterns of checks that pass only after an
	// approval.
	ApprovalChecks []string
	// AttentionPaths are glob patterns of files that put a mark on a pull
	// request.
	AttentionPaths []string
}

// Decision is what to do with one pull request.
type Decision struct {
	// Approve is whether to approve it first.
	Approve bool
	Step    Step
	Method  model.MergeMethod
	// Reason says why it is skipped, or how it will be merged.
	Reason string
	// Notes are what the user should know before saying yes.
	Notes []string
}

// Failing reports whether a check that must pass has failed. Without the
// single checks (they were not read), the combined state tells.
func Failing(mi model.MergeInfo) bool {
	if mi.Checks == nil {
		return mi.Rollup == model.CheckFailure || mi.Rollup == model.CheckError
	}
	for _, c := range mi.Checks {
		if c.Required && failed(c.State) {
			return true
		}
	}
	return false
}

// failed reports whether a check ended without passing: failed, errored,
// cancelled, timed out, or waiting on an action it will not get.
func failed(s model.CheckState) bool {
	switch s {
	case model.CheckFailure, model.CheckError, "CANCELLED", "TIMED_OUT", "ACTION_REQUIRED", "STARTUP_FAILURE", "STALE":
		return true
	}
	return false
}

func pending(s model.CheckState) bool {
	return s == model.CheckPending || s == model.CheckExpected || s == ""
}

// ChecksRunning reports whether a required check, other than one that
// waits for an approval, has not finished.
func ChecksRunning(mi model.MergeInfo, approvalChecks []string) bool {
	for _, c := range mi.Checks {
		if c.Required && pending(c.State) && !MatchAny(approvalChecks, c.Name) {
			return true
		}
	}
	return false
}

// AwaitingApproval reports whether every check that has not succeeded is
// one that waits for an approval, and at least one does: the pull request
// waits for a review, not for CI.
func AwaitingApproval(checks []model.Check, approvalChecks []string) bool {
	waiting := false
	for _, c := range checks {
		switch {
		case c.State == model.CheckSuccess || c.State == model.CheckSkipped || c.State == model.CheckNeutral:
		case pending(c.State) && MatchAny(approvalChecks, c.Name):
			waiting = true
		default:
			return false
		}
	}
	return waiting
}

// Method is the first of the preferred methods the repository allows.
func Method(preferred []model.MergeMethod, repo model.RepoMerge) (model.MergeMethod, bool) {
	for _, m := range preferred {
		if slices.Contains(repo.Methods, m) {
			return m, true
		}
	}
	return "", false
}

// needsApproval reports whether the viewer has to approve the pull request
// before merging it: it is not theirs and they have not approved it.
func needsApproval(mi model.MergeInfo, pr model.PR, viewer string) bool {
	if pr.Author == viewer {
		return false
	}
	for _, r := range mi.Reviews {
		if r.Author == viewer && r.State == model.ReviewApproved {
			return false
		}
	}
	return true
}

func notes(mi model.MergeInfo, set Settings) []string {
	var ns []string
	for _, f := range mi.Files {
		if MatchAny(set.AttentionPaths, f) {
			ns = append(ns, "changes "+f)
		}
	}
	if mi.Checks != nil {
		for _, c := range mi.Checks {
			if !c.Required && failed(c.State) {
				ns = append(ns, "a check that is not required failed: "+c.Name)
				break
			}
		}
	}
	return ns
}

// ForApprove decides approving a pull request. Failing required checks
// skip it unless includeFailing.
func ForApprove(mi model.MergeInfo, pr model.PR, viewer string, includeFailing bool, set Settings) Decision {
	d := Decision{Notes: notes(mi, set)}
	skip := func(why string) Decision { d.Step, d.Reason = StepSkip, why; return d }
	switch {
	case mi.State != model.StateOpen:
		return skip(stateWord(mi.State))
	case pr.Author == viewer:
		return skip("your own pull request")
	case !needsApproval(mi, pr, viewer):
		return skip("already approved")
	case Failing(mi) && !includeFailing:
		return skip("checks failing")
	}
	d.Approve, d.Reason = true, "approve"
	return d
}

// ForMerge decides merging a pull request, approving it first when the
// viewer has to. It is called again with fresh information before each
// write, so what it decided when the user said yes is checked against what
// is true when it acts.
func ForMerge(mi model.MergeInfo, pr model.PR, viewer string, set Settings) Decision {
	d := Decision{Notes: notes(mi, set)}
	skip := func(why string) Decision { d.Step, d.Reason, d.Approve, d.Method = StepSkip, why, false, ""; return d }
	if mi.State != model.StateOpen {
		return skip(stateWord(mi.State))
	}
	if mi.Draft {
		return skip("draft")
	}
	if Failing(mi) {
		return skip("checks failing")
	}
	method, ok := Method(set.Methods, mi.Repo)
	if !ok {
		return skip("no merge method allowed")
	}
	d.Method = method
	d.Approve = needsApproval(mi, pr, viewer)
	switch mi.MergeState {
	case model.MergeDirty:
		return skip("conflicts")
	case model.MergeDraft:
		return skip("draft")
	case model.MergeUnknown, "":
		d.Step, d.Reason = StepRecheck, "checking whether it can be merged"
	case model.MergeClean, model.MergeUnstable, model.MergeHasHooks:
		d.Step, d.Reason = StepMerge, "merge now"
	case model.MergeBehind:
		switch {
		case set.UpdateBranch == "never":
			return skip("behind its base")
		case pr.AuthorIsBot && set.UpdateBranch != "all":
			return skip("behind · the bot will rebase")
		}
		d.Step, d.Reason = StepUpdate, "update the branch, then merge"
	case model.MergeBlocked:
		if set.AutoMerge && mi.Repo.AutoMerge {
			d.Step, d.Reason = StepAutoMerge, "turn on auto-merge"
			d.Notes = append(d.Notes, "auto-merge also merges commits pushed after it is on")
		} else {
			d.Step, d.Reason = StepWait, "wait for checks, then merge"
		}
	default:
		return skip("cannot be merged: " + string(mi.MergeState))
	}
	return d
}

func stateWord(s model.PRState) string {
	switch s {
	case model.StateMerged:
		return "merged"
	case model.StateClosed:
		return "closed"
	}
	return "not open"
}
