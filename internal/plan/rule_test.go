package plan

import (
	"regexp"
	"testing"

	"github.com/babarot/gh-triage/internal/model"
)

func TestRuleMatch(t *testing.T) {
	bot := model.PR{Author: "renovate", AuthorIsBot: true, State: model.StateOpen, ChangedFiles: 1, Title: "Update actions/checkout to v5", Checks: model.CheckSuccess}
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
		{"author", Rule{Author: "app/renovate"}, bot, Facts{}, true},
		{"other author", Rule{Author: "dependabot"}, bot, Facts{}, false},
		{"title", Rule{Title: regexp.MustCompile(`^Update actions/`)}, bot, Facts{}, true},
		{"max files", Rule{MaxFiles: 0}, bot, Facts{}, true},
		{"too many files", Rule{MaxFiles: 1}, model.PR{AuthorIsBot: true, ChangedFiles: 3}, Facts{}, false},
		{"lines at the cap", Rule{MaxLines: 500}, model.PR{Additions: 300, Deletions: 200}, Facts{}, true},
		{"too many lines", Rule{MaxLines: 500}, model.PR{Additions: 300, Deletions: 201}, Facts{}, false},
		{"package listed", Rule{Packages: []string{"actions/checkout"}}, bot, Facts{Body: body, BodyKnown: true}, true},
		{"package not listed", Rule{Packages: []string{"songmu/tagpr"}}, bot, Facts{Body: body, BodyKnown: true}, false},
		{"body not read", Rule{Packages: []string{"actions/checkout"}}, bot, Facts{}, false},
		{"checks failing", Rule{ChecksPassing: true}, failing, Facts{}, false},
		{"approval check pending", Rule{ChecksPassing: true}, pending, Facts{AwaitingApproval: true}, true},
		{"checks running", Rule{ChecksPassing: true}, pending, Facts{}, false},
		{"approved by me", ApprovedByMe, approved, Facts{}, true},
		{"not approved", ApprovedByMe, bot, Facts{}, false},
		{"a person's", ApprovedByMe, person, Facts{}, false},
		{"a person's, by a key", Rule{ApprovedByMe: true}, person, Facts{}, true},
		{"not approved, by a key", Rule{ApprovedByMe: true}, bot, Facts{}, false},
		{"draft", Rule{}, model.PR{Draft: true}, Facts{}, false},
	}
	for _, tt := range tests {
		if got := tt.rule.Match(tt.pr, "me", tt.f); got != tt.want {
			t.Errorf("%s: Match = %v", tt.name, got)
		}
	}
}
