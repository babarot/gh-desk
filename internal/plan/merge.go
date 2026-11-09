package plan

import (
	"path"
	"slices"
	"strings"
	"time"

	"github.com/babarot/gh-desk/internal/model"
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
	// Ready is whether to mark it ready for review first: a draft of the
	// viewer's or of a bot.
	Ready bool
	// Approve is whether to approve it first, after Ready.
	Approve bool
	Step    Step
	Method  model.MergeMethod
	// Reason says why it is skipped, or, as steps joined by " → ", how it
	// will be merged: the steps after Ready and Approve.
	Reason string
	// Admin is whether it is merged as an admin, past the rules of its base
	// branch.
	Admin bool
	// AdminLater is whether the merge it waits for goes as an admin (for a
	// draft, if it is still blocked once it is ready).
	AdminLater bool
	// Notes are what the user should know of this pull request before
	// saying yes. What a step means, the same for every pull request that
	// takes it, is not a note: the confirm popup says it once.
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
// canceled, timed out, or waiting on an action it will not get.
func failed(s model.CheckState) bool {
	switch s {
	// GitHub spells the state with two Ls
	case model.CheckFailure, model.CheckError, "CANCELLED", "TIMED_OUT", "ACTION_REQUIRED", "STARTUP_FAILURE", "STALE": //nolint:misspell
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

// RunningChecks names the checks a wait is for: the required checks still
// running, apart from approval checks, or with approval set, the approval
// checks still running.
func RunningChecks(mi model.MergeInfo, approvalChecks []string, approval bool) []string {
	var out []string
	for _, c := range mi.Checks {
		if !pending(c.State) {
			continue
		}
		isApproval := MatchAny(approvalChecks, c.Name)
		if approval && isApproval || !approval && c.Required && !isApproval {
			out = append(out, c.Name)
		}
	}
	return out
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

// notes are what the user should know of a pull request apart from how it
// is merged. The files it changes that call for care make one note, as the
// popup shows every note of a row; it names the patterns they match, not
// each file (attentionName).
func notes(mi model.MergeInfo, set Settings) []string {
	var ns, marked []string
	for _, pat := range set.AttentionPaths {
		name := attentionName(pat)
		if slices.Contains(marked, name) {
			continue
		}
		for _, f := range mi.Files {
			if MatchAny([]string{pat}, f) {
				marked = append(marked, name)
				break
			}
		}
	}
	if len(marked) > 0 {
		ns = append(ns, "changes "+strings.Join(marked, ", "))
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

// attentionName is how a note names the files a pattern of
// AttentionPaths matches: a glob as it is written (.github/workflows/*),
// and one file by its name, so CODEOWNERS and .github/CODEOWNERS read as
// one.
func attentionName(pat string) string {
	if strings.ContainsAny(pat, "*?[\\") {
		return pat
	}
	return path.Base(pat)
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

// ForAutoApprove decides an approval a rule asked for (ADR-093): as
// ForApprove, and besides it never approves a draft, nor a head on which
// the viewer's approval was dismissed, which a person took back. An
// approval dismissed by a push is on an older head and does not count.
func ForAutoApprove(mi model.MergeInfo, pr model.PR, viewer string, includeFailing bool, set Settings) Decision {
	d := ForApprove(mi, pr, viewer, includeFailing, set)
	if d.Step == StepSkip {
		return d
	}
	switch v := mi.ViewerReview; {
	case mi.Draft:
		d.Step, d.Reason, d.Approve = StepSkip, "draft", false
	case v != nil && v.State == model.ReviewDismissed && v.Commit == mi.HeadOID:
		d.Step, d.Reason, d.Approve = StepSkip, "your approval was dismissed on this head", false
	}
	return d
}

// ForReview decides sending a review of the kind event: an approval as
// ForApprove does, a request for changes to another's open pull request,
// and a comment to any.
func ForReview(mi model.MergeInfo, pr model.PR, viewer string, event model.ReviewEvent, includeFailing bool, set Settings) Decision {
	switch event {
	case model.EventApprove:
		return ForApprove(mi, pr, viewer, includeFailing, set)
	case model.EventRequestChanges:
		switch {
		case mi.State != model.StateOpen:
			return Decision{Step: StepSkip, Reason: stateWord(mi.State)}
		case pr.Author == viewer:
			return Decision{Step: StepSkip, Reason: "your own pull request"}
		}
		return Decision{Reason: "request changes"}
	}
	return Decision{Reason: "comment"}
}

// ForReady decides marking a draft pull request ready for review. Only a
// draft of the viewer's or of a bot is: another person's says that person
// does not want it reviewed yet (ADR-108). Whether GitHub lets the viewer
// is GitHub's to say.
func ForReady(mi model.MergeInfo, pr model.PR, viewer string) Decision {
	switch {
	case mi.State != model.StateOpen:
		return Decision{Step: StepSkip, Reason: stateWord(mi.State)}
	case !OwnOrBot(pr, viewer):
		return Decision{Step: StepSkip, Reason: "someone else's pull request"}
	case !mi.Draft:
		return Decision{Step: StepSkip, Reason: "not a draft"}
	}
	return Decision{Reason: "mark ready for review"}
}

// ForDraft decides turning a pull request back into a draft, which only a
// pull request of the viewer's or of a bot is (ADR-108).
func ForDraft(mi model.MergeInfo, pr model.PR, viewer string) Decision {
	switch {
	case mi.State != model.StateOpen:
		return Decision{Step: StepSkip, Reason: stateWord(mi.State)}
	case !OwnOrBot(pr, viewer):
		return Decision{Step: StepSkip, Reason: "someone else's pull request"}
	case mi.Draft:
		return Decision{Step: StepSkip, Reason: "already a draft"}
	}
	return Decision{Reason: "convert to draft"}
}

// OwnOrBot reports whether pr is the viewer's or a bot's: whose draft
// state gh-desk changes (ADR-046, ADR-108).
func OwnOrBot(pr model.PR, viewer string) bool {
	return pr.Author == viewer || pr.AuthorIsBot
}

// ForMerge decides merging a pull request, marking it ready for review
// first when it is a draft of the viewer's or of a bot, and approving it
// when the viewer has to. Another person's draft is left: the author has
// said it is not ready. It is called again with fresh information before each
// write, so what it decided when the user said yes is checked against what
// is true when it acts.
func ForMerge(mi model.MergeInfo, pr model.PR, viewer string, set Settings) Decision {
	d := Decision{Notes: notes(mi, set)}
	skip := func(why string) Decision {
		d.Step, d.Reason, d.Ready, d.Approve, d.Method, d.Admin, d.AdminLater = StepSkip, why, false, false, "", false, false
		return d
	}
	if mi.State != model.StateOpen {
		return skip(stateWord(mi.State))
	}
	if mi.Draft {
		if !OwnOrBot(pr, viewer) {
			return skip("someone else's draft")
		}
		d.Ready = true
	}
	if Failing(mi) {
		return skip("checks failing")
	}
	if failed := FailedJobs(mi); len(failed) > 0 {
		return skip("a job failed: " + failed[0])
	}
	// Jobs at work, required or not, are waited for before any merge: the
	// merge goes once they passed. The quiet time after a trigger is the
	// runner's, which knows when it wrote.
	jobs := JobsRunning(mi, Trigger{}, time.Time{})
	method, ok := Method(set.Methods, mi.Repo)
	if !ok {
		return skip("no merge method allowed")
	}
	d.Method = method
	d.Approve = needsApproval(mi, pr, viewer)
	if d.Ready {
		// What GitHub says of a draft is only that it is one. Marking it
		// ready starts its checks, so it waits for them; the runner
		// decides again once it is ready.
		if asAdmin(mi, pr, viewer) {
			// Auto-merge would wait for the review an admin merge goes
			// past; waiting here decides again once it is ready.
			d.Step, d.Reason, d.AdminLater = StepWait, "wait for checks → merge as admin if blocked", true
			return d
		}
		return blocked(d, mi, set, jobs)
	}
	switch mi.MergeState {
	case model.MergeDirty:
		return skip("conflicts")
	case model.MergeDraft:
		return skip("draft")
	case model.MergeUnknown, "":
		d.Step, d.Reason = StepRecheck, "checking whether it can be merged"
	case model.MergeClean, model.MergeUnstable, model.MergeHasHooks:
		if jobs {
			d.Step, d.Reason = StepWait, "wait for checks → merge"
		} else {
			d.Step, d.Reason = StepMerge, "merge now"
		}
	case model.MergeBehind:
		switch {
		case set.UpdateBranch == "never":
			return skip("behind its base")
		case pr.AuthorIsBot && set.UpdateBranch != "all":
			return skip("behind · the bot will rebase")
		}
		d.Step, d.Reason = StepUpdate, "update the branch → wait for checks → merge"
	case model.MergeBlocked:
		if asAdmin(mi, pr, viewer) {
			return adminMerge(d, mi, set, jobs)
		}
		return blocked(d, mi, set, jobs)
	default:
		return skip("cannot be merged: " + string(mi.MergeState))
	}
	return d
}

// blocked is the step of a pull request that waits for its checks or
// rules: auto-merge where the repository allows it, waiting otherwise.
// While jobs are at work it waits: GitHub's auto-merge waits only for the
// required ones.
func blocked(d Decision, mi model.MergeInfo, set Settings, jobs bool) Decision {
	if set.AutoMerge && mi.Repo.AutoMerge && !jobs {
		d.Step, d.Reason = StepAutoMerge, "turn on auto-merge"
	} else {
		d.Step, d.Reason = StepWait, "wait for checks → merge"
	}
	return d
}

// asAdmin reports whether a blocked pull request is merged as an admin:
// it is the viewer's own, which nobody else may be there to approve, and
// GitHub says the viewer can merge past the base branch's rules. Another
// person's pull request waits for its reviews as before.
func asAdmin(mi model.MergeInfo, pr model.PR, viewer string) bool {
	return pr.Author == viewer && mi.AdminMerge
}

// adminMerge is the step of the viewer's own blocked pull request that the
// viewer can merge as an admin: merge it once its required checks have
// finished, wait for them before. An admin merge goes past required checks
// too, so it never goes before they are known to have passed.
//
// Checks not all read leave it to wait as before: one not read may be
// failing.
func adminMerge(d Decision, mi model.MergeInfo, set Settings, jobs bool) Decision {
	if mi.ChecksPartial {
		return blocked(d, mi, set, jobs)
	}
	if mi.Checks == nil || ChecksRunning(mi, set.ApprovalChecks) || jobs {
		d.Step, d.Reason, d.AdminLater = StepWait, "wait for checks → merge as admin", true
		return d
	}
	d.Step, d.Reason, d.Admin = StepMerge, "merge as admin", true
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
