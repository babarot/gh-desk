package tui

import (
	"fmt"
	"slices"
	"strings"

	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

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

// triageSearchRows is how many questions a search shows in the popup.
const triageSearchRows = 8

// search is a search of the questions, which takes the place of the text box
// and has a text box of its own, so an input method works in it.
type search struct {
	input textarea.Model
	hits  askbox.Search
}

// edit gives msg to the search's text box; a text changed starts from the
// first hit.
func (s *search) edit(msg tea.Msg) tea.Cmd {
	before := s.input.Value()
	var cmd tea.Cmd
	s.input, cmd = s.input.Update(msg)
	if s.input.Value() != before {
		s.hits.Reset()
	}
	return cmd
}

// openSearch opens a search of list in place of the text box in; with no
// questions there is nothing to search.
func (m *Model) openSearch(sp **search, in *textarea.Model, list []askbox.Entry) tea.Cmd {
	if len(list) == 0 {
		m.refuse("nothing has been asked yet")
		return nil
	}
	ta := newInput(m.st)
	ta.Placeholder = "Search the questions"
	ta.ShowLineNumbers = false
	ta.SetHeight(1)
	ta.SetWidth(in.Width())
	in.Blur()
	*sp = &search{input: ta}
	return (*sp).input.Focus()
}

// searchKey handles a key while the search of list is open. accept puts the
// question in the text box in, and sends nothing.
func (m *Model) searchKey(sp **search, in *textarea.Model, r *askbox.Recall, list []askbox.Entry, msg tea.KeyPressMsg) tea.Cmd {
	s, k := *sp, msg.String()
	is := func(a keymap.Action) bool { return m.keys.Matches(a, k) }
	hits := askbox.Hits(list, s.input.Value())
	switch {
	case k == keymap.Back:
	case is("accept"):
		if len(hits) > 0 {
			in.SetValue(hits[s.hits.At(len(hits))].Question)
			r.Leave()
		}
	case is("history_search"), is("history_next"), is("down") && msg.Text == "":
		s.hits.Next(len(hits))
		return nil
	case is("history_prev"), is("up") && msg.Text == "":
		s.hits.Prev(len(hits))
		return nil
	default:
		return s.edit(msg)
	}
	*sp = nil
	return in.Focus()
}

// searchLines are the hits of s among list, at most rows of them, the one
// chosen marked, and the first line of its answer, in width.
func (m *Model) searchLines(s *search, list []askbox.Entry, width, rows int) []string {
	st := m.st
	q := s.input.Value()
	hits := askbox.Hits(list, q)
	if len(hits) == 0 {
		return []string{st.dim.Render("no question holds that")}
	}
	at := s.hits.At(len(hits))
	from, to := askbox.Shown(at, len(hits), rows)
	var out []string
	for i := from; i < to; i++ {
		e := hits[i]
		mark, text := "  ", st.dim
		if i == at {
			mark, text = st.mauve.Render("›")+" ", st.sub1
		}
		age := ""
		if !e.At.IsZero() {
			age = shortAge(m.set.Now().Sub(e.At))
		}
		lead := mark + st.dim.Render(fit(age, 5))
		if e.Tab != "" {
			lead += st.muted.Render(fit(safeLine(e.Tab), 10))
		}
		out = append(out, lead+m.markHit(safeLine(e.Question), q, text))
	}
	reply := hits[at].Reply
	if reply == "" {
		reply = "no answer kept"
	}
	return append(out, st.dim.Render("⏺ "+safeLine(reply)))
}

// markHit is text in style, with what holds q in the search's color. It looks
// by rune, as lower case keeps the runes of a text but not always its bytes.
func (m *Model) markHit(text, q string, style lipgloss.Style) string {
	rs, low, want := []rune(text), []rune(strings.ToLower(text)), []rune(strings.ToLower(q))
	if len(want) == 0 || len(low) != len(rs) {
		return style.Render(text)
	}
	for i := 0; i+len(want) <= len(low); i++ {
		if slices.Equal(low[i:i+len(want)], want) {
			end := i + len(want)
			return style.Render(string(rs[:i])) + m.st.peach.Render(string(rs[i:end])) + style.Render(string(rs[end:]))
		}
	}
	return style.Render(text)
}

// searchRows is how many hits fit in rows lines of a search's box, beside its
// text box and the line of the answer.
func searchRows(lines, most int) int { return max(min(most, lines-2), 1) }

// askGhost is what would complete the text box in, shown after its cursor:
// nothing while a question is recalled, or with the cursor not at the end,
// where it would cover what is typed.
func askGhost(in *textarea.Model, r *askbox.Recall, try []string, list []askbox.Entry) string {
	v := in.Value()
	if r.Active() || v == "" || in.Line() != in.LineCount()-1 {
		return ""
	}
	last := v[strings.LastIndexByte(v, '\n')+1:]
	if in.Column() != len([]rune(last)) {
		return ""
	}
	return askbox.Ghost(v, try, list)
}

// completeAsk fills in the text box: a suggestion, the next one while it
// holds one, or else what its ghost offers.
func completeAsk(r *askbox.Recall, in *textarea.Model, try []string, list []askbox.Entry) {
	v := in.Value()
	s := nextSuggestion(try, v)
	if g := askGhost(in, r, try, list); s == "" && g != "" {
		s = v + g
	}
	if s != "" {
		in.SetValue(s)
		r.Leave()
	}
}

// askLines are the lines of the text box in, with ghost faint after its
// cursor, cut to the box. The cursor stays where the text ends, for an input
// method to show the text being composed there.
func (m *Model) askLines(in *textarea.Model, ghost string) []string {
	lines := strings.Split(in.View(), "\n")
	cur := in.Cursor()
	if ghost == "" || cur == nil || cur.Y >= len(lines) {
		return lines
	}
	line := lines[cur.Y]
	w := ansi.StringWidth(line)
	if cur.X >= w {
		return lines
	}
	bg := in.Styles().Focused.CursorLine.GetBackground()
	g := ansi.Truncate(ghost, w-cur.X, "")
	lines[cur.Y] = ansi.Cut(line, 0, cur.X) + m.st.dim.Background(bg).Render(g) + ansi.Cut(line, cur.X+ansi.StringWidth(g), w)
	return lines
}
