package plan

import (
	"slices"
	"strings"
	"testing"

	"github.com/babarot/gh-desk/internal/model"
)

var set = Settings{
	Methods:        []model.MergeMethod{model.MethodMerge, model.MethodSquash, model.MethodRebase},
	AutoMerge:      true,
	UpdateBranch:   "humans",
	ApprovalChecks: []string{"policy-bot*"},
	AttentionPaths: []string{".github/workflows/*", "CODEOWNERS"},
}

func info(state model.MergeState) model.MergeInfo {
	return model.MergeInfo{State: model.StateOpen, MergeState: state, HeadOID: "h1",
		Repo: model.RepoMerge{Methods: []model.MergeMethod{model.MethodSquash, model.MethodMerge}}}
}

func TestForMerge(t *testing.T) {
	human := model.PR{Author: "alice"}
	bot := model.PR{Author: "renovate", AuthorIsBot: true}
	mine := model.PR{Author: "me"}
	tests := []struct {
		name    string
		mi      func() model.MergeInfo
		pr      model.PR
		step    Step
		approve bool
		reason  string
		method  model.MergeMethod
	}{
		{"clean", func() model.MergeInfo { return info(model.MergeClean) }, human, StepMerge, true, "merge now", model.MethodMerge},
		{"own clean", func() model.MergeInfo { return info(model.MergeClean) }, mine, StepMerge, false, "merge now", model.MethodMerge},
		{"unstable merges", func() model.MergeInfo { return info(model.MergeUnstable) }, human, StepMerge, true, "merge now", model.MethodMerge},
		{"already approved", func() model.MergeInfo {
			mi := info(model.MergeClean)
			mi.Reviews = []model.Review{{Author: "me", State: model.ReviewApproved}}
			return mi
		}, human, StepMerge, false, "merge now", model.MethodMerge},
		{"dirty", func() model.MergeInfo { return info(model.MergeDirty) }, human, StepSkip, false, "conflicts", ""},
		{"someone else's draft", func() model.MergeInfo { mi := info(model.MergeDraft); mi.Draft = true; return mi }, human, StepSkip, false, "someone else's draft", ""},
		{"merged", func() model.MergeInfo { mi := info(model.MergeClean); mi.State = model.StateMerged; return mi }, human, StepSkip, false, "merged", ""},
		{"unknown rechecks", func() model.MergeInfo { return info(model.MergeUnknown) }, human, StepRecheck, true, "", model.MethodMerge},
		{"behind human updates", func() model.MergeInfo { return info(model.MergeBehind) }, human, StepUpdate, true, "", model.MethodMerge},
		{"behind bot is left to the bot", func() model.MergeInfo { return info(model.MergeBehind) }, bot, StepSkip, false, "behind · the bot will rebase", ""},
		{"blocked with auto-merge", func() model.MergeInfo {
			mi := info(model.MergeBlocked)
			mi.Repo.AutoMerge = true
			return mi
		}, bot, StepAutoMerge, true, "turn on auto-merge", model.MethodMerge},
		{"blocked without auto-merge waits", func() model.MergeInfo { return info(model.MergeBlocked) }, bot, StepWait, true, "", model.MethodMerge},
		{"required check failed", func() model.MergeInfo {
			mi := info(model.MergeBlocked)
			mi.Checks = []model.Check{{Name: "test", State: model.CheckFailure, Required: true}}
			return mi
		}, human, StepSkip, false, "checks failing", ""},
		{"failing rollup without single checks", func() model.MergeInfo {
			mi := info(model.MergeBlocked)
			mi.Rollup = model.CheckFailure
			return mi
		}, human, StepSkip, false, "checks failing", ""},
		{"only the squash the repository allows", func() model.MergeInfo {
			mi := info(model.MergeClean)
			mi.Repo.Methods = []model.MergeMethod{model.MethodSquash}
			return mi
		}, human, StepMerge, true, "", model.MethodSquash},
		{"no allowed method", func() model.MergeInfo {
			mi := info(model.MergeClean)
			mi.Repo.Methods = nil
			return mi
		}, human, StepSkip, false, "no merge method allowed", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := ForMerge(tt.mi(), tt.pr, "me", set)
			if d.Step != tt.step || d.Approve != tt.approve || d.Method != tt.method || (tt.reason != "" && d.Reason != tt.reason) {
				t.Errorf("ForMerge = %+v; want step %s approve %v method %s reason %q", d, tt.step, tt.approve, tt.method, tt.reason)
			}
		})
	}
}

// The viewer's own blocked pull request is merged as an admin when GitHub
// says the viewer can, once its required checks have finished; another
// person's, or one the viewer cannot merge as an admin, waits as before.
func TestForMergeAsAdmin(t *testing.T) {
	blocked := func(admin bool, checks ...model.Check) model.MergeInfo {
		mi := info(model.MergeBlocked)
		mi.AdminMerge, mi.Repo.AutoMerge, mi.Checks = admin, true, append([]model.Check{}, checks...)
		return mi
	}
	running := model.Check{Name: "build", State: model.CheckPending, Required: true}
	passed := model.Check{Name: "build", State: model.CheckSuccess, Required: true}
	approval := model.Check{Name: "policy-bot: main", State: model.CheckPending, Required: true}
	unread := blocked(true)
	unread.Checks = nil
	partial := blocked(true, passed)
	partial.ChecksPartial = true
	tests := []struct {
		name  string
		mi    model.MergeInfo
		pr    model.PR
		step  Step
		admin bool
		later bool
	}{
		{"own, checks passed", blocked(true, passed), model.PR{Author: "me"}, StepMerge, true, true},
		{"own, no checks", blocked(true), model.PR{Author: "me"}, StepMerge, true, true},
		{"own, only an approval check left", blocked(true, passed, approval), model.PR{Author: "me"}, StepMerge, true, true},
		{"own, checks running", blocked(true, running), model.PR{Author: "me"}, StepWait, false, true},
		{"own, checks not read", unread, model.PR{Author: "me"}, StepWait, false, true},
		{"own, checks not all read", partial, model.PR{Author: "me"}, StepWait, false, false},
		{"own, not an admin", blocked(false, passed), model.PR{Author: "me"}, StepAutoMerge, false, false},
		{"another person's", blocked(true, passed), model.PR{Author: "alice"}, StepAutoMerge, false, false},
		{"a bot's", blocked(true, passed), model.PR{Author: "renovate", AuthorIsBot: true}, StepAutoMerge, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := ForMerge(tt.mi, tt.pr, "me", set)
			if d.Step != tt.step || d.Admin != tt.admin {
				t.Errorf("ForMerge = %+v; want step %s admin %v", d, tt.step, tt.admin)
			}
			if d.AdminLater != (tt.later && !tt.admin) {
				t.Errorf("ForMerge = %+v; want admin later %v", d, tt.later && !tt.admin)
			}
			if said := strings.Contains(d.Reason, "as admin"); said != tt.later {
				t.Errorf("reason %q; want the admin merge said: %v", d.Reason, tt.later)
			}
		})
	}
	failing := blocked(true, model.Check{Name: "build", State: model.CheckFailure, Required: true})
	if d := ForMerge(failing, model.PR{Author: "me"}, "me", set); d.Step != StepSkip || d.Admin {
		t.Errorf("failing: %+v", d)
	}
	draft := blocked(true, passed)
	draft.Draft, draft.MergeState = true, model.MergeDraft
	if d := ForMerge(draft, model.PR{Author: "me"}, "me", set); !d.Ready || d.Step != StepWait || d.Admin || !d.AdminLater || !strings.Contains(d.Reason, "as admin") {
		t.Errorf("own draft: %+v", d)
	}
}

// A draft of the viewer's or of a bot is marked ready first, then waits for
// the checks that starts; the approval is the viewer's to send only on a
// bot's.
func TestForMergeDraft(t *testing.T) {
	draft := func(autoMerge bool) model.MergeInfo {
		mi := info(model.MergeDraft)
		mi.Draft, mi.Repo.AutoMerge = true, autoMerge
		return mi
	}
	tests := []struct {
		name    string
		mi      model.MergeInfo
		pr      model.PR
		step    Step
		approve bool
	}{
		{"own waits", draft(false), model.PR{Author: "me"}, StepWait, false},
		{"own on auto-merge", draft(true), model.PR{Author: "me"}, StepAutoMerge, false},
		{"bot's is approved", draft(false), model.PR{Author: "renovate", AuthorIsBot: true}, StepWait, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := ForMerge(tt.mi, tt.pr, "me", set)
			if !d.Ready || d.Step != tt.step || d.Approve != tt.approve || d.Method != model.MethodMerge {
				t.Errorf("ForMerge = %+v; want ready, step %s approve %v", d, tt.step, tt.approve)
			}
			// What marking it ready means is the popup's to say once, for
			// every draft: not a note of this pull request.
			if len(d.Notes) > 0 {
				t.Errorf("notes %q; want none", d.Notes)
			}
		})
	}
	failing := draft(false)
	failing.Rollup = model.CheckFailure
	if d := ForMerge(failing, model.PR{Author: "me"}, "me", set); d.Step != StepSkip || d.Ready {
		t.Errorf("failing draft: %+v", d)
	}
}

func TestAttentionNoteNamesPatterns(t *testing.T) {
	set := Settings{AttentionPaths: []string{".github/workflows/*", "CODEOWNERS", ".github/CODEOWNERS", "docs/CODEOWNERS"}}
	mi := info(model.MergeBlocked)
	mi.Files = []string{".github/workflows/ci.yaml", ".github/CODEOWNERS", ".github/workflows/release.yaml", "main.go"}
	d := ForApprove(mi, model.PR{Author: "other"}, "me", false, set)
	if want := []string{"changes .github/workflows/*, CODEOWNERS"}; !slices.Equal(d.Notes, want) {
		t.Errorf("notes %q, want %q", d.Notes, want)
	}
}

func TestForMergeNotes(t *testing.T) {
	mi := info(model.MergeBlocked)
	mi.Repo.AutoMerge = true
	mi.Files = []string{".github/workflows/ci.yaml", "main.go", ".github/workflows/release.yaml"}
	mi.Checks = []model.Check{{Name: "lint", State: model.CheckFailure}}
	d := ForMerge(mi, model.PR{Author: "renovate", AuthorIsBot: true}, "me", set)
	all := strings.Join(d.Notes, "\n")
	for _, want := range []string{"changes .github/workflows/*", "not required failed: lint"} {
		if !strings.Contains(all, want) {
			t.Errorf("notes %q lack %q", d.Notes, want)
		}
	}
	if strings.Contains(all, "main.go") {
		t.Error("main.go marked")
	}
	if strings.Contains(all, "auto-merge") {
		t.Errorf("notes %q say what auto-merge is: the popup says it once", d.Notes)
	}
}

func TestForApprove(t *testing.T) {
	failing := info(model.MergeBlocked)
	failing.Checks = []model.Check{{Name: "test", State: model.CheckFailure, Required: true}}
	approved := info(model.MergeBlocked)
	approved.Reviews = []model.Review{{Author: "me", State: model.ReviewApproved}}
	tests := []struct {
		name    string
		mi      model.MergeInfo
		pr      model.PR
		failing bool
		approve bool
		reason  string
	}{
		{"approve", info(model.MergeBlocked), model.PR{Author: "a"}, false, true, "approve"},
		{"own", info(model.MergeBlocked), model.PR{Author: "me"}, false, false, "your own pull request"},
		{"already", approved, model.PR{Author: "a"}, false, false, "already approved"},
		{"failing", failing, model.PR{Author: "a"}, false, false, "checks failing"},
		{"failing included", failing, model.PR{Author: "a"}, true, true, "approve"},
	}
	for _, tt := range tests {
		d := ForApprove(tt.mi, tt.pr, "me", tt.failing, set)
		if d.Approve != tt.approve || d.Reason != tt.reason {
			t.Errorf("%s: ForApprove = %+v", tt.name, d)
		}
	}
}

func TestForAutoApprove(t *testing.T) {
	draft := info(model.MergeBlocked)
	draft.Draft = true
	dismissedHere := info(model.MergeBlocked)
	dismissedHere.HeadOID = "h2"
	dismissedHere.ViewerReview = &model.ViewerReview{State: model.ReviewDismissed, Commit: "h2"}
	dismissedBefore := dismissedHere
	dismissedBefore.ViewerReview = &model.ViewerReview{State: model.ReviewDismissed, Commit: "h1"}
	approved := info(model.MergeBlocked)
	approved.Reviews = []model.Review{{Author: "me", State: model.ReviewApproved}}
	tests := []struct {
		name    string
		mi      model.MergeInfo
		approve bool
		reason  string
	}{
		{"approve", info(model.MergeBlocked), true, "approve"},
		{"draft", draft, false, "draft"},
		{"dismissed on this head", dismissedHere, false, "your approval was dismissed on this head"},
		{"dismissed by a push", dismissedBefore, true, "approve"},
		{"already", approved, false, "already approved"},
	}
	for _, tt := range tests {
		d := ForAutoApprove(tt.mi, model.PR{Author: "a"}, "me", false, set)
		if d.Approve != tt.approve || d.Reason != tt.reason || (d.Step == StepSkip) == tt.approve {
			t.Errorf("%s: ForAutoApprove = %+v", tt.name, d)
		}
	}
}

func TestForReady(t *testing.T) {
	draft := info(model.MergeDraft)
	draft.Draft = true
	merged := draft
	merged.State = model.StateMerged
	mine := model.PR{Author: "me"}
	bot := model.PR{Author: "renovate", AuthorIsBot: true}
	theirs := model.PR{Author: "alice"}
	tests := []struct {
		name   string
		mi     model.MergeInfo
		pr     model.PR
		skip   bool
		reason string
	}{
		{"draft", draft, mine, false, "mark ready for review"},
		{"a bot's draft", draft, bot, false, "mark ready for review"},
		{"someone else's draft", draft, theirs, true, "someone else's pull request"},
		{"not a draft", info(model.MergeClean), mine, true, "not a draft"},
		{"merged", merged, mine, true, "merged"},
	}
	for _, tt := range tests {
		d := ForReady(tt.mi, tt.pr, "me")
		if (d.Step == StepSkip) != tt.skip || d.Reason != tt.reason || d.Approve {
			t.Errorf("%s: ForReady = %+v", tt.name, d)
		}
	}
}

func TestForDraft(t *testing.T) {
	draft := info(model.MergeDraft)
	draft.Draft = true
	closed := info(model.MergeClean)
	closed.State = model.StateClosed
	mine := model.PR{Author: "me"}
	bot := model.PR{Author: "renovate", AuthorIsBot: true}
	theirs := model.PR{Author: "alice"}
	tests := []struct {
		name   string
		mi     model.MergeInfo
		pr     model.PR
		skip   bool
		reason string
	}{
		{"ready", info(model.MergeClean), mine, false, "convert to draft"},
		{"a bot's", info(model.MergeClean), bot, false, "convert to draft"},
		{"someone else's", info(model.MergeClean), theirs, true, "someone else's pull request"},
		{"already a draft", draft, mine, true, "already a draft"},
		{"closed", closed, mine, true, "closed"},
	}
	for _, tt := range tests {
		d := ForDraft(tt.mi, tt.pr, "me")
		if (d.Step == StepSkip) != tt.skip || d.Reason != tt.reason {
			t.Errorf("%s: ForDraft = %+v", tt.name, d)
		}
	}
}

func TestChecksRunningAndAwaitingApproval(t *testing.T) {
	mi := model.MergeInfo{Checks: []model.Check{
		{Name: "test", State: model.CheckSuccess, Required: true},
		{Name: "policy-bot: main", State: model.CheckPending, Required: true},
	}}
	if ChecksRunning(mi, set.ApprovalChecks) {
		t.Error("an approval check counted as running")
	}
	if !AwaitingApproval(mi.Checks, set.ApprovalChecks) {
		t.Error("not awaiting approval")
	}
	mi.Checks = append(mi.Checks, model.Check{Name: "build", State: model.CheckPending, Required: true})
	if !ChecksRunning(mi, set.ApprovalChecks) || AwaitingApproval(mi.Checks, set.ApprovalChecks) {
		t.Error("a running build was missed")
	}
}

func TestCancelledRequiredCheckFails(t *testing.T) {
	mi := info(model.MergeBlocked)
	mi.Checks = []model.Check{{Name: "test", State: "CANCELLED", Required: true}} //nolint:misspell // GitHub's spelling
	if !Failing(mi) {
		t.Error("a canceled required check is not failing")
	}
}

func TestForReview(t *testing.T) {
	open := info(model.MergeClean)
	merged := open
	merged.State = model.StateMerged
	other, mine := model.PR{Author: "alice"}, model.PR{Author: "me"}
	tests := []struct {
		name   string
		mi     model.MergeInfo
		pr     model.PR
		event  model.ReviewEvent
		skip   bool
		reason string
	}{
		{"approve", open, other, model.EventApprove, false, "approve"},
		{"approve own", open, mine, model.EventApprove, true, "your own pull request"},
		{"approve merged", merged, other, model.EventApprove, true, "merged"},
		{"request changes", open, other, model.EventRequestChanges, false, "request changes"},
		{"request changes own", open, mine, model.EventRequestChanges, true, "your own pull request"},
		{"request changes merged", merged, other, model.EventRequestChanges, true, "merged"},
		{"comment own", open, mine, model.EventComment, false, "comment"},
		{"comment merged", merged, other, model.EventComment, false, "comment"},
	}
	for _, tt := range tests {
		d := ForReview(tt.mi, tt.pr, "me", tt.event, false, set)
		if (d.Step == StepSkip) != tt.skip || d.Reason != tt.reason {
			t.Errorf("%s: ForReview = %+v", tt.name, d)
		}
	}
}
