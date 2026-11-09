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
// hold. S selects the pull requests a rule matches (Match). A condition
// says "not" by its values, never by an operator: a list asks for one of
// the values in it, and a flag set to false asks for its opposite. A
// condition left out (nil, empty) does not ask.
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
	// Bot asks for a bot's pull request, or with false a person's.
	Bot *bool
	// Mine asks for the viewer's own pull request, or with false one of
	// someone else.
	Mine *bool
	// Draft asks for a draft, or with false one ready for review.
	Draft *bool
	// States are the states it must be in one of; a state not read is
	// open, as the list reads only open pull requests unless asked.
	States []model.PRState
	// Checks are the states its checks must be in one of.
	Checks []Checks
	// Decisions are the review decisions it must have one of; NoDecision
	// for none.
	Decisions []model.ReviewDecision
	// Labels are labels one of which it must have.
	Labels []string
	// ApprovedByMe asks for the viewer's latest review to approve it, or
	// with false for none that does.
	ApprovedByMe *bool
	// ReviewedByMe asks for a latest review of the viewer's, of any state,
	// or with false for none.
	ReviewedByMe *bool
	// Requested are whom a review must be asked of, one of them: "me" for
	// the viewer, or a team as "@slug".
	Requested []string
	// Reasons are the reasons of the row's notification, one of which it
	// must have (Facts.Reason); a row with no notification has none.
	Reasons []string
}

// NoDecision is the review decision of a pull request that has none, as
// in a repository that requires no review.
const NoDecision model.ReviewDecision = "NONE"

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
var ApprovedByMe = Rule{Name: "approved by me", Bot: new(true), ApprovedByMe: new(true)}

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
	return r.MaxFiles > 0 || r.MaxLines > 0 || r.MinFiles > 0 || r.MinLines > 0 || len(r.Checks) > 0 || len(r.Decisions) > 0 || len(r.Labels) > 0
}

// Match reports whether S selects pr by the rule for viewer: an open pull
// request, not a draft, that the rule says Yes of. S acts on what it
// selects, so a pull request the rule cannot tell yet is not selected.
func (r Rule) Match(pr model.PR, viewer string, f Facts) bool {
	return Selectable(pr) && r.Judge(pr, viewer, f) == Yes
}

// Selectable reports whether S can select pr at all, whatever the rule.
func Selectable(pr model.PR) bool {
	return !pr.Draft && (pr.State == model.StateOpen || pr.State == "")
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
	if r.approvalUnknown(pr, f) {
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
	case len(r.Repos) > 0 && !slices.ContainsFunc(r.Repos, func(g string) bool { return repoMatches(g, pr) }):
		return false
	case len(r.Authors) > 0 && !slices.ContainsFunc(r.Authors, func(a string) bool { return authorIs(a, pr) }):
		return false
	case !holds(r.Bot, pr.AuthorIsBot):
		return false
	case !holds(r.Draft, pr.Draft):
		return false
	// What is the viewer's is not told before the viewer is known, either
	// way.
	case (r.Mine != nil || r.ApprovedByMe != nil || r.ReviewedByMe != nil) && viewer == "":
		return false
	case !holds(r.Mine, pr.Author == viewer):
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
	case len(r.Reasons) > 0 && !slices.ContainsFunc(r.Reasons, func(s string) bool { return reasonIs(s, f) }):
		return false
	case len(r.Requested) > 0 && !slices.ContainsFunc(r.Requested, func(s string) bool { return requested(pr, viewer, s) }):
		return false
	}
	rv, reviewed := pr.ReviewBy(viewer)
	return holds(r.ApprovedByMe, reviewed && rv.State == model.ReviewApproved) && holds(r.ReviewedByMe, reviewed)
}

// approvalUnknown reports whether the checks condition waits to know if
// pr's pending checks only wait for an approval: that changes passing and
// pending, not failing.
func (r Rule) approvalUnknown(pr model.PR, f Facts) bool {
	return (slices.Contains(r.Checks, ChecksPassing) || slices.Contains(r.Checks, ChecksPending)) &&
		(pr.Checks == model.CheckPending || pr.Checks == model.CheckExpected) && !f.ApprovalKnown
}

// holds reports whether a flag asking for want holds of a pull request
// that is or is not so: a flag not set always does.
func holds(want *bool, is bool) bool { return want == nil || *want == is }

// heavy reports whether the conditions on the heavy fields hold; they are
// read for the head shown.
func (r Rule) heavy(pr model.PR, f Facts) bool {
	switch {
	case r.MaxFiles > 0 && !atMost(pr.ChangedFiles, r.MaxFiles):
		return false
	case r.MaxLines > 0 && !atMost(linesOf(pr), r.MaxLines):
		return false
	case r.MinFiles > 0 && !atLeast(pr.ChangedFiles, r.MinFiles):
		return false
	case r.MinLines > 0 && !atLeast(linesOf(pr), r.MinLines):
		return false
	case len(r.Decisions) > 0 && !r.decisionHolds(pr):
		return false
	case len(r.Labels) > 0 && !slices.ContainsFunc(r.Labels, func(l string) bool { return hasLabel(l, pr) }):
		return false
	}
	return len(r.Checks) == 0 || slices.ContainsFunc(r.Checks, func(c Checks) bool { return checksAre(c, pr, f) })
}

// atMost and atLeast compare a count with a rule's cap and floor.
func atMost(v, n int) bool  { return v <= n }
func atLeast(v, n int) bool { return v >= n }

// linesOf are the lines pr adds and deletes together.
func linesOf(pr model.PR) int { return pr.Additions + pr.Deletions }

// decisionHolds reports whether pr's review decision is one the rule asks.
func (r Rule) decisionHolds(pr model.PR) bool { return slices.Contains(r.Decisions, decisionOf(pr)) }

// checksAre reports whether pr's checks are in the state c.
func checksAre(c Checks, pr model.PR, f Facts) bool {
	switch c {
	case ChecksPassing:
		return ChecksPass(pr, f.AwaitingApproval)
	case ChecksFailing:
		return pr.Checks == model.CheckFailure || pr.Checks == model.CheckError
	case ChecksPending:
		return (pr.Checks == model.CheckPending || pr.Checks == model.CheckExpected) && !f.AwaitingApproval
	}
	return false
}

// decisionOf is pr's review decision, NoDecision when it has none.
func decisionOf(pr model.PR) model.ReviewDecision {
	if pr.Decision == "" {
		return NoDecision
	}
	return pr.Decision
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

// repoMatches reports whether pr's repository matches the owner/repo glob g,
// ignoring case.
func repoMatches(g string, pr model.PR) bool {
	ok, _ := path.Match(strings.ToLower(g), strings.ToLower(pr.Key.Repository()))
	return ok
}

// authorIs reports whether pr's author is a, ignoring case and app/.
func authorIs(a string, pr model.PR) bool {
	return strings.EqualFold(strings.TrimPrefix(pr.Author, "app/"), strings.TrimPrefix(a, "app/"))
}

// reasonIs reports whether the reason of the row's notification is s; a
// row with no notification has none.
func reasonIs(s string, f Facts) bool { return f.Reason != "" && strings.EqualFold(s, f.Reason) }

// hasLabel reports whether pr has the label l, ignoring case.
func hasLabel(l string, pr model.PR) bool {
	return slices.ContainsFunc(pr.Labels, func(have string) bool { return strings.EqualFold(have, l) })
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
