package plan

import (
	"reflect"
	"testing"

	"github.com/babarot/gh-triage/internal/model"
)

func TestGroupOf(t *testing.T) {
	me := []model.Reviewer{{Login: "me"}}
	team := []model.Reviewer{{Login: "core", Team: true}}
	tests := []struct {
		pr   model.PR
		want Group
	}{
		{model.PR{Requested: me}, GroupYou},
		{model.PR{Requested: team}, GroupTeam},
		{model.PR{Requested: append(team, me...)}, GroupYou},
		{model.PR{Requested: me, AuthorIsBot: true}, GroupBots},
		{model.PR{}, GroupTeam},
	}
	for _, tt := range tests {
		if got := GroupOf(tt.pr, "me"); got != tt.want {
			t.Errorf("GroupOf(%+v) = %s; want %s", tt.pr.Requested, got, tt.want)
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
