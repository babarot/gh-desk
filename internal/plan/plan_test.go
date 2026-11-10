package plan

import (
	"reflect"
	"testing"

	"github.com/babarot/gh-desk/internal/model"
)

// The sections a tab of review requests is split into by default put each
// pull request where the four groups did: a bot's under bots, whatever is
// asked of whom, and other is what neither you nor a team is asked of.
func TestSectionsOfTheGroups(t *testing.T) {
	me := []model.Reviewer{{Login: "me"}}
	team := []model.Reviewer{{Login: "core", Team: true}}
	ss := []Section{
		{Name: "requested from you", Rule: Rule{Requested: []string{"me"}, Bot: new(false)}},
		{Name: "requested from a team", Rule: Rule{Requested: []string{AnyTeam}, Bot: new(false)}},
		{Name: "other", Rule: Rule{Bot: new(false)}},
		{Name: "bots", Rule: Rule{Bot: new(true)}},
	}
	tests := []struct {
		pr   model.PR
		want int
	}{
		{model.PR{Requested: me}, 0},
		{model.PR{Requested: team}, 1},
		{model.PR{Requested: append(team, me...)}, 0},
		{model.PR{Requested: me, AuthorIsBot: true}, 3},
		{model.PR{Requested: team, AuthorIsBot: true}, 3},
		{model.PR{Requested: []model.Reviewer{{Login: "someone"}}}, 2},
		{model.PR{}, 2},
	}
	for _, tt := range tests {
		if got, _ := Place(ss, tt.pr, "me", Facts{}); got != tt.want {
			t.Errorf("Place(%+v) = %d; want %d", tt.pr.Requested, got, tt.want)
		}
	}
}

func TestChecksPass(t *testing.T) {
	tests := []struct {
		state    model.CheckState
		awaiting bool
		want     bool
	}{
		{model.CheckSuccess, false, true},
		{"", false, true},
		{model.CheckPending, false, false},
		{model.CheckPending, true, true},
		{model.CheckFailure, true, false},
	}
	for _, tt := range tests {
		if got := ChecksPass(model.PR{Checks: tt.state}, tt.awaiting); got != tt.want {
			t.Errorf("ChecksPass(%s, %v) = %v", tt.state, tt.awaiting, got)
		}
	}
}

func TestMatchAny(t *testing.T) {
	pats := []string{"*.lock", "go.sum", ".github/workflows/*"}
	for name, want := range map[string]bool{
		"pubspec.lock":              true,
		"web/yarn.lock":             true,
		"go.sum":                    true,
		"tools/go.sum":              true,
		".github/workflows/ci.yaml": true,
		"main.go":                   false,
	} {
		if got := MatchAny(pats, name); got != want {
			t.Errorf("MatchAny(%q) = %v", name, got)
		}
	}
}

func TestBumps(t *testing.T) {
	renovate := "This PR contains the following updates:\n\n" +
		"| Package | Update | Change | Pending |\n|---|---|---|---|\n" +
		"| [octo-sts/app](https://redirect.github.com/octo-sts/app) | minor | `v0.12.0` → `v0.13.2` | `v0.13.3` |\n" +
		"| actions/checkout | digest | `abc` -> `def` |  |\n\n---\n\n| other | table |\n"
	dependabot := "Bumps [golang.org/x/net](https://github.com/golang/net) from 0.30.0 to 0.31.0.\n"
	tests := []struct {
		name, body string
		want       []Bump
	}{
		{"renovate", renovate, []Bump{
			{Package: "octo-sts/app", Update: "minor", From: "v0.12.0", To: "v0.13.2", Pending: "v0.13.3"},
			{Package: "actions/checkout", Update: "digest", From: "abc", To: "def"},
		}},
		{"dependabot", dependabot, []Bump{{Package: "golang.org/x/net", From: "0.30.0", To: "0.31.0"}}},
		{"none", "Just a description.", nil},
	}
	for _, tt := range tests {
		if got := Bumps(tt.body); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("%s: Bumps = %+v", tt.name, got)
		}
	}
}
