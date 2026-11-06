package tui

import (
	"strconv"
	"strings"
	"time"

	"github.com/babarot/gh-desk/internal/model"
)

// mascot is Clawd, Claude's mascot, drawn as Claude Code draws it: three lines,
// each as wide as the others.
const mascot = " ▐▛███▜▌ \n▝▜█████▛▘\n  ▘▘ ▝▝  "

// mascotPoses are what Clawd does while the worktree is made, one pose
// after another: it waves and blinks. Each pose is drawn as mascot is,
// with lines as wide as mascot's, so the text beside it stays put.
var mascotPoses = []string{
	mascot,
	mascot,
	" ▐▛███▜▌ \n▗▜█████▛▖\n  ▘▘ ▝▝  ",
	mascot,
	" ▐▛███▜▌ \n▗▜█████▛▖\n  ▘▘ ▝▝  ",
	mascot,
	" ▐█████▌ \n▝▜█████▛▘\n  ▘▘ ▝▝  ",
	mascot,
}

// mascotEvery is how long Clawd keeps a pose. It is longer than any
// moving spinner's step, so the screen, drawn at the spinner's pace while
// the worktree is made, never skips a pose.
const mascotEvery = 250 * time.Millisecond

// mascotNow is Clawd as it is drawn now: moving while c's worktree is
// made, still otherwise and when the spinner chosen does not move.
func (m *Model) mascotNow(c *chatScreen) string {
	if !c.preparing || m.spinStyle.still {
		return mascot
	}
	return mascotPoses[int(m.set.Now().UnixMilli()/mascotEvery.Milliseconds())%len(mascotPoses)]
}

// welcomeGap is the space between Clawd and the text beside it.
const welcomeGap = 3

// welcomeLines are what the claude pane shows of c while nothing has been
// asked, in at most height lines: Clawd beside the pull request,
// the worktree being made, and the questions fill_suggestion puts in the
// text box. The questions give way first, then the worktree; when even the
// pull request does not fit, nothing is shown.
func (m *Model) welcomeLines(c *chatScreen, height int) []string {
	st, p := m.st, c.PR()
	margin := spaces(chatMargin)
	art := strings.Split(m.mascotNow(c), "\n")
	beside := spaces(len([]rune(art[0])) + welcomeGap)
	var right []string
	right = append(right, st.title.Render(p.Key.String())+"  "+st.dim.Render(shownTitle(p.Title)))
	who := st.dim.Render(p.Author+" wants to merge ") + st.muted.Render(p.HeadRef) + st.dim.Render(" into ") + st.muted.Render(p.BaseRef)
	if p.Draft {
		who += st.dim.Render(" · draft")
	}
	right = append(right, who, m.welcomeFacts(p))
	var head []string
	for i, a := range art {
		head = append(head, margin+st.mascot.Render(a)+spaces(welcomeGap)+right[i])
	}
	var worktree []string
	switch {
	case c.preparing:
		worktree = []string{margin + beside + m.working(st.muted, "making the worktree…")}
	case c.Dir() != "":
		worktree = []string{margin + beside + st.green.Render("✓") + st.dim.Render(" worktree ready at "+short(p.HeadOID))}
	case c.prepareErr != "":
		// The first question makes it again, and says again what went
		// wrong if it fails.
		worktree = []string{margin + beside + st.red.Render("✗ worktree not made: "+safe(firstLine(c.prepareErr))) + st.dim.Render(" · the first question tries again")}
	}
	var asks []string
	if ss := suggestions(c); len(ss) > 0 {
		asks = append(asks, "", margin+st.muted.Render("try"))
		for _, s := range ss {
			if s == c.input.Value() {
				asks = append(asks, margin+st.mauve.Render("›")+" "+st.sub1.Render(s))
			} else {
				asks = append(asks, margin+"  "+st.dim.Render(s))
			}
		}
	}
	// A line is left above, as a conversation leaves one under the
	// pane's top edge.
	for _, L := range [][]string{
		append(append(append([]string{""}, head...), worktree...), asks...),
		append(append([]string{""}, head...), worktree...),
		append([]string{""}, head...),
	} {
		if len(L) <= height {
			return L
		}
	}
	return nil
}

// welcomeFacts is the line of the welcome that says how big the pull
// request is, how its checks and reviews stand, and how old it is, with
// what has been read of them.
func (m *Model) welcomeFacts(p model.PR) string {
	st := m.st
	sep := st.dim.Render(" · ")
	var parts []string
	if p.HeavyRead() {
		parts = append(parts,
			st.add.Render("+"+strconv.Itoa(p.Additions))+" "+st.del.Render("−"+strconv.Itoa(p.Deletions))+st.dim.Render(" in "+files(p.ChangedFiles)),
			st.dim.Render("CI ")+m.ciMark(p.Checks)+" "+st.dim.Render(checkWord(p.Checks)))
		if w := decisionWord(p.Decision); w != "" {
			parts = append(parts, m.decisionMark(p.Decision)+" "+st.dim.Render(w))
		}
	}
	if !p.Updated.IsZero() {
		parts = append(parts, st.dim.Render("updated "+shortAge(m.set.Now().Sub(p.Updated))+" ago"))
	}
	return strings.Join(parts, sep)
}

func files(n int) string {
	if n == 1 {
		return "1 file"
	}
	return strconv.Itoa(n) + " files"
}

func checkWord(c model.CheckState) string {
	switch c {
	case model.CheckSuccess:
		return "passing"
	case model.CheckFailure, model.CheckError:
		return "failing"
	case model.CheckPending, model.CheckExpected:
		return "running"
	}
	return "none"
}

func decisionWord(d model.ReviewDecision) string {
	switch d {
	case model.DecisionApproved:
		return "approved"
	case model.DecisionChangesRequested:
		return "changes requested"
	case model.DecisionReviewRequired:
		return "review required"
	}
	return ""
}
