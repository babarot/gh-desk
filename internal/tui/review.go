package tui

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/babarot/gh-triage/internal/model"
	"github.com/babarot/gh-triage/internal/runner"
	"github.com/babarot/gh-triage/internal/tui/keymap"
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

// confirm asks before a comment on the conversations is sent. A review is
// asked about in the confirm popup of a batch, as an approval is.
type confirm struct {
	pr model.PR
	// prs are the comment's pull requests.
	prs  []model.PR
	body string
	// top is the first line of the popup's body shown.
	top int
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

// busy refuses a write while a comment is on its way, so a second key
// press does not send it twice.
func (m *Model) busy() bool {
	if m.sending == "" {
		return false
	}
	m.refuse(fmt.Sprintf("a comment on %s is being sent; wait for it", m.sending))
	return true
}

// startComment opens the popup for a comment on the conversation of each
// selected pull request, or of the one under the cursor.
func (m *Model) startComment() tea.Cmd {
	prs := m.menuTargets()
	if len(prs) == 0 {
		return nil
	}
	c := &compose{pr: prs[0], prs: prs, single: true, ta: newInput()}
	c.ta.Placeholder = "Write the comment (markdown)"
	// A comment that failed comes back to be sent again.
	c.ta.SetValue(m.unsent)
	m.unsent = ""
	return m.openCompose(c)
}

// startReview opens the popup for a review of each selected pull request,
// or of the one under the cursor. A review that was not sent to some
// comes back to be sent again.
func (m *Model) startReview() tea.Cmd {
	prs := m.menuTargets()
	if len(prs) == 0 {
		return nil
	}
	ev, body := model.EventComment, ""
	if u := m.unsentReview; u != nil {
		ev, body = u.Event, u.Body
		m.unsentReview = nil
	}
	if len(prs) == 1 && m.own(prs[0]) {
		ev = model.EventComment
	}
	cmd := m.startCompose(prs, ev)
	if m.compose != nil {
		m.compose.ta.SetValue(body)
	}
	return cmd
}

// startCompose opens the popup for a review of prs, of the kind event.
func (m *Model) startCompose(prs []model.PR, event model.ReviewEvent) tea.Cmd {
	own := len(prs) == 1 && m.own(prs[0])
	if own && event != model.EventComment {
		m.refuse("you cannot request changes on your own pull request")
		return nil
	}
	c := &compose{pr: prs[0], prs: prs, own: own, ta: newInput()}
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

func (m *Model) composeKey(msg tea.KeyPressMsg) tea.Cmd {
	c := m.compose
	k := msg.String()
	switch {
	case k == keymap.Back:
		m.compose = nil
		m.say(sentWord(c.single, kinds[c.kind]) + " discarded")
		return nil
	case m.keys.Matches("next_kind", k):
		if !c.own && !c.single {
			c.kind = (c.kind + 1) % len(kinds)
		}
		return nil
	case m.keys.Matches("post", k):
		if m.busy() {
			return nil
		}
		ev := kinds[c.kind]
		body := strings.TrimSpace(c.ta.Value())
		if body == "" && (c.single || ev != model.EventApprove) {
			m.refuse("write the " + sentWord(c.single, ev) + " first")
			return nil
		}
		if !c.single {
			// The popup reads each pull request again and asks; the
			// review stays under it until its batch starts.
			return m.startBulkFor(runner.Review, runner.ReviewText{Event: ev, Body: body}, c.prs)
		}
		m.confirm = &confirm{pr: c.pr, prs: c.prs, body: body}
		return nil
	case m.keys.Matches("editor", k):
		return m.openEditor()
	}
	return c.update(msg)
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
	f, err := os.CreateTemp("", "gh-triage-review-*.md")
	if err != nil {
		m.fail("could not make a file for the editor: " + err.Error())
		return nil
	}
	path := f.Name()
	_, err = f.WriteString(m.compose.ta.Value())
	f.Close()
	if err != nil {
		os.Remove(path)
		m.fail("could not write the review for the editor: " + err.Error())
		return nil
	}
	cmd := exec.Command(args[0], append(args[1:], path)...)
	return tea.ExecProcess(cmd, func(err error) tea.Msg { return editedMsg{path, err} })
}

func (m *Model) edited(msg editedMsg) tea.Cmd {
	defer os.Remove(msg.path)
	if msg.err != nil {
		m.fail("the editor failed: " + msg.err.Error())
		return nil
	}
	b, err := os.ReadFile(msg.path)
	if err != nil {
		m.fail("could not read the review back: " + err.Error())
		return nil
	}
	if m.compose != nil {
		m.compose.ta.SetValue(strings.TrimRight(string(b), "\n"))
	}
	return nil
}

func (m *Model) confirmKey(k string) tea.Cmd {
	c := m.confirm
	switch {
	case m.keys.Matches("confirm", k):
		m.confirm = nil
		// The popup closes at once; the status line follows the comment
		// to each pull request.
		m.compose = nil
		m.sending = prsWord(c.prs)
		m.posting = &posting{body: c.body, prs: c.prs}
		return m.postNext(m.posting)
	case k == keymap.Back:
		m.confirm = nil
		m.say("canceled")
	case m.keys.Matches("down", k):
		// Kept within the body when it is drawn.
		c.top++
	case m.keys.Matches("up", k):
		c.top = max(c.top-1, 0)
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

// posting is a comment on its way to its pull requests, one after
// another, while the screen is free.
type posting struct {
	body   string
	prs    []model.PR
	next   int
	sent   int
	failed []model.PR
	err    error
}

// postedMsg is the comment sent to one pull request of a posting.
type postedMsg struct {
	p   *posting
	err error
}

// postNext sends the comment to the posting's next pull request. GitHub
// asks for writes one at a time.
func (m *Model) postNext(p *posting) tea.Cmd {
	pr := p.prs[p.next]
	return func() tea.Msg { return postedMsg{p, m.src.PostComment(m.ctx, pr, p.body)} }
}

// posted takes in one comment sent and sends the next. When the last is
// back, those it failed on are selected and the comment is kept, so c
// writes it again for them.
func (m *Model) posted(msg postedMsg) tea.Cmd {
	p := msg.p
	if msg.err != nil {
		p.failed = append(p.failed, p.prs[p.next])
		if p.err == nil {
			p.err = msg.err
		}
	} else {
		p.sent++
	}
	p.next++
	if p.next < len(p.prs) {
		return m.postNext(p)
	}
	m.posting, m.sending = nil, ""
	if len(p.failed) == 0 {
		m.say(fmt.Sprintf("✓ comment sent to %s", prsWord(p.prs)))
		return nil
	}
	clear(m.selected)
	for _, f := range p.failed {
		m.selected[f.Key] = byHand
	}
	m.unsent = p.body
	return m.failNoted(fmt.Sprintf("comment failed on %d of %d (%s: %v) · they are selected; %s to send it again",
		len(p.failed), len(p.prs), p.failed[0].Key, p.err, m.firstKey("comment")), p.failed[0].Key)
}

// postingActivity is a comment on its way.
func (m *Model) postingActivity() string {
	p := m.posting
	if p == nil {
		return ""
	}
	text := "sending the comment to " + prsWord(p.prs) + "…"
	if len(p.prs) > 1 {
		text = fmt.Sprintf("sending the comment… %d/%d", p.next+1, len(p.prs))
	}
	return text
}

// renderCompose draws the popup a comment or a review is written in.
// renderCompose draws the popup and says on which of its rows the text
// box starts.
func (m *Model) renderCompose() ([]string, int) {
	c, st := m.compose, m.st
	lines := []string{st.muted.Render(c.pr.Title)}
	if len(c.prs) > 1 {
		lines = keysLines(st.sub0, c.prs, composeWidth(m.width)-4, composeKeysLines)
	}
	title := prsWord(c.prs) + " · review"
	keys := st.key.Render(m.firstKey("post")) + " " + st.muted.Render("post")
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
		keys += st.dim.Render(" · ") + st.key.Render(m.firstKey("next_kind")) + " " + st.muted.Render("kind")
	}
	// Below the box's top edge.
	at := 1 + len(lines)
	lines = append(lines, strings.Split(c.ta.View(), "\n")...)
	lines = append(lines, "", keys+
		st.dim.Render(" · ")+st.key.Render(m.firstKey("editor"))+" "+st.muted.Render("$EDITOR")+
		st.dim.Render(" · ")+st.key.Render(keymap.Back)+" "+st.muted.Render("discard"))
	return box(st, title, lines, composeWidth(m.width)), at
}

// renderConfirm draws the popup over the screen's lines.
func (m *Model) renderConfirm() []string {
	c, st := m.confirm, m.st
	w := min(max(m.width-8, 30), 100)
	inner := w - 4
	title := "comment"
	lines := []string{
		st.sub1.Render(c.pr.Key.String()) + " " + st.muted.Render(c.pr.Author),
		// The title may be cut: the key above names the pull request.
		st.muted.Render(c.pr.Title),
	}
	if len(c.prs) > 1 {
		title = fmt.Sprintf("comment on %d pull requests", len(c.prs))
		lines = keysLines(st.sub0, c.prs, inner, 3)
	}
	lines = append(lines, "")
	lines = append(lines, bodyLines(st, c.body, inner)...)
	lines = append(lines, "")
	lines = append(lines, wrapped(st.dim, "Adds the comment to each conversation; a review asked of you stays asked.", inner)...)
	foot := []string{"", st.sub0.Render("send "+title+"? ") + st.key.Render(m.firstKey("confirm")) + " " + st.muted.Render("yes") +
		st.dim.Render(" · ") + st.key.Render(keymap.Back) + " " + st.muted.Render("no")}
	out, top := scrollBox(st, title, lines, foot, w, m.popupHeight(), c.top, keepLines{}, m.scrollKeys())
	c.top = top
	return out
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
// input method shows the text being composed where it goes.
func newInput() textarea.Model {
	ta := textarea.New()
	ta.SetVirtualCursor(false)
	return ta
}

func box(st styles, title string, lines []string, w int) []string {
	inner := w - 4
	top := st.border.Render("╭ ") + st.title.Render(title) + " "
	top += st.border.Render(strings.Repeat("─", max(w-lipgloss.Width(top)-1, 0)) + "╮")
	out := []string{top}
	for _, l := range lines {
		out = append(out, st.border.Render("│ ")+fit(l, inner)+st.border.Render(" │"))
	}
	return append(out, st.border.Render("╰"+strings.Repeat("─", max(w-2, 0))+"╯"))
}
