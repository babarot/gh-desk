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

// kinds are the kinds of review, in the order the writing screen cycles
// through them.
var kinds = []model.ReviewEvent{model.EventComment, model.EventApprove, model.EventRequestChanges}

var kindWords = map[model.ReviewEvent]string{
	model.EventComment:        "comment",
	model.EventApprove:        "approve",
	model.EventRequestChanges: "request changes",
}

// compose is a review being written.
type compose struct {
	pr   model.PR
	kind int
	// own is the viewer's own pull request, which can only be commented
	// on.
	own  bool
	ta   textarea.Model
	back view
}

func (c *compose) resize(w, h int) {
	c.ta.SetWidth(max(w-4, 20))
	c.ta.SetHeight(max(h-8, 3))
}

func (c *compose) update(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	c.ta, cmd = c.ta.Update(msg)
	return cmd
}

// confirm asks before a review is sent: GitHub cannot take one back.
type confirm struct {
	pr    model.PR
	event model.ReviewEvent
	body  string
}

func (m *Model) own(p model.PR) bool { return m.viewer != "" && p.Author == m.viewer }

// busy refuses a review while one is on its way, so a second key press
// does not send it twice.
func (m *Model) busy() bool {
	if m.sending == nil {
		return false
	}
	m.setStatus(fmt.Sprintf("a review of %s is being sent; wait for it", *m.sending), true)
	return true
}

// askApprove opens the approve popup for the pull request under the
// cursor or on screen; an approve goes through the runner, like many.
func (m *Model) askApprove(model.PR) tea.Cmd {
	return m.startBulk(runner.Approve)
}

func (m *Model) startCompose(p model.PR, event model.ReviewEvent) tea.Cmd {
	if m.own(p) && event != model.EventComment {
		m.setStatus("you cannot request changes on your own pull request", true)
		return nil
	}
	c := &compose{pr: p, own: m.own(p), ta: textarea.New(), back: m.view}
	for i, k := range kinds {
		if k == event {
			c.kind = i
		}
	}
	c.ta.Placeholder = "Write the review (markdown)"
	c.ta.ShowLineNumbers = false
	c.resize(m.width, m.height)
	m.compose = c
	m.view = viewCompose
	return c.ta.Focus()
}

func (m *Model) composeKey(msg tea.KeyPressMsg) tea.Cmd {
	c := m.compose
	k := msg.String()
	switch {
	case k == keymap.Back:
		m.view, m.compose = c.back, nil
		m.setStatus("review discarded", false)
		return nil
	case m.keys.Matches("next_kind", k):
		if !c.own {
			c.kind = (c.kind + 1) % len(kinds)
		}
		return nil
	case m.keys.Matches("post", k):
		if m.busy() {
			return nil
		}
		ev := kinds[c.kind]
		body := strings.TrimSpace(c.ta.Value())
		if body == "" && ev != model.EventApprove {
			m.setStatus("write the review first", true)
			return nil
		}
		m.confirm = &confirm{pr: c.pr, event: ev, body: body}
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
		m.setStatus("set $EDITOR to write the review in an editor", true)
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
		key := c.pr.Key
		m.sending = &key
		m.setStatus(fmt.Sprintf("sending %s to %s…", kindWords[c.event], c.pr.Key), false)
		return func() tea.Msg { return reviewMsg{c.pr, c.event, m.src.Review(m.ctx, c.pr, c.event, c.body)} }
	case k == keymap.Back:
		m.confirm = nil
		m.setStatus("canceled", false)
	}
	return nil
}

// reviewed takes in a sent review: the row shows it, the writing screen
// closes, and the notification is marked done when the settings say so.
func (m *Model) reviewed(msg reviewMsg) tea.Cmd {
	m.sending = nil
	if msg.err != nil {
		m.setStatus(fmt.Sprintf("%s on %s failed: %v", kindWords[msg.event], msg.pr.Key, msg.err), true)
		return nil
	}
	if m.view == viewCompose && m.compose != nil && m.compose.pr.Key == msg.pr.Key {
		m.view, m.compose = m.compose.back, nil
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
	m.setStatus(fmt.Sprintf("✓ %s sent to %s", kindWords[msg.event], msg.pr.Key), false)
	n, ok := m.unread[msg.pr.Key]
	if !ok || !m.set.DoneAfterReview {
		return nil
	}
	delete(m.unread, msg.pr.Key)
	m.inboxTab().hidden[msg.pr.Key] = true
	return func() tea.Msg { return doneMsg{n, m.src.MarkDone(m.ctx, n)} }
}

func (m *Model) renderCompose(b *strings.Builder, height int) {
	c, st := m.compose, m.st
	b.WriteString(fit(st.sub1.Render(c.pr.Key.String())+"  "+st.muted.Render(c.pr.Title), m.width) + "\n")
	var ks []string
	for i, k := range kinds {
		switch {
		case c.own && k != model.EventComment:
			ks = append(ks, st.dim.Render(kindWords[k]))
		case i == c.kind:
			ks = append(ks, st.tabOn.Render(kindWords[k]))
		default:
			ks = append(ks, st.muted.Render(kindWords[k]))
		}
	}
	b.WriteString(strings.Join(ks, "   ") + "\n\n")
	view := c.ta.View()
	b.WriteString(view + "\n")
	for i := lipgloss.Height(view) + 3; i < height; i++ {
		b.WriteString("\n")
	}
}

// renderConfirm draws the popup over the screen's lines.
func (m *Model) renderConfirm() []string {
	c, st := m.confirm, m.st
	w := min(max(m.width-8, 30), 100)
	title := kindWords[c.event]
	lines := []string{
		st.sub1.Render(c.pr.Key.String()) + " " + st.muted.Render(c.pr.Author),
		st.muted.Render(c.pr.Title),
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
	lines = append(lines, "", st.dim.Render(fmt.Sprintf("Reviews the head you saw (%s). GitHub cannot take a review back.", short(c.pr.HeadOID))))
	if _, ok := m.unread[c.pr.Key]; ok && m.set.DoneAfterReview {
		lines = append(lines, st.dim.Render("Its notification is marked done after."))
	}
	lines = append(lines, "", st.sub0.Render("send "+title+"? ")+st.key.Render(m.firstKey("confirm"))+" "+st.muted.Render("yes")+
		st.dim.Render(" · ")+st.key.Render(keymap.Back)+" "+st.muted.Render("no"))
	return box(st, title, lines, w)
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
