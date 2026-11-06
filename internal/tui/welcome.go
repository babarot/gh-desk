package tui

import (
	"strconv"
	"strings"
	"time"

	"github.com/babarot/gh-desk/internal/model"
)

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
	f, _ := m.clawdNow(c)
	w := clawdW
	if c.scene != nil {
		w = c.scene.width()
	}
	clawd := st.clawdLines(f, w)
	above, art := clawd[0], clawd[1:]
	beside := spaces(w + welcomeGap)
	var right []string
	right = append(right, st.title.Render(p.Key.String())+"  "+st.dim.Render(shownTitle(p.Title)))
	who := st.dim.Render(p.Author+" wants to merge ") + st.muted.Render(p.HeadRef) + st.dim.Render(" into ") + st.muted.Render(p.BaseRef)
	if p.Draft {
		who += st.dim.Render(" · draft")
	}
	right = append(right, who, m.welcomeFacts(p))
	var head []string
	for i, a := range art {
		head = append(head, margin+a+spaces(welcomeGap)+right[i])
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
	// pane's top edge; what Clawd holds up is drawn in it.
	top := ""
	if strings.TrimSpace(above) != "" {
		top = margin + above
	}
	for _, L := range [][]string{
		append(append(append([]string{top}, head...), worktree...), asks...),
		append(append([]string{top}, head...), worktree...),
		append([]string{top}, head...),
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

// clawdNow is the frame Clawd is drawn in for c now, and whether it is
// moving. It plays c's scene from when the worktree began to be made;
// with no scene, or with the spinner chosen still, it stands still, in
// the frame its scene ends in once the worktree's end came.
func (m *Model) clawdNow(c *chatScreen) (clawdFrame, bool) {
	s := c.scene
	if s == nil {
		return stand(pDefault), false
	}
	ended := time.Duration(-1)
	if !c.endedAt.IsZero() {
		ended = c.endedAt.Sub(c.sceneAt)
	}
	ok := c.prepareErr == ""
	if m.spinStyle.still {
		switch {
		case ended < 0:
			return stand(pDefault), false
		case ok:
			return s.readyRest, false
		}
		return s.failRest, false
	}
	return s.at(m.set.Now().Sub(c.sceneAt), ended, ok)
}

// clawdMoving reports whether Clawd moves in the claude pane shown.
func (m *Model) clawdMoving() bool {
	c := m.chat()
	if !m.claudeShown() || c == nil || busy(c) || c.Started() {
		return false
	}
	_, moving := m.clawdNow(c)
	return moving
}

// pickScene picks one of Clawd's themes for a worktree being made, not
// the one picked last, and its parts.
func (m *Model) pickScene() *clawdScene {
	ts := clawdThemes()
	i := m.set.Pick(len(ts))
	if m.lastTheme >= 0 && len(ts) > 1 {
		i = m.set.Pick(len(ts) - 1)
		if i >= m.lastTheme {
			i++
		}
	}
	m.lastTheme = i
	return newScene(ts[i], m.set.Pick)
}
