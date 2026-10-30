package plan

import (
	"regexp"
	"slices"
	"strings"

	"github.com/babarot/gh-triage/internal/model"
)

// Rule picks pull requests for S: a pull request matches when it meets
// every condition the rule sets.
type Rule struct {
	Name string
	// Author is the author's login, without app/ for a bot.
	Author string
	// Packages are dependencies a bot's pull request must update; one of
	// them is enough, and every one it updates must be among them.
	Packages []string
	Title    *regexp.Regexp
	// MaxFiles caps the files changed; 0 does not.
	MaxFiles int
	// Bot requires a bot's pull request.
	Bot bool
	// ChecksPassing requires its checks to pass, an approval check
	// pending counting as passing.
	ChecksPassing bool
	// ApprovedByMe requires the viewer's latest review to approve it.
	ApprovedByMe bool
}

// ApprovedByMe is the rule S always offers first: bot pull requests the
// viewer has approved and that are still open. A person's pull request is
// left to its author to merge.
var ApprovedByMe = Rule{Name: "approved by me", Bot: true, ApprovedByMe: true}

// Facts are what a rule may need beyond the pull request: its body (for
// the packages it updates) and whether only approval checks are pending.
type Facts struct {
	Body             string
	BodyKnown        bool
	AwaitingApproval bool
}

// NeedsBody reports whether matching the rule reads pull requests' bodies.
func (r Rule) NeedsBody() bool { return len(r.Packages) > 0 }

// Match reports whether pr matches the rule for viewer.
func (r Rule) Match(pr model.PR, viewer string, f Facts) bool {
	switch {
	case pr.Draft || pr.State != model.StateOpen && pr.State != "":
		return false
	case r.Author != "" && !strings.EqualFold(strings.TrimPrefix(pr.Author, "app/"), strings.TrimPrefix(r.Author, "app/")):
		return false
	case r.Bot && !pr.AuthorIsBot:
		return false
	case r.Title != nil && !r.Title.MatchString(pr.Title):
		return false
	case r.MaxFiles > 0 && pr.ChangedFiles > r.MaxFiles:
		return false
	case r.ChecksPassing && !ChecksPass(pr, f.AwaitingApproval):
		return false
	}
	if r.ApprovedByMe {
		rv, ok := pr.ReviewBy(viewer)
		if !ok || rv.State != model.ReviewApproved {
			return false
		}
	}
	if len(r.Packages) > 0 {
		if !f.BodyKnown {
			return false
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
	}
	return true
}
