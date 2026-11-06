package app

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/babarot/gh-desk/internal/model"
	"github.com/babarot/gh-desk/internal/plan"
)

// excludedBy is what the runner asks right before it sends an approval a
// rule asked for: whether an [[exclude]] rule keeps the pull request from
// it (ADR-092). The pull request is read again, as a label may have come
// while the approval waited, and judged with the checks of mi, which the
// runner read with their details, so that checks waiting only for an
// approval are told as the list tells them. A rule that cannot tell keeps
// it too. Nil when there are no exclusions.
func excludedBy(gh prReader, rules []plan.Rule, viewer string, set plan.Settings, now func() time.Time) func(context.Context, model.PR, model.MergeInfo) (string, error) {
	if len(rules) == 0 {
		return nil
	}
	return func(ctx context.Context, pr model.PR, mi model.MergeInfo) (string, error) {
		got, err := gh.PullRequests(ctx, []model.PRKey{pr.Key})
		if err != nil {
			return "", err
		}
		p, ok := got[pr.Key]
		if !ok {
			return "", errors.New("not found")
		}
		// No body: [[exclude]] takes no packages. No notification: the
		// runner has none, so a rule on reasons says No here.
		f := plan.Facts{Now: now(), ApprovalKnown: true, AwaitingApproval: plan.AwaitingApproval(mi.Checks, set.ApprovalChecks)}
		for _, r := range rules {
			switch r.Judge(p, viewer, f) {
			case plan.Yes:
				return ruleLabel(r), nil
			case plan.Pending:
				return ruleLabel(r) + " (cannot tell yet)", nil
			}
		}
		return "", nil
	}
}

// ruleLabel names a rule in what the runner says: its name, or that it
// is one of [[exclude]].
func ruleLabel(r plan.Rule) string {
	if r.Name != "" {
		return r.Name
	}
	return "a rule of [[exclude]]"
}

// defaultAutoQuery is the search a rule without a query reads: the open
// pull requests that are not drafts and whose review is asked of the
// viewer, which leave it once approved (ADR-092).
const defaultAutoQuery = "is:pr is:open draft:false review-requested:@me"

// autoQuery is the search of a rule without one: the default search,
// narrowed by the rule's repositories only when each of them can be
// written as a qualifier. Narrowing by some of them would drop the pull
// requests of the rest, which the rule matches. Authors are left out: a
// bot is written without app/, and searched only with it.
func autoQuery(repos []string) string {
	var qs []string
	for _, g := range repos {
		owner, repo, _ := strings.Cut(g, "/")
		switch {
		case strings.ContainsAny(owner, "*?[\\"):
			return defaultAutoQuery
		case repo == "*":
			qs = append(qs, "user:"+owner)
		case strings.ContainsAny(repo, "*?[\\"):
			return defaultAutoQuery
		default:
			qs = append(qs, "repo:"+g)
		}
	}
	if len(qs) == 0 {
		return defaultAutoQuery
	}
	return defaultAutoQuery + " " + strings.Join(qs, " ")
}
