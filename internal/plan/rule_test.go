package plan

import (
	"regexp"
	"testing"
	"time"

	"github.com/babarot/gh-desk/internal/model"
)

func TestRuleMatch(t *testing.T) {
	day0 := time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)
	// The heavy fields of these are read for their head.
	bot := model.PR{Author: "renovate", AuthorIsBot: true, State: model.StateOpen, ChangedFiles: 1, Title: "Update actions/checkout to v5", Checks: model.CheckSuccess,
		HeadOID: "h", HeavyHead: "h"}
	body := "| Package | Update | Change |\n|---|---|---|\n| actions/checkout | major | `v4` → `v5` |\n"
	approved := bot
	approved.Reviews = []model.Review{{Author: "me", State: model.ReviewApproved}}
	failing := bot
	failing.Checks = model.CheckFailure
	pending := bot
	pending.Checks = model.CheckPending
	person := approved
	person.AuthorIsBot, person.Author = false, "alice"
	tests := []struct {
		name string
		rule Rule
		pr   model.PR
		f    Facts
		want bool
	}{
		{"author", Rule{Authors: []string{"app/renovate"}}, bot, Facts{}, true},
		{"one of the authors", Rule{Authors: []string{"dependabot", "Renovate"}}, bot, Facts{}, true},
		{"other author", Rule{Authors: []string{"dependabot"}}, bot, Facts{}, false},
		{"title", Rule{Title: regexp.MustCompile(`^Update actions/`)}, bot, Facts{}, true},
		{"max files", Rule{MaxFiles: 0}, bot, Facts{}, true},
		{"too many files", Rule{MaxFiles: 1}, model.PR{AuthorIsBot: true, ChangedFiles: 3, HeadOID: "h", HeavyHead: "h"}, Facts{}, false},
		{"lines at the cap", Rule{MaxLines: 500}, model.PR{Additions: 300, Deletions: 200, HeadOID: "h", HeavyHead: "h"}, Facts{}, true},
		{"lines not read for the head", Rule{MaxLines: 500}, model.PR{Additions: 300, Deletions: 200, HeadOID: "h2", HeavyHead: "h"}, Facts{}, false},
		{"too many lines", Rule{MaxLines: 500}, model.PR{Additions: 300, Deletions: 201, HeadOID: "h", HeavyHead: "h"}, Facts{}, false},
		{"updated on the last day", Rule{UpdatedWithinDays: 90}, model.PR{Updated: day0.Add(-90 * 24 * time.Hour)}, Facts{Now: day0}, true},
		{"updated a second too early", Rule{UpdatedWithinDays: 90}, model.PR{Updated: day0.Add(-90*24*time.Hour - time.Second)}, Facts{Now: day0}, false},
		{"updated time not read", Rule{UpdatedWithinDays: 90}, model.PR{}, Facts{Now: day0}, false},
		{"updated ahead of the clock", Rule{UpdatedWithinDays: 1}, model.PR{Updated: day0.Add(time.Hour)}, Facts{Now: day0}, true},
		{"created lately", Rule{CreatedWithinDays: 7}, model.PR{Created: day0.Add(-24 * time.Hour), Updated: day0}, Facts{Now: day0}, true},
		{"created long ago, updated lately", Rule{CreatedWithinDays: 7}, model.PR{Created: day0.Add(-30 * 24 * time.Hour), Updated: day0}, Facts{Now: day0}, false},
		{"package listed", Rule{Packages: []string{"actions/checkout"}}, bot, Facts{Body: body, BodyKnown: true}, true},
		{"package not listed", Rule{Packages: []string{"songmu/tagpr"}}, bot, Facts{Body: body, BodyKnown: true}, false},
		{"body not read", Rule{Packages: []string{"actions/checkout"}}, bot, Facts{}, false},
		{"checks failing", Rule{Checks: ChecksPassing}, failing, Facts{}, false},
		{"approval check pending", Rule{Checks: ChecksPassing}, pending, Facts{AwaitingApproval: true, ApprovalKnown: true}, true},
		{"approval not known yet", Rule{Checks: ChecksPassing}, pending, Facts{AwaitingApproval: true}, false},
		{"checks running", Rule{Checks: ChecksPassing}, pending, Facts{}, false},
		{"approved by me", ApprovedByMe, approved, Facts{}, true},
		{"not approved", ApprovedByMe, bot, Facts{}, false},
		{"a person's", ApprovedByMe, person, Facts{}, false},
		{"a person's, by a key", Rule{ApprovedByMe: true}, person, Facts{}, true},
		{"not approved, by a key", Rule{ApprovedByMe: true}, bot, Facts{}, false},
		{"draft", Rule{}, model.PR{Draft: true}, Facts{}, false},
		{"merged", Rule{Bot: true}, model.PR{AuthorIsBot: true, State: model.StateMerged}, Facts{}, false},
	}
	for _, tt := range tests {
		if got := tt.rule.Match(tt.pr, "me", tt.f); got != tt.want {
			t.Errorf("%s: Match = %v", tt.name, got)
		}
	}
}

func TestRuleJudge(t *testing.T) {
	day0 := time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)
	read := model.PR{Key: model.PRKey{Owner: "acme", Repo: "web", Number: 1}, Author: "alice", State: model.StateOpen, Title: "Upgrade x",
		Updated: day0.Add(-40 * 24 * time.Hour), Created: day0.Add(-60 * 24 * time.Hour),
		Additions: 400, Deletions: 200, ChangedFiles: 12, Decision: model.DecisionChangesRequested, Checks: model.CheckFailure,
		Labels: []string{"WIP"}, Requested: []model.Reviewer{{Login: "me"}, {Login: "platform", Team: true}},
		Reviews: []model.Review{{Author: "me", State: model.ReviewCommented}}, HeadOID: "h", HeavyHead: "h"}
	unread := read
	unread.HeavyHead = ""
	draft := read
	draft.Draft, draft.State = true, model.StateMerged
	pending := read
	pending.Checks = model.CheckPending
	mine := read
	mine.Author = "me"
	now := Facts{Now: day0, ApprovalKnown: true}
	tests := []struct {
		name string
		rule Rule
		pr   model.PR
		f    Facts
		want Verdict
	}{
		{"repo glob", Rule{Repos: []string{"ACME/*"}}, read, now, Yes},
		{"other repo", Rule{Repos: []string{"acme/api", "babarot/*"}}, read, now, No},
		{"mine", Rule{Mine: true}, read, now, No},
		{"mine, by the author", Rule{Mine: true}, mine, now, Yes},
		{"draft", Rule{Draft: true}, draft, now, Yes},
		{"not a draft", Rule{Draft: true}, read, now, No},
		{"merged is judged as any other", Rule{Title: regexp.MustCompile(`^Upgrade`)}, draft, now, Yes},
		{"updated before", Rule{UpdatedBeforeDays: 40}, read, now, Yes},
		{"updated a day too late", Rule{UpdatedBeforeDays: 41}, read, now, No},
		{"updated time not read", Rule{UpdatedBeforeDays: 1}, model.PR{}, now, No},
		{"created before", Rule{CreatedBeforeDays: 60}, read, now, Yes},
		{"reviewed by me", Rule{ReviewedByMe: true}, read, now, Yes},
		{"approved by me, only commented", Rule{ApprovedByMe: true}, read, now, No},
		{"requested from me", Rule{Requested: []string{"me"}}, read, now, Yes},
		{"requested from the team", Rule{Requested: []string{"@Platform"}}, read, now, Yes},
		{"requested from another team", Rule{Requested: []string{"@core"}}, read, now, No},
		{"reason", Rule{Reasons: []string{"mention", "subscribed"}}, read, Facts{Now: day0, Reason: "subscribed"}, Yes},
		{"no notification", Rule{Reasons: []string{"subscribed"}}, read, now, No},
		{"labels", Rule{Labels: []string{"do-not-merge", "wip"}}, read, now, Yes},
		{"no such label", Rule{Labels: []string{"do-not-merge"}}, read, now, No},
		{"decision", Rule{Decision: model.DecisionChangesRequested}, read, now, Yes},
		{"min lines", Rule{MinLines: 600}, read, now, Yes},
		{"too few lines", Rule{MinLines: 601}, read, now, No},
		{"min files", Rule{MinFiles: 13}, read, now, No},
		{"failing", Rule{Checks: ChecksFailing}, read, now, Yes},
		{"pending", Rule{Checks: ChecksPending}, pending, now, Yes},
		{"pending only for an approval", Rule{Checks: ChecksPending}, pending, Facts{Now: day0, ApprovalKnown: true, AwaitingApproval: true}, No},
		{"pending, approval not known", Rule{Checks: ChecksPending}, pending, Facts{Now: day0}, Pending},
		{"failing, approval not known", Rule{Checks: ChecksFailing}, read, Facts{Now: day0}, Yes},
		{"failing asked of pending checks, approval not known", Rule{Checks: ChecksFailing}, pending, Facts{Now: day0}, No},
		{"heavy not read", Rule{Labels: []string{"wip"}}, unread, now, Pending},
		{"light says no before heavy is read", Rule{Labels: []string{"wip"}, Title: regexp.MustCompile(`^Fix`)}, unread, now, No},
		{"light only, heavy not read", Rule{Title: regexp.MustCompile(`^Upgrade`)}, unread, now, Yes},
		{"body not read", Rule{Packages: []string{"x"}}, read, now, Pending},
	}
	for _, tt := range tests {
		if got := tt.rule.Judge(tt.pr, "me", tt.f); got != tt.want {
			t.Errorf("%s: Judge = %v, want %v", tt.name, got, tt.want)
		}
	}
	// Before the viewer is known, no pull request is the viewer's.
	if got := (Rule{Mine: true}).Judge(mine, "", now); got != No {
		t.Errorf("mine, viewer unknown: Judge = %v, want No", got)
	}
}
