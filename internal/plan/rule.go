package plan

import (
	"path"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/babarot/gh-desk/internal/model"
)

// Rule is a set of conditions on a pull request, every one of which must
// hold. S selects the pull requests a rule matches (Match).
type Rule struct {
	Name string
	// Repos are owner/repo globs, as path.Match reads them, the
	// repository must match one of; * does not cross the slash.
	Repos []string
	// Authors are logins the author's must be one of, without app/ for a
	// bot.
	Authors []string
	// Packages are dependencies a bot's pull request must update; one of
	// them is enough, and every one it updates must be among them.
	Packages []string
	Title    *regexp.Regexp
	// MaxFiles and MaxLines cap the files changed and the lines added and
	// deleted together; MinFiles and MinLines are the least. 0 does not.
	MaxFiles int
	MaxLines int
	MinFiles int
	MinLines int
	// UpdatedWithinDays and CreatedWithinDays ask for a pull request last
	// updated, or created, at most so many days (of 24 hours) before
	// Facts.Now; UpdatedBeforeDays and CreatedBeforeDays at least so many.
	// 0 does not.
	UpdatedWithinDays int
	CreatedWithinDays int
	UpdatedBeforeDays int
	CreatedBeforeDays int
	// Bot requires a bot's pull request.
	Bot bool
	// Mine requires the viewer's own pull request.
	Mine bool
	// Draft requires a draft.
	Draft bool
	// States are the states it must be in one of; a state not read is
	// open, as the list reads only open pull requests unless asked.
	States []model.PRState
	// Checks is the state its checks must be in; "" does not ask.
	Checks Checks
	// Decision is the review decision it must have; "" does not ask.
	Decision model.ReviewDecision
	// Labels are labels one of which it must have.
	Labels []string
	// ApprovedByMe requires the viewer's latest review to approve it.
	ApprovedByMe bool
	// ReviewedByMe requires a latest review of the viewer's, of any state.
	ReviewedByMe bool
	// Requested are whom a review must be asked of, one of them: "me" for
	// the viewer, or a team as "@slug".
	Requested []string
	// Reasons are the reasons of the row's notification, one of which it
	// must have (Facts.Reason); a row with no notification has none.
	Reasons []string
}

// Checks is a state of a pull request's checks a rule asks for.
type Checks string

const (
	// ChecksPassing are checks that passed, none at all, or only checks
	// that wait for an approval still pending (ChecksPass).
	ChecksPassing Checks = "passing"
	// ChecksFailing are checks that failed or erred.
	ChecksFailing Checks = "failing"
	// ChecksPending are checks still running, not only waiting for an
	// approval.
	ChecksPending Checks = "pending"
)

// ApprovedByMe is the rule S always offers first: bot pull requests the
// viewer has approved and that are still open. A person's pull request is
// left to its author to merge.
var ApprovedByMe = Rule{Name: "approved by me", Bot: true, ApprovedByMe: true}

// Facts are what a rule may need beyond the pull request: its body (for
// the packages it updates), whether only approval checks are pending, and
// the reason of the row's notification.
type Facts struct {
	Body      string
	BodyKnown bool
	// AwaitingApproval says whether the pending checks are only checks
	// that wait for an approval; ApprovalKnown whether that is known yet.
	// It is known at once when no check waits for an approval.
	AwaitingApproval bool
	ApprovalKnown    bool
	// Reason is the reason of the row's notification, "" for none.
	Reason string
	// Now is the time the days count back from, the same for every pull
	// request matched at once.
	Now time.Time
}

// Verdict is what a rule says of a pull request.
type Verdict int

const (
	// No: a condition does not hold.
	No Verdict = iota
	// Yes: every condition holds.
	Yes
	// Pending: the conditions that can be told hold, and the rest wait for
	// what is not read yet: the heavy fields for the head shown, the body,
	// or whether the pending checks only wait for an approval.
	Pending
)

// NeedsBody reports whether matching the rule reads pull requests' bodies.
func (r Rule) NeedsBody() bool { return len(r.Packages) > 0 }

// NeedsHeavy reports whether matching the rule reads the heavy fields (the
// size, the checks, the review decision and the labels). Unread, they look
// like an empty pull request with no checks and no labels, which would
// pass, so a pull request whose heavy fields are not read for its head is
// never judged on them.
func (r Rule) NeedsHeavy() bool {
	return r.MaxFiles > 0 || r.MaxLines > 0 || r.MinFiles > 0 || r.MinLines > 0 || r.Checks != "" || r.Decision != "" || len(r.Labels) > 0
}

// Match reports whether S selects pr by the rule for viewer: an open pull
// request, not a draft, that the rule says Yes of. S acts on what it
// selects, so a pull request the rule cannot tell yet is not selected.
func (r Rule) Match(pr model.PR, viewer string, f Facts) bool {
	if pr.Draft || pr.State != model.StateOpen && pr.State != "" {
		return false
	}
	return r.Judge(pr, viewer, f) == Yes
}

// Judge says whether pr meets every condition of the rule for viewer. The
// conditions on what is read with the list come first: one that does not
// hold says No at once, without waiting for what is read later. It looks
// at no condition beyond the rule's, so a draft or a merged pull request is
// judged as any other.
func (r Rule) Judge(pr model.PR, viewer string, f Facts) Verdict {
	if !r.light(pr, viewer, f) {
		return No
	}
	if r.NeedsHeavy() && !pr.HeavyRead() || r.NeedsBody() && !f.BodyKnown {
		return Pending
	}
	// Whether pending checks only wait for an approval changes passing and
	// pending, not failing.
	if (r.Checks == ChecksPassing || r.Checks == ChecksPending) && (pr.Checks == model.CheckPending || pr.Checks == model.CheckExpected) && !f.ApprovalKnown {
		return Pending
	}
	if !r.heavy(pr, f) || !r.packages(f) {
		return No
	}
	return Yes
}

// light reports whether the conditions on what the list reads at once
// hold.
func (r Rule) light(pr model.PR, viewer string, f Facts) bool {
	switch {
	case len(r.Repos) > 0 && !slices.ContainsFunc(r.Repos, func(g string) bool {
		ok, _ := path.Match(strings.ToLower(g), strings.ToLower(pr.Key.Repository()))
		return ok
	}):
		return false
	case len(r.Authors) > 0 && !slices.ContainsFunc(r.Authors, func(a string) bool {
		return strings.EqualFold(strings.TrimPrefix(pr.Author, "app/"), strings.TrimPrefix(a, "app/"))
	}):
		return false
	case r.Bot && !pr.AuthorIsBot:
		return false
	case r.Mine && (viewer == "" || pr.Author != viewer):
		return false
	case r.Draft && !pr.Draft:
		return false
	case len(r.States) > 0 && !slices.Contains(r.States, stateOf(pr)):
		return false
	case r.Title != nil && !r.Title.MatchString(pr.Title):
		return false
	case r.UpdatedWithinDays > 0 && !within(pr.Updated, f.Now, r.UpdatedWithinDays):
		return false
	case r.CreatedWithinDays > 0 && !within(pr.Created, f.Now, r.CreatedWithinDays):
		return false
	case r.UpdatedBeforeDays > 0 && !before(pr.Updated, f.Now, r.UpdatedBeforeDays):
		return false
	case r.CreatedBeforeDays > 0 && !before(pr.Created, f.Now, r.CreatedBeforeDays):
		return false
	case len(r.Reasons) > 0 && !slices.ContainsFunc(r.Reasons, func(s string) bool { return f.Reason != "" && strings.EqualFold(s, f.Reason) }):
		return false
	case len(r.Requested) > 0 && !slices.ContainsFunc(r.Requested, func(s string) bool { return requested(pr, viewer, s) }):
		return false
	}
	if r.ApprovedByMe {
		rv, ok := pr.ReviewBy(viewer)
		if !ok || rv.State != model.ReviewApproved {
			return false
		}
	}
	if r.ReviewedByMe {
		if _, ok := pr.ReviewBy(viewer); !ok {
			return false
		}
	}
	return true
}

// heavy reports whether the conditions on the heavy fields hold; they are
// read for the head shown.
func (r Rule) heavy(pr model.PR, f Facts) bool {
	switch {
	case r.MaxFiles > 0 && pr.ChangedFiles > r.MaxFiles:
		return false
	case r.MaxLines > 0 && pr.Additions+pr.Deletions > r.MaxLines:
		return false
	case r.MinFiles > 0 && pr.ChangedFiles < r.MinFiles:
		return false
	case r.MinLines > 0 && pr.Additions+pr.Deletions < r.MinLines:
		return false
	case r.Decision != "" && pr.Decision != r.Decision:
		return false
	case len(r.Labels) > 0 && !slices.ContainsFunc(r.Labels, func(l string) bool {
		return slices.ContainsFunc(pr.Labels, func(have string) bool { return strings.EqualFold(have, l) })
	}):
		return false
	}
	switch r.Checks {
	case ChecksPassing:
		return ChecksPass(pr, f.AwaitingApproval)
	case ChecksFailing:
		return pr.Checks == model.CheckFailure || pr.Checks == model.CheckError
	case ChecksPending:
		return (pr.Checks == model.CheckPending || pr.Checks == model.CheckExpected) && !f.AwaitingApproval
	}
	return true
}

// packages reports whether every dependency the body says the pull request
// updates is among the rule's packages.
func (r Rule) packages(f Facts) bool {
	if len(r.Packages) == 0 {
		return true
	}
	bs := Bumps(f.Body)
	if len(bs) == 0 {
		return false
	}
	for _, b := range bs {
		if !slices.ContainsFunc(r.Packages, func(p string) bool { return strings.EqualFold(p, b.Package) }) {
			return false
		}
	}
	return true
}

// stateOf is pr's state, open when it is not read.
func stateOf(pr model.PR) model.PRState {
	if pr.State == "" {
		return model.StateOpen
	}
	return pr.State
}

// requested reports whether a review of pr is asked of who: "me" for the
// viewer, or "@slug" for a team.
func requested(pr model.PR, viewer, who string) bool {
	if team, ok := strings.CutPrefix(who, "@"); ok {
		return slices.ContainsFunc(pr.Requested, func(rv model.Reviewer) bool { return rv.Team && strings.EqualFold(rv.Login, team) })
	}
	return who == "me" && viewer != "" && pr.RequestedFrom(viewer)
}

// within reports whether t is at most days days of 24 hours before now. A
// time not read is not within; one after now, from a clock ahead, is.
func within(t, now time.Time, days int) bool {
	if t.IsZero() {
		return false
	}
	return now.Sub(t) <= time.Duration(days)*24*time.Hour
}

// before reports whether t is at least days days of 24 hours before now. A
// time not read is not.
func before(t, now time.Time, days int) bool {
	if t.IsZero() {
		return false
	}
	return now.Sub(t) >= time.Duration(days)*24*time.Hour
}
