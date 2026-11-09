package plan

import (
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/babarot/gh-desk/internal/model"
)

// Cond names a condition of a Rule. The names are this package's: what a
// config file calls a condition is the caller's to say.
type Cond string

const (
	CondRepos             Cond = "repos"
	CondAuthors           Cond = "authors"
	CondBot               Cond = "bot"
	CondDraft             Cond = "draft"
	CondMine              Cond = "mine"
	CondStates            Cond = "states"
	CondTitle             Cond = "title"
	CondUpdatedWithinDays Cond = "updated_within_days"
	CondCreatedWithinDays Cond = "created_within_days"
	CondUpdatedBeforeDays Cond = "updated_before_days"
	CondCreatedBeforeDays Cond = "created_before_days"
	CondReasons           Cond = "reasons"
	CondRequested         Cond = "requested"
	CondApprovedByMe      Cond = "approved_by_me"
	CondReviewedByMe      Cond = "reviewed_by_me"
	CondMaxFiles          Cond = "max_files"
	CondMaxLines          Cond = "max_lines"
	CondMinFiles          Cond = "min_files"
	CondMinLines          Cond = "min_lines"
	CondDecisions         Cond = "decisions"
	CondLabels            Cond = "labels"
	CondChecks            Cond = "checks"
	CondPackages          Cond = "packages"
)

// Need is what a condition that cannot be told yet waits for.
type Need string

const (
	// NeedHeavy: the heavy fields, read for the head shown.
	NeedHeavy Need = "heavy"
	// NeedBody: the body, for the packages a bot's pull request updates.
	NeedBody Need = "body"
	// NeedApproval: whether the pending checks only wait for an approval.
	NeedApproval Need = "approval"
)

// Explained is what one condition of a rule says of a pull request.
type Explained struct {
	Cond Cond
	// Verdict is Yes when the condition holds, No when it does not, and
	// Pending when it cannot be told yet, for Need.
	Verdict Verdict
	Need    Need
	// Values are the pull request's side of the condition, as words: the
	// repository, the author, the labels, the days since it was updated.
	// When the condition holds and asks for one of several values, they
	// are the ones it has; otherwise all it has.
	Values []string
	// Matched are the rule's values that the pull request meets: the
	// globs its repository matches, the authors, labels, reviewers asked,
	// states and the like. Empty for a condition with one value.
	Matched []string
	// Title is the title split where the rule's pattern first matches it:
	// before, the match, after. Set only for a title that holds.
	Title [3]string
}

// Explain says, condition by condition, what the rule makes of pr for
// viewer, in the order Judge looks at them. It tells what Judge decides:
// for a pull request Judge says Yes of, every condition holds; No, one
// does not; Pending, none of those Judge looks at first fails and one
// cannot be told yet. It does not decide in Judge's place: Judge says
// Pending before it looks at the heavy fields and the packages, while
// Explain still tells the ones it can.
func (r Rule) Explain(pr model.PR, viewer string, f Facts) []Explained {
	var out []Explained
	add := func(c Cond, ok bool, values, matched []string) {
		v := No
		if ok {
			v = Yes
		}
		out = append(out, Explained{Cond: c, Verdict: v, Values: values, Matched: matched})
	}
	pick := func(c Cond, want []string, values []string, meets func(string) bool) {
		var m []string
		for _, w := range want {
			if meets(w) {
				m = append(m, w)
			}
		}
		add(c, len(m) > 0, values, m)
	}
	flag := func(c Cond, want *bool, is, needsViewer bool) {
		if want == nil {
			return
		}
		add(c, (!needsViewer || viewer != "") && *want == is, []string{strconv.FormatBool(is)}, nil)
	}

	if len(r.Repos) > 0 {
		pick(CondRepos, r.Repos, []string{pr.Key.Repository()}, func(g string) bool { return repoMatches(g, pr) })
	}
	if len(r.Authors) > 0 {
		pick(CondAuthors, r.Authors, []string{pr.Author}, func(a string) bool { return authorIs(a, pr) })
	}
	flag(CondBot, r.Bot, pr.AuthorIsBot, false)
	flag(CondDraft, r.Draft, pr.Draft, false)
	flag(CondMine, r.Mine, viewer != "" && pr.Author == viewer, true)
	if len(r.States) > 0 {
		st := stateOf(pr)
		pick(CondStates, words(r.States), []string{string(st)}, func(s string) bool { return model.PRState(s) == st })
	}
	if r.Title != nil {
		e := Explained{Cond: CondTitle, Verdict: No, Values: []string{pr.Title}}
		if at := r.Title.FindStringIndex(pr.Title); at != nil {
			e.Verdict = Yes
			e.Title = [3]string{pr.Title[:at[0]], pr.Title[at[0]:at[1]], pr.Title[at[1]:]}
		}
		out = append(out, e)
	}
	days := func(c Cond, n int, t time.Time, ok func(time.Time, time.Time, int) bool) {
		if n > 0 {
			add(c, ok(t, f.Now, n), daysSince(t, f.Now), nil)
		}
	}
	days(CondUpdatedWithinDays, r.UpdatedWithinDays, pr.Updated, within)
	days(CondCreatedWithinDays, r.CreatedWithinDays, pr.Created, within)
	days(CondUpdatedBeforeDays, r.UpdatedBeforeDays, pr.Updated, before)
	days(CondCreatedBeforeDays, r.CreatedBeforeDays, pr.Created, before)
	if len(r.Reasons) > 0 {
		pick(CondReasons, r.Reasons, nonEmpty(f.Reason), func(s string) bool { return reasonIs(s, f) })
	}
	if len(r.Requested) > 0 {
		var asked []string
		for _, rv := range pr.Requested {
			if rv.Team {
				asked = append(asked, "@"+rv.Login)
			} else {
				asked = append(asked, rv.Login)
			}
		}
		pick(CondRequested, r.Requested, asked, func(s string) bool { return requested(pr, viewer, s) })
	}
	rv, reviewed := pr.ReviewBy(viewer)
	flag(CondApprovedByMe, r.ApprovedByMe, reviewed && rv.State == model.ReviewApproved, true)
	flag(CondReviewedByMe, r.ReviewedByMe, reviewed, true)

	// The heavy fields are told only when read for the head shown: unread,
	// they look like an empty pull request.
	heavy := func(c Cond, set bool, ok func() bool, values func() []string, matched func() []string) {
		if !set {
			return
		}
		if !pr.HeavyRead() {
			out = append(out, Explained{Cond: c, Verdict: Pending, Need: NeedHeavy})
			return
		}
		var m []string
		if matched != nil {
			m = matched()
		}
		add(c, ok(), values(), m)
	}
	files := func() []string { return []string{strconv.Itoa(pr.ChangedFiles)} }
	lines := func() []string { return []string{strconv.Itoa(linesOf(pr))} }
	heavy(CondMaxFiles, r.MaxFiles > 0, func() bool { return atMost(pr.ChangedFiles, r.MaxFiles) }, files, nil)
	heavy(CondMaxLines, r.MaxLines > 0, func() bool { return atMost(linesOf(pr), r.MaxLines) }, lines, nil)
	heavy(CondMinFiles, r.MinFiles > 0, func() bool { return atLeast(pr.ChangedFiles, r.MinFiles) }, files, nil)
	heavy(CondMinLines, r.MinLines > 0, func() bool { return atLeast(linesOf(pr), r.MinLines) }, lines, nil)
	heavy(CondDecisions, len(r.Decisions) > 0, func() bool { return r.decisionHolds(pr) },
		func() []string { return []string{string(decisionOf(pr))} },
		func() []string {
			if r.decisionHolds(pr) {
				return []string{string(decisionOf(pr))}
			}
			return nil
		})
	heavy(CondLabels, len(r.Labels) > 0, func() bool { return slices.ContainsFunc(r.Labels, func(l string) bool { return hasLabel(l, pr) }) },
		func() []string {
			var have []string
			for _, l := range pr.Labels {
				if slices.ContainsFunc(r.Labels, func(w string) bool { return strings.EqualFold(w, l) }) {
					have = append(have, l)
				}
			}
			if len(have) == 0 {
				return pr.Labels
			}
			return have
		},
		func() []string {
			var m []string
			for _, l := range r.Labels {
				if hasLabel(l, pr) {
					m = append(m, l)
				}
			}
			return m
		})
	if len(r.Checks) > 0 {
		switch {
		case !pr.HeavyRead():
			out = append(out, Explained{Cond: CondChecks, Verdict: Pending, Need: NeedHeavy})
		case r.approvalUnknown(pr, f):
			out = append(out, Explained{Cond: CondChecks, Verdict: Pending, Need: NeedApproval, Values: []string{string(pr.Checks)}})
		default:
			pick(CondChecks, words(r.Checks), []string{string(pr.Checks)}, func(c string) bool { return checksAre(Checks(c), pr, f) })
		}
	}
	if len(r.Packages) > 0 {
		if !f.BodyKnown {
			out = append(out, Explained{Cond: CondPackages, Verdict: Pending, Need: NeedBody})
		} else {
			var bumped []string
			for _, b := range Bumps(f.Body) {
				bumped = append(bumped, b.Package)
			}
			var m []string
			if r.packages(f) {
				for _, p := range r.Packages {
					if slices.ContainsFunc(bumped, func(b string) bool { return strings.EqualFold(b, p) }) {
						m = append(m, p)
					}
				}
			}
			add(CondPackages, r.packages(f), bumped, m)
		}
	}
	return out
}

// words are the values of a list as strings.
func words[T ~string](vs []T) []string {
	out := make([]string, len(vs))
	for i, v := range vs {
		out[i] = string(v)
	}
	return out
}

// nonEmpty is s alone, or nothing for "".
func nonEmpty(s string) []string {
	if s == "" {
		return nil
	}
	return []string{s}
}

// daysSince is the whole days of 24 hours from t to now, as a word; none
// for a time not read.
func daysSince(t, now time.Time) []string {
	if t.IsZero() {
		return nil
	}
	return []string{strconv.Itoa(int(now.Sub(t) / (24 * time.Hour)))}
}
