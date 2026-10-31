// Package plan decides things about pull requests without asking anyone:
// which group a review request falls in, whether its checks pass, and what
// a dependency bot's pull request updates. Everything here is a function of
// its arguments, so it is tested without GitHub.
package plan

import (
	"path"
	"regexp"
	"strings"

	"github.com/babarot/gh-triage/internal/model"
)

// Group is where a pull request is listed in a grouped tab. A group says
// only what holds of the pull request itself, never what the tab's query
// implies, so it is true whatever the query is. See ADR-015.
type Group string

const (
	// GroupYou is a pull request whose review is requested from the viewer
	// in person.
	GroupYou Group = "you"
	// GroupTeam is a pull request whose review is requested from a team,
	// whether or not the viewer is on it.
	GroupTeam Group = "team"
	// GroupOther is a pull request with neither request.
	GroupOther Group = "other"
	// GroupBots is a pull request opened by a bot.
	GroupBots Group = "bots"
)

// Groups are the groups in the order a grouped tab lists them.
var Groups = []Group{GroupYou, GroupTeam, GroupOther, GroupBots}

// GroupOf is the group p falls in for viewer: the first of bots, you, team
// and other that holds. Groups do not overlap, so a bot's pull request is
// in bots even when it is requested from the viewer.
func GroupOf(p model.PR, viewer string) Group {
	switch {
	case p.AuthorIsBot:
		return GroupBots
	case p.RequestedFrom(viewer):
		return GroupYou
	case p.RequestedFromTeam():
		return GroupTeam
	default:
		return GroupOther
	}
}

// ChecksPass reports whether p's checks have passed: every check succeeded,
// there are none, or only checks that wait for an approval are pending.
// awaitingApproval says whether the pending rollup is only such checks; the
// list does not read single checks, so the caller tells it from the
// repository being one whose checks include them.
func ChecksPass(p model.PR, awaitingApproval bool) bool {
	switch p.Checks {
	case model.CheckSuccess, "":
		return true
	case model.CheckPending, model.CheckExpected:
		return awaitingApproval
	}
	return false
}

// MatchAny reports whether name matches any of the glob patterns, as
// path.Match reads them; a pattern without a slash matches the base name.
func MatchAny(patterns []string, name string) bool {
	for _, pat := range patterns {
		target := name
		if !strings.Contains(pat, "/") {
			target = path.Base(name)
		}
		if ok, _ := path.Match(pat, target); ok {
			return true
		}
	}
	return false
}

// Bump is one dependency a bot's pull request updates.
type Bump struct {
	Package string
	// Update is renovate's kind of update (major, minor, patch, digest...),
	// empty when the body does not say.
	Update string
	// From and To are the versions; To may be empty when the body gives
	// only a change it does not split.
	From, To string
	// Pending is a newer version renovate holds back, if any.
	Pending string
}

var (
	mdLink       = regexp.MustCompile(`\[([^\]]*)\]\([^)]*\)`)
	arrow        = regexp.MustCompile(`^(.*?)\s*(?:→|->)\s*(.*)$`)
	dependabotRe = regexp.MustCompile(`Bumps \[([^\]]+)\]\([^)]*\) from ([^\s]+) to ([^\s]+?)\.?(?:\s|$)`)
)

// Bumps reads what a dependency bot's pull request updates from its body:
// renovate's table (| Package | Update | Change | Pending |) or
// dependabot's "Bumps [x](...) from a to b". None when the body has
// neither.
func Bumps(body string) []Bump {
	if bs := renovateTable(body); len(bs) > 0 {
		return bs
	}
	if m := dependabotRe.FindStringSubmatch(body); m != nil {
		return []Bump{{Package: m[1], From: m[2], To: m[3]}}
	}
	return nil
}

func renovateTable(body string) []Bump {
	var head []string
	var out []Bump
	for line := range strings.SplitSeq(body, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "|") {
			if head != nil && len(out) > 0 {
				break
			}
			head = nil
			continue
		}
		cells := cells(line)
		switch {
		case head == nil:
			if index(cells, "Package") >= 0 {
				head = cells
			}
		case strings.Trim(strings.Join(cells, ""), "-: ") == "":
			// the line under the header
		default:
			get := func(name string) string {
				if i := index(head, name); i >= 0 && i < len(cells) {
					return clean(cells[i])
				}
				return ""
			}
			b := Bump{Package: get("Package"), Update: get("Update"), Pending: get("Pending")}
			if b.Package == "" {
				continue
			}
			change := get("Change")
			if m := arrow.FindStringSubmatch(change); m != nil {
				b.From, b.To = m[1], m[2]
			} else {
				b.To = change
			}
			out = append(out, b)
		}
	}
	return out
}

func cells(line string) []string {
	parts := strings.Split(strings.Trim(line, "|"), "|")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return parts
}

func index(cells []string, name string) int {
	for i, c := range cells {
		if strings.EqualFold(c, name) {
			return i
		}
	}
	return -1
}

// clean drops a cell's markdown: links keep their text, code marks go.
func clean(s string) string {
	s = mdLink.ReplaceAllString(s, "$1")
	return strings.TrimSpace(strings.ReplaceAll(s, "`", ""))
}
