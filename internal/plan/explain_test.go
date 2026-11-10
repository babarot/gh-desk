package plan

import (
	"fmt"
	"reflect"
	"regexp"
	"testing"
	"time"

	"github.com/babarot/gh-desk/internal/model"
)

// heavyConds are the conditions Judge looks at after the others, once the
// heavy fields and the body are read.
var heavyConds = map[Cond]bool{CondMaxFiles: true, CondMaxLines: true, CondMinFiles: true, CondMinLines: true,
	CondDecisions: true, CondMerges: true, CondLabels: true, CondChecks: true, CondPackages: true}

// Explain never says other than Judge, over every rule of one condition
// and of two, every pull request and every set of facts here: Yes when
// every condition holds, No when one fails, Pending when none Judge looks
// at first fails and one cannot be told.
func TestExplainTellsWhatJudgeDecides(t *testing.T) {
	day0 := time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)
	read := model.PR{Key: model.PRKey{Owner: "acme", Repo: "web", Number: 1}, Author: "renovate", AuthorIsBot: true, State: model.StateOpen,
		Title: "Update actions/checkout to v5", Updated: day0.Add(-40 * 24 * time.Hour), Created: day0.Add(-60 * 24 * time.Hour),
		Additions: 400, Deletions: 200, ChangedFiles: 12, Decision: model.DecisionChangesRequested, Checks: model.CheckFailure,
		Labels: []string{"WIP"}, Requested: []model.Reviewer{{Login: "me"}, {Login: "platform", Team: true}},
		Reviews: []model.Review{{Author: "me", State: model.ReviewApproved}}, HeadOID: "h", HeavyHead: "h"}
	unread := read
	unread.HeavyHead = ""
	pending := read
	pending.Checks = model.CheckPending
	passing := read
	passing.Checks, passing.Decision, passing.Labels, passing.Merge = model.CheckSuccess, "", nil, model.MergeClean
	person := read
	person.Author, person.AuthorIsBot, person.Draft, person.State, person.Reviews = "me", false, true, model.StateMerged, nil
	prs := map[string]model.PR{"read": read, "unread": unread, "pending": pending, "passing": passing, "person": person, "empty": {}}

	body := "| Package | Update | Change |\n|---|---|---|\n| actions/checkout | major | `v4` → `v5` |\n"
	facts := map[string]Facts{
		"now":              {Now: day0, ApprovalKnown: true},
		"approval unknown": {Now: day0},
		"awaiting":         {Now: day0, ApprovalKnown: true, AwaitingApproval: true},
		"reason":           {Now: day0, ApprovalKnown: true, Reason: "review_requested"},
		"body":             {Now: day0, ApprovalKnown: true, Body: body, BodyKnown: true},
	}
	one := []Rule{
		{Repos: []string{"acme/*"}}, {Repos: []string{"babarot/*"}},
		{Authors: []string{"app/renovate"}}, {Authors: []string{"alice"}},
		{Bot: new(true)}, {Bot: new(false)}, {Draft: new(true)}, {Draft: new(false)},
		{Mine: new(true)}, {Mine: new(false)},
		{States: []model.PRState{model.StateOpen}}, {States: []model.PRState{model.StateMerged, model.StateClosed}},
		{Title: regexp.MustCompile(`checkout`)}, {Title: regexp.MustCompile(`^Fix`)},
		{UpdatedWithinDays: 50}, {UpdatedWithinDays: 7}, {CreatedWithinDays: 90}, {UpdatedBeforeDays: 30}, {CreatedBeforeDays: 90},
		{Reasons: []string{"review_requested"}}, {Requested: []string{"me"}}, {Requested: []string{"@core"}}, {Requested: []string{AnyTeam}},
		{ApprovedByMe: new(true)}, {ApprovedByMe: new(false)}, {ReviewedByMe: new(true)}, {ReviewedByMe: new(false)},
		{MaxFiles: 20}, {MaxFiles: 12}, {MaxFiles: 11}, {MaxLines: 600}, {MaxLines: 599}, {MinFiles: 12}, {MinFiles: 13}, {MinLines: 600}, {MinLines: 601},
		{UpdatedWithinDays: 40}, {UpdatedBeforeDays: 40}, {UpdatedBeforeDays: 41}, {CreatedWithinDays: 60},
		{Decisions: []model.ReviewDecision{model.DecisionChangesRequested}}, {Decisions: []model.ReviewDecision{NoDecision}},
		{Merges: []Merges{MergeReady}}, {Merges: []Merges{MergeConflict, MergeUnknown}},
		{Labels: []string{"wip"}}, {Labels: []string{"do-not-merge"}},
		{Checks: []Checks{ChecksPassing}}, {Checks: []Checks{ChecksFailing}}, {Checks: []Checks{ChecksPending}},
		{Packages: []string{"actions/checkout"}}, {Packages: []string{"x"}},
	}
	rules := append([]Rule(nil), one...)
	for i := range one {
		for j := i + 1; j < len(one); j++ {
			rules = append(rules, merge(one[i], one[j]))
		}
	}
	for _, viewer := range []string{"me", ""} {
		for pn, pr := range prs {
			for fn, f := range facts {
				for _, r := range rules {
					judged := r.Judge(pr, viewer, f)
					es := r.Explain(pr, viewer, f)
					var yes, no, lightNo, pending int
					for _, e := range es {
						switch e.Verdict {
						case Yes:
							yes++
						case No:
							no++
							if !heavyConds[e.Cond] {
								lightNo++
							}
						case Pending:
							pending++
							if e.Need == "" {
								t.Errorf("%s cannot be told, for nothing: %+v of %s, %s", e.Cond, r, pn, fn)
							}
						}
					}
					where := fmt.Sprintf("%+v of %s, %s, viewer %q", r, pn, fn, viewer)
					switch judged {
					case Yes:
						if yes != len(es) {
							t.Errorf("Judge says Yes, Explain %v: %s", es, where)
						}
					case No:
						if no == 0 {
							t.Errorf("Judge says No, Explain fails none: %v: %s", es, where)
						}
					case Pending:
						if lightNo > 0 || pending == 0 {
							t.Errorf("Judge says Pending, Explain %v: %s", es, where)
						}
					}
				}
			}
		}
	}
}

// merge is a rule with the conditions of both.
func merge(a, b Rule) Rule {
	va, vb := reflect.ValueOf(&a).Elem(), reflect.ValueOf(b)
	for i := range va.NumField() {
		if !vb.Field(i).IsZero() {
			va.Field(i).Set(vb.Field(i))
		}
	}
	return a
}

func TestExplainSaysWhatMatched(t *testing.T) {
	day0 := time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)
	pr := model.PR{Key: model.PRKey{Owner: "acme", Repo: "web", Number: 1}, Author: "app/renovate", Title: "Configure Renovate",
		Updated: day0.Add(-40*24*time.Hour - time.Hour), Labels: []string{"WIP", "deps"}, Checks: model.CheckPending,
		Requested: []model.Reviewer{{Login: "platform", Team: true}}, HeadOID: "h", HeavyHead: "h"}
	r := Rule{Repos: []string{"babarot/*", "ACME/*"}, Authors: []string{"renovate"}, Title: regexp.MustCompile(`Con`),
		UpdatedBeforeDays: 30, Labels: []string{"wip", "x"}, Requested: []string{"@platform"}, Checks: []Checks{ChecksPassing},
		Packages: []string{"x"}}
	got := r.Explain(pr, "me", Facts{Now: day0})
	want := []Explained{
		{Cond: CondRepos, Verdict: Yes, Values: []string{"acme/web"}, Matched: []string{"ACME/*"}},
		{Cond: CondAuthors, Verdict: Yes, Values: []string{"app/renovate"}, Matched: []string{"renovate"}},
		{Cond: CondTitle, Verdict: Yes, Values: []string{"Configure Renovate"}, Title: [3]string{"", "Con", "figure Renovate"}},
		{Cond: CondUpdatedBeforeDays, Verdict: Yes, Values: []string{"40"}},
		{Cond: CondRequested, Verdict: Yes, Values: []string{"@platform"}, Matched: []string{"@platform"}},
		{Cond: CondLabels, Verdict: Yes, Values: []string{"WIP"}, Matched: []string{"wip"}},
		{Cond: CondChecks, Verdict: Pending, Need: NeedApproval, Values: []string{"PENDING"}},
		{Cond: CondPackages, Verdict: Pending, Need: NeedBody},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Explain =\n%+v\nwant\n%+v", got, want)
	}

	// A title that does not match, and heavy fields not read for the head.
	pr.HeavyHead = ""
	got = Rule{Title: regexp.MustCompile(`^Fix`), MaxLines: 10}.Explain(pr, "me", Facts{Now: day0})
	want = []Explained{
		{Cond: CondTitle, Verdict: No, Values: []string{"Configure Renovate"}},
		{Cond: CondMaxLines, Verdict: Pending, Need: NeedHeavy},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Explain =\n%+v\nwant\n%+v", got, want)
	}

	// A title split where the pattern first matches, in bytes of UTF-8.
	jp := model.PR{Title: "依存を更新: Renovate の設定"}
	got = Rule{Title: regexp.MustCompile(`Renovate`)}.Explain(jp, "me", Facts{})
	if got[0].Title != [3]string{"依存を更新: ", "Renovate", " の設定"} {
		t.Errorf("title split %q", got[0].Title)
	}
}
