package tui

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/babarot/gh-triage/internal/keymap"
	"github.com/babarot/gh-triage/internal/model"
	"github.com/babarot/gh-triage/internal/runner"
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
// screen it was opened from.
type compose struct {
	pr model.PR
	// single is a comment on the conversation, not a review: it has no
	// kind, and leaves a review request from the viewer as it is. It goes
	// to every one of prs, which starts with pr.
	single bool
	prs    []model.PR
	kind   int
	// own is the viewer's own pull request, which can only be commented
	// on.
	own bool
	ta  textarea.Model
}

// composeWidth is the width of the popup on a screen w wide.
func composeWidth(w int) int { return min(max(w-8, 40), 100) }

// resize fits the text area in the popup on a screen w by h: the popup
// takes up to seven lines besides it, and leaves the status and key lines under
// it in sight. A longer review is written in $EDITOR.
func (c *compose) resize(w, h int) {
	c.ta.SetWidth(composeWidth(w) - 4)
	c.ta.SetHeight(min(max(h-12, 3), 10))
}

func (c *compose) update(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	c.ta, cmd = c.ta.Update(msg)
	return cmd
}

// confirm asks before a comment or a review is sent: GitHub cannot take a
// review back.
type confirm struct {
	pr     model.PR
	single bool
	// prs and from are a comment's pull requests and its popup.
	prs   []model.PR
	from  *compose
	event model.ReviewEvent
	body  string
}

// sentWord names what is sent: a comment, or a review of its kind.
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

// busy refuses a review while one is on its way, so a second key press
// does not send it twice.
func (m *Model) busy() bool {
	if m.sending == "" {
		return false
	}
	m.setStatus(fmt.Sprintf("a comment or review of %s is being sent; wait for it", m.sending), true)
	return true
}

// askApprove opens the approve popup for the pull request under the
// cursor or on screen; an approve goes through the runner, like many.
func (m *Model) askApprove(model.PR) tea.Cmd {
	return m.startBulk(runner.Approve)
}

// startComment opens the popup for a comment on the conversation of each
// selected pull request, or of the one under the cursor.
func (m *Model) startComment() tea.Cmd {
	prs := m.menuTargets()
	if len(prs) == 0 {
		return nil
	}
	c := &compose{pr: prs[0], prs: prs, single: true, ta: textarea.New()}
	c.ta.Placeholder = "Write the comment (markdown)"
	return m.openCompose(c)
}

// startCompose opens the popup for a review of p, of the kind event.
func (m *Model) startCompose(p model.PR, event model.ReviewEvent) tea.Cmd {
	if m.own(p) && event != model.EventComment {
		m.setStatus("you cannot request changes on your own pull request", true)
		return nil
	}
	c := &compose{pr: p, own: m.own(p), ta: textarea.New()}
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
		m.setStatus(sentWord(c.single, kinds[c.kind])+" discarded", false)
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
			m.setStatus("write the "+sentWord(c.single, ev)+" first", true)
			return nil
		}
		m.confirm = &confirm{pr: c.pr, single: c.single, prs: c.prs, from: c, event: ev, body: body}
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
		m.setStatus("set review.editor or $EDITOR to write the review in an editor", true)
		return nil
	}
	f, err := os.CreateTemp("", "gh-triage-review-*.md")
	if err != nil {
		m.setStatus("could not make a file for the editor: "+err.Error(), true)
		return nil
	}
	path := f.Name()
	_, err = f.WriteString(m.compose.ta.Value())
	f.Close()
	if err != nil {
		os.Remove(path)
		m.setStatus("could not write the review for the editor: "+err.Error(), true)
		return nil
	}
	cmd := exec.Command(args[0], append(args[1:], path)...)
	return tea.ExecProcess(cmd, func(err error) tea.Msg { return editedMsg{path, err} })
}

func (m *Model) edited(msg editedMsg) tea.Cmd {
	defer os.Remove(msg.path)
	if msg.err != nil {
		m.setStatus("the editor failed: "+msg.err.Error(), true)
		return nil
	}
	b, err := os.ReadFile(msg.path)
	if err != nil {
		m.setStatus("could not read the review back: "+err.Error(), true)
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
		if c.single {
			m.sending = prsWord(c.prs)
			m.setStatus(fmt.Sprintf("sending the comment to %s…", m.sending), false)
			return m.postComments(c)
		}
		m.sending = c.pr.Key.String()
		m.setStatus(fmt.Sprintf("sending %s to %s…", sentWord(false, c.event), c.pr.Key), false)
		return func() tea.Msg { return reviewMsg{c.pr, c.event, m.src.Review(m.ctx, c.pr, c.event, c.body)} }
	case k == keymap.Back:
		m.confirm = nil
		m.setStatus("canceled", false)
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

// commentsMsg is a comment sent to each of its popup's pull requests, one
// after another: those it reached, and those it failed on with why.
type commentsMsg struct {
	from   *compose
	sent   []model.PR
	failed []model.PR
	errs   []error
}

// postComments sends a comment to its pull requests one at a time, as
// GitHub asks of writes, and goes on past one that fails.
func (m *Model) postComments(c *confirm) tea.Cmd {
	return func() tea.Msg {
		msg := commentsMsg{from: c.from}
		for _, p := range c.prs {
			if err := m.src.PostComment(m.ctx, p, c.body); err != nil {
				msg.failed, msg.errs = append(msg.failed, p), append(msg.errs, err)
				continue
			}
			msg.sent = append(msg.sent, p)
		}
		return msg
	}
}

// commented takes in a comment sent: the popup closes once it reached
// every pull request; otherwise it stays, with only those it failed on, to
// send again.
func (m *Model) commented(msg commentsMsg) tea.Cmd {
	m.sending = ""
	if len(msg.failed) == 0 {
		if m.compose == msg.from {
			m.compose = nil
		}
		m.setStatus(fmt.Sprintf("✓ comment sent to %s", prsWord(msg.sent)), false)
		return nil
	}
	if m.compose == msg.from {
		m.compose.prs, m.compose.pr = msg.failed, msg.failed[0]
	}
	text := fmt.Sprintf("comment on %s failed: %v", msg.failed[0].Key, msg.errs[0])
	if len(msg.sent) > 0 || len(msg.failed) > 1 {
		text = fmt.Sprintf("comment sent to %d, failed on %d (%s: %v); the popup keeps those it failed on",
			len(msg.sent), len(msg.failed), msg.failed[0].Key, msg.errs[0])
	}
	m.setStatus(text, true)
	return nil
}

// reviewed takes in a sent review: the popup closes, the row shows it, and
// the notification is marked done when the settings say so.
func (m *Model) reviewed(msg reviewMsg) tea.Cmd {
	m.sending = ""
	what := sentWord(false, msg.event)
	if msg.err != nil {
		m.setStatus(fmt.Sprintf("%s on %s failed: %v", what, msg.pr.Key, msg.err), true)
		return nil
	}
	if m.compose != nil && !m.compose.single && m.compose.pr.Key == msg.pr.Key {
		m.compose = nil
	}
	state := map[model.ReviewEvent]model.ReviewState{
		model.EventApprove: model.ReviewApproved, model.EventComment: model.ReviewCommented, model.EventRequestChanges: model.ReviewChangesRequested,
	}[msg.event]
	if p, ok := m.knownPR(msg.pr.Key); ok {
		var rs []model.Review
		for _, r := range p.Reviews {
			if r.Author != m.viewer {
				rs = append(rs, r)
			}
		}
		p.Reviews = append(rs, model.Review{Author: m.viewer, State: state})
		m.updatePR(p)
	}
	m.setStatus(fmt.Sprintf("✓ %s sent to %s", what, msg.pr.Key), false)
	n, ok := m.unread[msg.pr.Key]
	if !ok || !m.set.MarkDone {
		return nil
	}
	delete(m.unread, msg.pr.Key)
	m.inboxTab().hidden[msg.pr.Key] = true
	return func() tea.Msg { return doneMsg{n, m.src.MarkDone(m.ctx, n)} }
}

// renderCompose draws the popup a comment or a review is written in.
func (m *Model) renderCompose() []string {
	c, st := m.compose, m.st
	lines := []string{st.muted.Render(c.pr.Title)}
	title := c.pr.Key.String() + " · review"
	keys := st.key.Render(m.firstKey("post")) + " " + st.muted.Render("post")
	if c.single {
		title = prsWord(c.prs) + " · comment"
		if len(c.prs) > 1 {
			lines = []string{st.sub0.Render(keysOf(c.prs))}
		}
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
	lines = append(lines, strings.Split(c.ta.View(), "\n")...)
	lines = append(lines, "", keys+
		st.dim.Render(" · ")+st.key.Render(m.firstKey("editor"))+" "+st.muted.Render("$EDITOR")+
		st.dim.Render(" · ")+st.key.Render(keymap.Back)+" "+st.muted.Render("discard"))
	return box(st, title, lines, composeWidth(m.width))
}

// renderConfirm draws the popup over the screen's lines.
func (m *Model) renderConfirm() []string {
	c, st := m.confirm, m.st
	w := min(max(m.width-8, 30), 100)
	title := sentWord(c.single, c.event)
	lines := []string{
		st.sub1.Render(c.pr.Key.String()) + " " + st.muted.Render(c.pr.Author),
		st.muted.Render(c.pr.Title),
	}
	if len(c.prs) > 1 {
		title = fmt.Sprintf("comment on %d pull requests", len(c.prs))
		lines = []string{st.sub0.Render(keysOf(c.prs))}
	}
	if c.body != "" {
		lines = append(lines, "")
		for i, l := range strings.Split(c.body, "\n") {
			if i == 6 {
				lines = append(lines, st.dim.Render("  …"))
				break
			}
			lines = append(lines, st.sub0.Render("  "+l))
		}
	}
	if c.single {
		lines = append(lines, "", st.dim.Render("Adds the comment to each conversation; a review asked of you stays asked."))
	} else {
		lines = append(lines, "", st.dim.Render(fmt.Sprintf("Reviews the head you saw (%s). GitHub cannot take a review back.", short(c.pr.HeadOID))))
	}
	if _, ok := m.unread[c.pr.Key]; ok && m.set.MarkDone && !c.single {
		lines = append(lines, st.dim.Render("Its notification is marked done after."))
	}
	lines = append(lines, "", st.sub0.Render("send "+title+"? ")+st.key.Render(m.firstKey("confirm"))+" "+st.muted.Render("yes")+
		st.dim.Render(" · ")+st.key.Render(keymap.Back)+" "+st.muted.Render("no"))
	return box(st, title, lines, w)
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
