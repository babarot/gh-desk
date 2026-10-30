package plan

import (
	"strings"
	"testing"

	"github.com/babarot/gh-triage/internal/model"
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
		{"draft", func() model.MergeInfo { mi := info(model.MergeClean); mi.Draft = true; return mi }, human, StepSkip, false, "draft", ""},
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

func TestForMergeNotes(t *testing.T) {
	mi := info(model.MergeBlocked)
	mi.Repo.AutoMerge = true
	mi.Files = []string{".github/workflows/ci.yaml", "main.go"}
	mi.Checks = []model.Check{{Name: "lint", State: model.CheckFailure}}
	d := ForMerge(mi, model.PR{Author: "renovate", AuthorIsBot: true}, "me", set)
	all := strings.Join(d.Notes, "\n")
	for _, want := range []string{"changes .github/workflows/ci.yaml", "not required failed: lint", "pushed after it is on"} {
		if !strings.Contains(all, want) {
			t.Errorf("notes %q lack %q", d.Notes, want)
		}
	}
	if strings.Contains(all, "main.go") {
		t.Error("main.go marked")
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
	mi.Checks = []model.Check{{Name: "test", State: "CANCELLED", Required: true}}
	if !Failing(mi) {
		t.Error("a cancelled required check is not failing")
	}
}
