package tui

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/babarot/gh-desk/internal/model"
	"github.com/babarot/gh-desk/internal/runner"
)

// kinds are the kinds of review, in the order the review popup cycles
// through them.
var kinds = []model.ReviewEvent{model.EventComment, model.EventApprove, model.EventRequestChanges}

var kindWords = map[model.ReviewEvent]string{
	model.EventComment:        "comment",
	model.EventApprove:        "approve",
	model.EventRequestChanges: "request changes",
}

// compose is a comment or a review being written, in a popup over the
// screen it was opened from. Either goes to every one of prs, which
// starts with pr.
type compose struct {
	pr model.PR
	// single is a comment on the conversation, not a review: it has no
	// kind, and leaves a review request from the viewer as it is.
	single bool
	prs    []model.PR
	kind   int
	// own is a review of the viewer's own pull request alone, which can
	// only be a comment. Among many, the confirm popup skips one's own.
	own bool
	ta  textarea.Model
}

// composeWidth is the width of the popup on a screen w wide.
func composeWidth(w int) int { return min(max(w-8, 40), 100) }

// resize fits the text area in the popup on a screen w by h: the popup
// takes up to seven lines besides it, more when the keys of its pull
// requests take more than a line, and leaves the status and key lines under
// it in sight. A longer review is written in $EDITOR.
func (c *compose) resize(w, h int) {
	c.ta.SetWidth(composeWidth(w) - 4)
	c.ta.SetHeight(min(max(h-12-max(c.headHeight(composeWidth(w)-4)-3, 0), 3), 10))
}

// composeKeysLines is how many lines the keys of a comment's pull
// requests take at most.
const composeKeysLines = 3

// headHeight is how many lines are above the text area in a popup inner
// cells wide: the pull requests and the kinds of a review, and a blank.
func (c *compose) headHeight(inner int) int {
	n := 2
	if len(c.prs) > 1 {
		n = len(keysLines(lipgloss.NewStyle(), c.prs, inner, composeKeysLines)) + 1
	}
	if !c.single {
		n++
	}
	return n
}

func (c *compose) update(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	c.ta, cmd = c.ta.Update(msg)
	return cmd
}

// sentWord names what is written: a comment, or a review of its kind.
func sentWord(single bool, ev model.ReviewEvent) string {
	switch {
	case single:
		return "comment"
	case ev == model.EventComment:
		return "review comment"
	}
	return kindWords[ev]
}

func (m *Model) own(p model.PR) bool { return m.viewer != "" && p.Author == m.viewer }

// startComment opens the popup for a comment on the conversation of each
// selected pull request, or of the one under the cursor.
func (m *Model) startComment() tea.Cmd {
	prs := m.menuTargets()
	if len(prs) == 0 {
		return nil
	}
	c := &compose{pr: prs[0], prs: prs, single: true, ta: newInput(m.st)}
	c.ta.Placeholder = "Write the comment (markdown)"
	return m.openCompose(c)
}

// startReview opens the popup for a review of each selected pull request,
// or of the one under the cursor.
func (m *Model) startReview() tea.Cmd {
	prs := m.menuTargets()
	if len(prs) == 0 {
		return nil
	}
	return m.startCompose(prs, model.EventComment)
}

// startCompose opens the popup for a review of prs, of the kind event.
func (m *Model) startCompose(prs []model.PR, event model.ReviewEvent) tea.Cmd {
	own := len(prs) == 1 && m.own(prs[0])
	if own && event != model.EventComment {
		m.refuse("you cannot request changes on your own pull request")
		return nil
	}
	c := &compose{pr: prs[0], prs: prs, own: own, ta: newInput(m.st)}
	for i, k := range kinds {
		if k == event {
			c.kind = i
		}
	}
	c.ta.Placeholder = "Write the review (markdown)"
	return m.openCompose(c)
}

func (m *Model) openCompose(c *compose) tea.Cmd {
	c.ta.ShowLineNumbers = false
	c.resize(m.width, m.height)
	m.compose = c
	return c.ta.Focus()
}

func (m *Model) composeKey(msg tea.KeyPressMsg) tea.Cmd { return m.popupKey(m.composeSpec(), msg) }

// composeSpec is the popup a comment or a review is written in, as the
// shared popup layer reads it: a text box, with the focus when it opens,
// and the popup's own keys, which work in it and out of it (ADR-125). esc
// takes the focus out of the text box, and only then throws the text away.
func (m *Model) composeSpec() popupSpec {
	c := m.compose
	return popupSpec{
		own: []ownKey{
			{action: "post", keyLine: true, run: func() tea.Cmd {
				ev := kinds[c.kind]
				body := strings.TrimSpace(c.ta.Value())
				if body == "" && (c.single || ev != model.EventApprove) {
					m.refuse("write the " + sentWord(c.single, ev) + " first")
					return nil
				}
				if !c.single {
					// The popup reads each pull request again and asks, in
					// place of this one; the review is kept until its batch
					// starts.
					return m.startBulkFor(runner.Review, runner.ReviewText{Event: ev, Body: body}, c.prs)
				}
				// The text is the review's field the popup shows and a batch
				// keeps.
				return m.startBulkFor(runner.Comment, runner.ReviewText{Body: body}, c.prs)
			}},
			{action: "next_kind", words: "kind", hide: c.single, run: func() tea.Cmd {
				if !c.own && !c.single {
					c.kind = (c.kind + 1) % len(kinds)
				}
				return nil
			}},
			{action: "editor", words: "$EDITOR", run: m.openEditor},
		},
		text: &popupText{
			focused: c.ta.Focused(),
			focus:   c.ta.Focus,
			blur:    c.ta.Blur,
			edit:    c.update,
			editNav: navActions,
		},
		closeWords: "discard",
		close: func() {
			m.compose = nil
			m.say(sentWord(c.single, kinds[c.kind]) + " discarded")
		},
	}
}

// editedMsg is the editor's exit, with the file it edited.
type editedMsg struct {
	path string
	err  error
}

// openEditor edits the review in the editor, as a file, and reads it back.
func (m *Model) openEditor() tea.Cmd {
	args := strings.Fields(m.set.Editor)
	if len(args) == 0 {
		m.refuse("set review.editor or $EDITOR to write the review in an editor")
		return nil
	}
	f, err := os.CreateTemp("", "gh-desk-review-*.md")
	if err != nil {
		m.log.Warn("could not make a file for the editor", "err", err.Error())
		m.fail("could not make a file for the editor: " + err.Error())
		return nil
	}
	path := f.Name()
	_, err = f.WriteString(m.compose.ta.Value())
	f.Close()
	if err != nil {
		os.Remove(path)
		m.log.Warn("could not write the review for the editor", "err", err.Error())
		m.fail("could not write the review for the editor: " + err.Error())
		return nil
	}
	cmd := exec.Command(args[0], append(args[1:], path)...)
	return tea.ExecProcess(cmd, func(err error) tea.Msg { return editedMsg{path, err} })
}

func (m *Model) edited(msg editedMsg) tea.Cmd {
	defer os.Remove(msg.path)
	if msg.err != nil {
		m.log.Warn("the editor failed", "err", msg.err.Error())
		m.fail("the editor failed: " + msg.err.Error())
		return nil
	}
	b, err := os.ReadFile(msg.path)
	if err != nil {
		m.log.Warn("could not read the review back", "err", err.Error())
		m.fail("could not read the review back: " + err.Error())
		return nil
	}
	if m.compose != nil {
		m.compose.ta.SetValue(strings.TrimRight(string(b), "\n"))
	}
	return nil
}

// prsWord names one pull request by its key, and more by their count.
func prsWord(prs []model.PR) string {
	if len(prs) == 1 {
		return prs[0].Key.String()
	}
	return fmt.Sprintf("%d pull requests", len(prs))
}

// renderCompose draws the popup a comment or a review is written in.
// renderCompose draws the popup and says on which of its rows the text
// box starts.
func (m *Model) renderCompose() ([]string, int) {
	c, st := m.compose, m.st
	lines := []string{st.muted.Render(shownTitle(c.pr.Title))}
	if len(c.prs) > 1 {
		lines = keysLines(st.sub0, c.prs, composeWidth(m.width)-4, composeKeysLines)
	}
	title := prsWord(c.prs) + " · review"
	if c.single {
		title = prsWord(c.prs) + " · comment"
		lines = append(lines, "")
	} else {
		var ks []string
		for i, k := range kinds {
			off := st.subOff
			if c.own && k != model.EventComment {
				off = st.dim
			}
			ks = append(ks, pill(st.subOn, off, kindWords[k], i == c.kind))
		}
		lines = append(lines, strings.Join(ks, tabGap), "")
	}
	// Below the box's top edge.
	at := 1 + len(lines)
	lines = append(lines, strings.Split(c.ta.View(), "\n")...)
	lines = append(lines, "", m.popupFoot(m.composeSpec(), ""))
	return box(st, title, lines, composeWidth(m.width)), at
}

// confirmBodyLines is how much of a comment or a review a confirm popup
// shows; the rest was read where it was written.
const confirmBodyLines = 6

// bodyLines are the first lines of a comment or a review, made safe and
// wrapped to width.
func bodyLines(st styles, body string, width int) []string {
	var out []string
	for l := range strings.SplitSeq(body, "\n") {
		out = append(out, hang("  ", st.sub0.Render(safeLine(l)), width)...)
	}
	if len(out) > confirmBodyLines {
		out = append(out[:confirmBodyLines], st.dim.Render("  …"))
	}
	return out
}

// keysOf names pull requests by their keys, on one line.
func keysOf(prs []model.PR) string {
	var ks []string
	for _, p := range prs {
		ks = append(ks, p.Key.String())
	}
	return strings.Join(ks, "  ")
}

func short(oid string) string {
	if len(oid) > 7 {
		return oid[:7]
	}
	return oid
}

// box draws lines in a rounded frame w wide, with title on its top edge.
// newInput is a text box that leaves the cursor to the terminal, so an
// input method shows the text being composed where it goes. Its colors are
// those for the background st's theme is for: the box's own are for a dark
// one, and on a light one they drew the line being typed in dark on black.
func newInput(st styles) textarea.Model {
	ta := textarea.New()
	ta.SetStyles(textarea.DefaultStyles(st.p.dark))
	ta.SetVirtualCursor(false)
	return ta
}

// boxTop is the top edge of a popup's box, w wide, with title on its left
// and, when given, info on its right.
func boxTop(st styles, title, info string, w int) string {
	head := st.focus.Render("╭ ") + st.title.Render(title) + " "
	right := st.focus.Render("╮")
	if info != "" {
		right = st.dim.Render(" "+info+" ") + st.focus.Render("─╮")
	}
	return head + st.focus.Render(strings.Repeat("─", max(w-lipgloss.Width(head)-lipgloss.Width(right), 0))) + right
}

// box is a popup's box, w wide, with title on its top border. It takes
// the keys while it is shown, so its frame is in the focus color
// (ADR-079).
func box(st styles, title string, lines []string, w int) []string {
	inner := w - 4
	out := []string{boxTop(st, title, "", w)}
	for _, l := range lines {
		out = append(out, st.focus.Render("│ ")+fit(l, inner)+st.focus.Render(" │"))
	}
	return append(out, st.focus.Render("╰"+strings.Repeat("─", max(w-2, 0))+"╯"))
}
