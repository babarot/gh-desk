package tui

import (
	"fmt"
	"slices"
	"strings"

	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"

	"github.com/babarot/gh-desk/internal/model"
	"github.com/babarot/gh-desk/internal/plan"
	"github.com/babarot/gh-desk/internal/runner"
)

// closeForm is what the confirm popup of a close holds besides its pull
// requests: a comment and whether to delete the head branches, both drawn
// from the start (ADR-066).
type closeForm struct {
	ta  textarea.Model
	del bool
	// editing is set while the text box has the focus: every key is typed
	// into it, and esc gives the keys back to the popup.
	editing bool
	// based are the open pull requests based on each head branch, read
	// once the pull requests are; basedErr is why they could not be.
	based    map[model.Branch][]int
	basedErr error
	// at is the line of the popup the text box starts on, for the cursor,
	// and checkAt the line of the check, for the mouse.
	at, checkAt int
	// armed is set by a click on the check: the next click on it changes
	// it, as a choice in a popup is made by a second click (ADR-053).
	armed bool
}

// closeMouse takes a click in the popup of a close: a second click on
// the check changes it, and a click on the text box puts the focus in it.
// While the box has the focus it takes no mouse input, so a click does
// not throw away what is typed (ADR-053). ok is false for input the
// popup's own handling takes.
func (m *Model) closeMouse(msg tea.MouseMsg) (cmd tea.Cmd, ok bool) {
	f := m.bulk.close
	if f.editing {
		return nil, true
	}
	c, click := msg.(tea.MouseClickMsg)
	if !click || c.Button != tea.MouseLeft || !m.popupAt.has(c.X, c.Y) {
		return nil, false
	}
	switch row := c.Y - m.popupAt.y; {
	case row == f.checkAt && f.armed:
		f.armed, f.del = false, !f.del
		m.decideBulk()
	case row == f.checkAt:
		f.armed = true
	case row >= f.at && row < f.at+closeInputLines:
		f.armed = false
		return f.focus(), true
	default:
		f.armed = false
	}
	return nil, true
}

// closeInputLines is how many lines the text box of a close takes.
const closeInputLines = 4

// startClose opens the confirm popup of a close for the selection or the
// pull request under the cursor. A comment written in a popup closed
// without confirming comes back.
func (m *Model) startClose() tea.Cmd {
	cmd := m.startBulkOf(runner.Close, runner.ReviewText{})
	if m.bulk == nil || m.bulk.kind != runner.Close {
		return cmd
	}
	m.bulk.close = m.newCloseForm(m.unsentClose)
	m.unsentClose = ""
	return cmd
}

// newCloseForm is the form of a close popup, starting with the comment.
func (m *Model) newCloseForm(comment string) *closeForm {
	f := &closeForm{ta: newInput(m.st)}
	f.ta.ShowLineNumbers = true
	f.ta.EndOfBufferCharacter = '~'
	f.ta.Placeholder = "Leave a comment (markdown)"
	f.ta.SetHeight(closeInputLines)
	f.ta.SetValue(comment)
	f.ta.Blur()
	return f
}

// decide is what closing a pull request read again would do, with the
// form as it is now.
func (f *closeForm) decide(mi model.MergeInfo) plan.Close {
	c := plan.ForClose(mi, f.based[mi.Head], f.body() != "", f.del)
	if c.Delete && f.basedErr != nil {
		c.Delete, c.Keep = false, "could not tell what is based on it"
	}
	return c
}

func (f *closeForm) body() string { return strings.TrimSpace(f.ta.Value()) }

// text is what the runner is given.
func (f *closeForm) text() runner.CloseText {
	return runner.CloseText{Body: f.body(), DeleteBranch: f.del}
}

func (f *closeForm) focus() tea.Cmd {
	f.editing = true
	return f.ta.Focus()
}

type basedOnMsg struct {
	b   *bulk
	got map[model.Branch][]int
	err error
}

// readBasedOn reads the open pull requests based on the head branches the
// popup could delete, so it can say which it keeps before the user says
// yes. The popup stays reading until they are back.
func (m *Model) readBasedOn(b *bulk) tea.Cmd {
	if m.set.Offline {
		return nil
	}
	var bs []model.Branch
	for _, it := range b.items {
		mi := it.mi
		if it.ok && mi.State == model.StateOpen && !mi.HeadInFork && mi.HeadRefID != "" && mi.CanWrite && !slices.Contains(bs, mi.Head) {
			bs = append(bs, mi.Head)
		}
	}
	if len(bs) == 0 {
		return nil
	}
	b.loading = true
	return func() tea.Msg {
		got, err := m.src.BasedOn(m.ctx, bs)
		return basedOnMsg{b, got, err}
	}
}

func (m *Model) basedOnRead(msg basedOnMsg) {
	b := msg.b
	if m.bulk != b || b.close == nil {
		return
	}
	b.loading = false
	b.close.based, b.close.basedErr = msg.got, msg.err
	if msg.err != nil {
		m.log.Info("based on failed", "err", msg.err.Error())
	}
	m.decideBulk()
}

// closeText is the text box of a close's comment as the shared popup layer
// reads it: write puts the focus in it, esc takes it out, and what is
// typed decides the popup again, as the comment changes what it does.
func (m *Model) closeText(f *closeForm) *popupText {
	return &popupText{
		focused: f.editing,
		focus:   f.focus,
		cursor:  f.ta.Cursor,
		blur: func() {
			f.editing = false
			f.ta.Blur()
			m.decideBulk()
		},
		edit: func(msg tea.Msg) tea.Cmd {
			var cmd tea.Cmd
			f.ta, cmd = f.ta.Update(msg)
			m.decideBulk()
			return cmd
		},
		editNav:    navActions,
		writeWords: "write a comment",
	}
}

// closePaste puts pasted text into the text box of a close, only while it
// has the focus.
func (m *Model) closePaste(msg tea.PasteMsg) tea.Cmd {
	f := m.bulk.close
	if f == nil || !f.editing {
		return nil
	}
	var cmd tea.Cmd
	f.ta, cmd = f.ta.Update(msg)
	m.decideBulk()
	return cmd
}

// closeWhat says what a close does to one pull request it acts on, as the
// pieces of a bulkRow's what.
func (m *Model) closeWhat(c plan.Close) []string {
	st := m.st
	var steps []string
	if m.bulk.close.body() != "" {
		steps = append(steps, "comment")
	}
	steps = append(steps, "close")
	if c.Delete {
		steps = append(steps, "delete branch")
	}
	for i, s := range steps {
		steps[i] = st.green.Render(s)
	}
	what := m.arrowed(steps)
	if c.Keep != "" {
		what = append(what, st.dim.Render("·")+" "+st.peach.Render("branch kept: "+c.Keep))
	}
	return what
}

// closeProse is what the user is told before saying yes: what can be
// taken back on GitHub and what cannot.
func (m *Model) closeProse() string {
	f := m.bulk.close
	s := "A closed pull request can be reopened."
	if f.del && slices.ContainsFunc(m.bulk.items, func(it bulkItem) bool { return it.goes() && it.closing.Delete }) {
		s += " A deleted branch can be restored from its pull request."
	}
	if f.body() != "" {
		s += " GitHub cannot take a comment back."
	}
	return s + " Each acts only on the head shown when it was read."
}

// closeFoot is the check and the text box of a close, kept under the list
// of pull requests however long it is, so both stay in sight.
func (m *Model) closeFoot(inner int) (lines []string, inputAt int) {
	st, f := m.st, m.bulk.close
	check := "[ ]"
	if f.del {
		check = "[x]"
	}
	line := st.mauve.Bold(true).Render(check) + " " + st.text.Render("delete the head branches")
	if !m.bulk.loading {
		n, can := 0, 0
		for _, it := range m.bulk.items {
			if !it.goes() {
				continue
			}
			n++
			if f.basedErr == nil && plan.KeepBranch(it.mi, f.based[it.mi.Head]) == "" {
				can++
			}
		}
		if n > 0 {
			line += st.muted.Render(fmt.Sprintf("  %d of %d can be deleted", can, n))
		}
	}
	label := st.muted.Render("comment") + st.dim.Render(" (optional)")
	if f.editing {
		label = st.blue.Bold(true).Render("comment") + st.dim.Render(" (optional)")
	}
	lines = []string{"", line, "", label}
	inputAt = len(lines)
	f.ta.SetWidth(inner)
	lines = append(lines, strings.Split(f.ta.View(), "\n")...)
	return lines, inputAt
}

// markClosed shows a pull request gh-desk closed as closed.
func (m *Model) markClosed(k model.PRKey) {
	if p, ok := m.knownPR(k); ok && p.State == model.StateOpen {
		p.State = model.StateClosed
		m.updatePR(p)
	}
}

// detailComments is how many comments the pane holds of a pull request,
// as the read of its detail asks GitHub for.
const detailComments = 20

// addSentComment shows a comment GitHub took in the pane without reading
// the pull request again (ADR-067). A read on its way may have left before
// the comment was taken, so it is thrown away and done again.
func (m *Model) addSentComment(k model.PRKey, body string) tea.Cmd {
	e, ok := m.cache.details[k]
	switch {
	case !ok || body == "":
		return nil
	case e.loading || e.err != nil:
		delete(m.cache.details, k)
		return m.ensure()
	}
	c := safeComment(model.Comment{Author: m.viewer, Body: body, Created: m.set.Now()})
	cs := append(slices.Clone(e.val.Comments), c)
	if len(cs) > detailComments {
		cs = cs[len(cs)-detailComments:]
	}
	e.val.Comments = cs
	return nil
}
