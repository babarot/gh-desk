package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/babarot/gh-desk/internal/model"
	"github.com/babarot/gh-desk/internal/plan"
)

type prsByKey map[model.PRKey]model.PR

func (f prsByKey) PullRequests(_ context.Context, keys []model.PRKey) (map[model.PRKey]model.PR, error) {
	if f == nil {
		return nil, errors.New("read failed")
	}
	out := map[model.PRKey]model.PR{}
	for _, k := range keys {
		if p, ok := f[k]; ok {
			out[k] = p
		}
	}
	return out, nil
}

// The exclusions are judged on the pull request read again, with the
// checks the runner read: checks waiting only for an approval are told,
// so a rule on checks says Yes or No, not Pending.
func TestExcludedBy(t *testing.T) {
	k := model.PRKey{Owner: "acme", Repo: "api", Number: 1}
	held := model.PR{Key: k, State: model.StateOpen, Author: "renovate", HeadOID: "h", HeavyHead: "h", Labels: []string{"hold"}}
	waiting := model.PR{Key: k, State: model.StateOpen, Author: "renovate", HeadOID: "h", HeavyHead: "h", Checks: model.CheckPending}
	rules := []plan.Rule{
		{Name: "held", Labels: []string{"hold"}},
		{Checks: []plan.Checks{plan.ChecksFailing, plan.ChecksPending}},
	}
	set := plan.Settings{ApprovalChecks: []string{"policy-bot*"}}
	onlyApproval := model.MergeInfo{Checks: []model.Check{{Name: "policy-bot: main", State: model.CheckPending}}}
	tests := []struct {
		name string
		prs  prsByKey
		mi   model.MergeInfo
		why  string
		err  bool
	}{
		{"labeled", prsByKey{k: held}, model.MergeInfo{}, "held", false},
		{"only an approval check pending", prsByKey{k: waiting}, onlyApproval, "", false},
		{"a job pending", prsByKey{k: waiting}, model.MergeInfo{Checks: []model.Check{{Name: "test", State: model.CheckPending}}}, "a rule of [[exclude]]", false},
		{"read failed", nil, model.MergeInfo{}, "", true},
	}
	for _, tt := range tests {
		f := excludedBy(tt.prs, rules, "me", set, time.Now)
		why, err := f(t.Context(), model.PR{Key: k}, tt.mi)
		if why != tt.why || (err != nil) != tt.err {
			t.Errorf("%s: excludedBy = %q, %v", tt.name, why, err)
		}
	}
	if excludedBy(prsByKey{}, nil, "me", set, time.Now) != nil {
		t.Error("no exclusions should ask nothing")
	}
}

// A rule's repositories narrow the default search only when every one of
// them can be searched; authors never do.
func TestAutoQuery(t *testing.T) {
	tests := []struct {
		repos []string
		want  string
	}{
		{nil, defaultAutoQuery},
		{[]string{"acme/api", "acme/web"}, defaultAutoQuery + " repo:acme/api repo:acme/web"},
		{[]string{"acme/*"}, defaultAutoQuery + " user:acme"},
		{[]string{"acme/api", "other*/worker"}, defaultAutoQuery},
		{[]string{"acme/api-*"}, defaultAutoQuery},
	}
	for _, tt := range tests {
		if got := autoQuery(tt.repos); got != tt.want {
			t.Errorf("autoQuery(%v) = %q, want %q", tt.repos, got, tt.want)
		}
	}
}
