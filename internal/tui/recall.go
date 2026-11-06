package tui

import (
	"fmt"
	"slices"
	"strings"

	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"

	"github.com/babarot/gh-desk/internal/claude"
	"github.com/babarot/gh-desk/internal/logging"
	"github.com/babarot/gh-desk/internal/tui/askbox"
	"github.com/babarot/gh-desk/internal/tui/keymap"
)

// historyFailedMsg says a question could not be kept. The question was still
// sent, so this only tells.
type historyFailedMsg struct{ err error }

// maxKeptReply is how many characters of an answer's first line are kept.
const maxKeptReply = 200

// historyMsg is the questions kept, as read.
type historyMsg struct {
	entries []askbox.Entry
	err     error
}

// loadHistory reads the questions kept, away from the screen, as it may wait
// for another gh-desk.
func (m *Model) loadHistory() tea.Cmd {
	s := m.set.History
	if s == nil {
		return nil
	}
	return func() tea.Msg {
		es, err := s.Load()
		return historyMsg{entries: es, err: err}
	}
}

// historyLoaded takes in the questions read, after those asked meanwhile. A
// failure is told; the history then holds only those.
func (m *Model) historyLoaded(msg historyMsg) {
	if msg.err != nil {
		m.historyFailed("read", msg.err)
		return
	}
	tr := m.triage
	list := msg.entries
	for _, e := range slices.Backward(tr.history) {
		list = askbox.Prepend(list, e)
	}
	tr.history = list
}

// historyFailed tells that the questions could not be read or kept.
func (m *Model) historyFailed(what string, err error) {
	m.log.Warn("history not "+what, logging.KeyOp, "triage", "err", err.Error())
	text := "the questions could not be " + what + ": " + err.Error()
	m.fail(text)
	m.noted = append(m.noted, m.pushNote("✗", text, nil))
}

// keepQuestion adds the request to the history and keeps it.
func (m *Model) keepQuestion(request string) tea.Cmd {
	tr := m.triage
	e := askbox.Entry{ID: claude.NewSession(), At: m.set.Now(), Tab: m.tab().name, Question: request}
	tr.history = askbox.Prepend(tr.history, e)
	tr.askedID = e.ID
	kept := make(chan struct{})
	tr.kept = kept
	return m.keepHistory(func(s askbox.Store) error {
		defer close(kept)
		return s.Add(e)
	})
}

// keepReply keeps the first line of the reply to the last question.
func (m *Model) keepReply(reply string) tea.Cmd {
	tr, line := m.triage, keptReply(reply)
	if line == "" {
		return nil
	}
	id := tr.askedID
	for i := range tr.history {
		if tr.history[i].ID == id {
			tr.history[i].Reply = line
		}
	}
	// After its question: a line of answer with no question yet would go
	// when another gh-desk reads the history.
	kept := tr.kept
	return m.keepHistory(func(s askbox.Store) error {
		<-kept
		return s.Reply(id, line)
	})
}

// keepHistory writes to the store away from the screen, as it may wait for
// another gh-desk.
func (m *Model) keepHistory(write func(askbox.Store) error) tea.Cmd {
	s := m.set.History
	if s == nil {
		return nil
	}
	return func() tea.Msg {
		if err := write(s); err != nil {
			return historyFailedMsg{err}
		}
		return nil
	}
}

// keptReply is the first line of an answer, without its Markdown marks, as
// it is kept.
func keptReply(reply string) string {
	r := []rune(strings.NewReplacer("**", "", "`", "").Replace(firstLine(reply)))
	return strings.TrimSpace(string(r[:min(len(r), maxKeptReply)]))
}

// recallKey takes a key that goes back through the questions list, the newest
// first, and reports whether it did. The arrows are nav's up and down, whose
// other keys are letters a text box types; they recall only on the box's
// first or last row, where they would have nowhere to move the cursor.
func (m *Model) recallKey(r *askbox.Recall, in *textarea.Model, list []askbox.Entry, msg tea.KeyPressMsg) bool {
	k := msg.String()
	is := func(a keymap.Action) bool { return m.keys.Matches(a, k) }
	info := in.LineInfo()
	switch {
	case is("history_prev"), is("up") && msg.Text == "":
		if in.Line() != 0 || info.RowOffset != 0 {
			return false
		}
		s, ok := r.Older(list, in.Value())
		if ok {
			in.SetValue(s)
		}
		return ok
	case is("history_next"), is("down") && msg.Text == "":
		if !r.Active() || in.Line() != in.LineCount()-1 || info.RowOffset != info.Height-1 {
			return false
		}
		in.SetValue(r.Newer(list))
		return true
	}
	return false
}

// editAsk gives msg to the text box; text changed by hand is no longer a
// question recalled.
func editAsk(r *askbox.Recall, in *textarea.Model, msg tea.Msg) tea.Cmd {
	before := in.Value()
	var cmd tea.Cmd
	*in, cmd = in.Update(msg)
	if in.Value() != before {
		r.Leave()
	}
	return cmd
}

// recallHint tells the keys of the questions list, and where in them the text
// box is; empty when there are none.
func (m *Model) recallHint(r *askbox.Recall, list []askbox.Entry) string {
	if len(list) == 0 {
		return ""
	}
	st := m.st
	if !r.Active() {
		return st.key.Render(m.firstKey("up")) + " " + st.muted.Render(fmt.Sprintf("history %d", len(list)))
	}
	e := list[min(r.At(), len(list)-1)]
	out := st.key.Render(m.firstKey("up")+"/"+m.firstKey("down")) + " " + st.muted.Render(fmt.Sprintf("history %d/%d", r.At()+1, len(list)))
	if !e.At.IsZero() {
		out += st.dim.Render(" · " + shortAge(m.set.Now().Sub(e.At)))
	}
	if e.Tab != "" {
		out += st.dim.Render(" · " + e.Tab)
	}
	return out
}
