package tui

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/babarot/gh-triage/internal/claude"
	"github.com/babarot/gh-triage/internal/keymap"
	"github.com/babarot/gh-triage/internal/model"
	"github.com/babarot/gh-triage/internal/runner"
)

// Claude asks Claude Code about pull requests. The app package passes the
// claude package's types.
type Claude interface {
	// Prepare makes the worktree of the pull request at head.
	Prepare(ctx context.Context, key model.PRKey, head string) (string, error)
	Ask(ctx context.Context, r claude.Request, progress func(claude.Step)) (claude.Answer, error)
	// Interactive is Claude Code resuming a session, to run in the
	// terminal.
	Interactive(dir, session string) *exec.Cmd
	// Once asks with no tools and no session kept, in dir, or where the
	// implementation picks when dir is empty.
	Once(ctx context.Context, dir, prompt, schema string) (claude.Answer, error)
}

// chat is a conversation with Claude about one pull request. It stays
// when the screen is left, and goes on when it is opened again.
type chat struct {
	pr      model.PR
	session string
	// dir is the worktree, once made.
	dir string
	// started is set once Claude Code has the session.
	started bool
	turns   []turn
	// running is the question on its way, and steps what Claude did for
	// it so far.
	running bool
	steps   []claude.Step
	cancel  context.CancelFunc
	cost    float64
	model   string
	// scroll is how many lines up from the end the screen shows.
	scroll int
	typing bool
	input  textarea.Model
	back   view
}

// turn is a question or an answer.
type turn struct {
	you   bool
	text  string
	steps []claude.Step
	err   bool
}

// Messages of a question on its way.
type (
	chatStepMsg struct {
		key  model.PRKey
		step claude.Step
		ch   <-chan tea.Msg
	}
	chatPreparedMsg struct {
		key model.PRKey
		dir string
		ch  <-chan tea.Msg
	}
	chatAnswerMsg struct {
		key    model.PRKey
		answer claude.Answer
		err    error
		draft  bool
	}
)

// listenChat waits for the next message of a question.
func listenChat(ch <-chan tea.Msg) tea.Cmd {
	return func() tea.Msg {
		msg, ok := <-ch
		if !ok {
			return nil
		}
		return msg
	}
}

// openChat opens the conversation about p, starting it with a review when
// it is new.
func (m *Model) openChat(p model.PR) tea.Cmd {
	if m.claude == nil {
		m.setStatus("Claude is not available", true)
		return nil
	}
	back := m.view
	if back == viewChat {
		back = viewList
	}
	if c, ok := m.chats[p.Key]; ok {
		c.back = back
		m.chatKey, m.view = p.Key, viewChat
		return nil
	}
	ta := textarea.New()
	ta.Placeholder = "Ask Claude"
	ta.ShowLineNumbers = false
	ta.SetHeight(3)
	ta.SetWidth(max(m.width-4, 20))
	c := &chat{pr: p, session: claude.NewSession(), input: ta, back: back}
	m.chats[p.Key] = c
	m.chatKey, m.view = p.Key, viewChat
	return m.ask(c, "Review this pull request.", "", false)
}

func (m *Model) chat() *chat { return m.chats[m.chatKey] }

// ask sends a question; the first one carries the pull request, and makes
// its worktree first.
func (m *Model) ask(c *chat, shown, prompt string, draft bool) tea.Cmd {
	if c.running {
		m.setStatus("Claude is still answering", true)
		return nil
	}
	c.turns = append(c.turns, turn{you: true, text: shown})
	c.running, c.steps, c.scroll = true, nil, 0
	ctx, cancel := context.WithCancel(m.ctx)
	c.cancel = cancel
	ch := make(chan tea.Msg, 16)
	first, dir, key, pr, session := !c.started, c.dir, c.pr.Key, c.pr, c.session
	schema := ""
	if draft {
		schema = claude.DraftSchema
	}
	go func() {
		defer close(ch)
		defer cancel()
		send := func(msg tea.Msg) {
			select {
			case ch <- msg:
			case <-ctx.Done():
			}
		}
		if dir == "" {
			d, err := m.claude.Prepare(ctx, key, pr.HeadOID)
			if err != nil {
				send(chatAnswerMsg{key: key, err: fmt.Errorf("could not make the worktree: %w", err)})
				return
			}
			dir = d
			send(chatPreparedMsg{key: key, dir: d, ch: ch})
		}
		if first {
			d, err := m.src.Detail(ctx, key)
			if err != nil {
				send(chatAnswerMsg{key: key, err: fmt.Errorf("could not read the pull request: %w", err)})
				return
			}
			diff, err := m.src.Diff(ctx, pr)
			if err != nil {
				send(chatAnswerMsg{key: key, err: fmt.Errorf("could not read the diff: %w", err)})
				return
			}
			prompt = claude.ReviewPrompt(pr, d, diff)
		}
		a, err := m.claude.Ask(ctx, claude.Request{Dir: dir, Session: session, First: first, Prompt: prompt, Schema: schema},
			func(s claude.Step) { send(chatStepMsg{key: key, step: s, ch: ch}) })
		send(chatAnswerMsg{key: key, answer: a, err: err, draft: draft})
	}()
	return listenChat(ch)
}

func (m *Model) chatStep(msg chatStepMsg) tea.Cmd {
	if c, ok := m.chats[msg.key]; ok {
		c.steps = append(c.steps, msg.step)
	}
	return listenChat(msg.ch)
}

func (m *Model) chatPrepared(msg chatPreparedMsg) tea.Cmd {
	if c, ok := m.chats[msg.key]; ok {
		c.dir = msg.dir
	}
	return listenChat(msg.ch)
}

func (m *Model) chatAnswer(msg chatAnswerMsg) tea.Cmd {
	c, ok := m.chats[msg.key]
	if !ok {
		return nil
	}
	c.running, c.cancel = false, nil
	if msg.err != nil {
		if m.ctx.Err() == nil {
			c.turns = append(c.turns, turn{text: msg.err.Error(), steps: c.steps, err: true})
		}
		c.steps = nil
		return nil
	}
	// Claude Code keeps the session from the first answer on.
	c.started = true
	c.cost += msg.answer.Cost
	if msg.answer.Model != "" {
		c.model = msg.answer.Model
	}
	if msg.draft {
		d, err := claude.ParseDraft(msg.answer)
		if err != nil {
			c.turns = append(c.turns, turn{text: err.Error(), steps: c.steps, err: true})
			c.steps = nil
			return nil
		}
		c.turns = append(c.turns, turn{text: "Drafted a review (" + kindWords[d.Event] + ").", steps: c.steps})
		c.steps = nil
		if m.view != viewChat || m.chatKey != msg.key {
			m.setStatus(fmt.Sprintf("the review of %s is drafted; open Claude to see it", msg.key), false)
			return nil
		}
		return m.draftCompose(c, d)
	}
	c.turns = append(c.turns, turn{text: msg.answer.Text, steps: c.steps})
	c.steps = nil
	if m.view != viewChat || m.chatKey != msg.key {
		m.setStatus(fmt.Sprintf("Claude answered on %s", msg.key), false)
	}
	return nil
}

// draftCompose opens the review screen with Claude's draft.
func (m *Model) draftCompose(c *chat, d claude.Draft) tea.Cmd {
	ev := d.Event
	if m.own(c.pr) || c.pr.State != model.StateOpen {
		ev = model.EventComment
	}
	cmd := m.startCompose(c.pr, ev)
	if m.compose != nil {
		m.compose.ta.SetValue(d.Body)
	}
	return cmd
}

func (m *Model) chatKeyPress(msg tea.KeyPressMsg) tea.Cmd {
	c := m.chat()
	k := msg.String()
	if c == nil {
		m.view = viewList
		return nil
	}
	if c.typing {
		switch {
		case k == keymap.Back:
			c.typing = false
			c.input.Blur()
			return nil
		case m.keys.Matches("accept", k):
			q := strings.TrimSpace(c.input.Value())
			if q == "" {
				return nil
			}
			if c.running {
				m.setStatus("Claude is still answering", true)
				return nil
			}
			c.input.Reset()
			c.typing = false
			c.input.Blur()
			return m.ask(c, q, q, false)
		}
		var cmd tea.Cmd
		c.input, cmd = c.input.Update(msg)
		return cmd
	}
	is := func(a keymap.Action) bool { return m.keys.Matches(a, k) }
	switch {
	case k == keymap.Back, is("quit"):
		m.view = c.back
		if c.back == viewList {
			return m.ensure()
		}
	case is("down"):
		c.scroll = max(0, c.scroll-1)
	case is("up"):
		c.scroll++
	case is("page_down"):
		c.scroll = max(0, c.scroll-m.height/2)
	case is("page_up"):
		c.scroll += m.height / 2
	case is("bottom"):
		c.scroll = 0
	case is("top"):
		c.scroll = 1 << 30
	case is("ask_more"):
		c.typing = true
		c.input.SetWidth(max(m.width-4, 20))
		return c.input.Focus()
	case is("draft_review"):
		return m.ask(c, "Draft the review.", claude.DraftPrompt, true)
	case is("chat_approve"):
		if m.own(c.pr) {
			m.setStatus("you cannot approve your own pull request", true)
			return nil
		}
		if m.busy() {
			return nil
		}
		return m.startBulkFor(runner.Approve, []model.PR{c.pr})
	case is("open_claude"):
		switch {
		case c.running:
			m.setStatus("wait for Claude to answer first", true)
		case !c.started:
			m.setStatus("the conversation has not started", true)
		default:
			return tea.ExecProcess(m.claude.Interactive(c.dir, c.session), func(err error) tea.Msg { return claudeExitMsg{err} })
		}
	}
	return nil
}

// claudeExitMsg is Claude Code's exit after O.
type claudeExitMsg struct{ err error }

func (m *Model) claudeExited(msg claudeExitMsg) tea.Cmd {
	if msg.err != nil {
		m.setStatus("Claude Code: "+msg.err.Error(), true)
		return nil
	}
	m.setStatus("back from Claude Code; what was said there is not shown here", false)
	return nil
}

// chatLines are the conversation, wrapped to width.
func (m *Model) chatLines(c *chat, width int) []string {
	st := m.st
	wrap := func(s string) []string {
		var out []string
		for _, l := range strings.Split(ansi.Wrap(safe(s), max(width-2, 10), " "), "\n") {
			out = append(out, "  "+l)
		}
		return out
	}
	steps := func(ss []claude.Step) []string {
		var out []string
		for _, s := range ss {
			out = append(out, "  "+st.mauve.Render("⏺ "+s.Tool)+" "+st.dim.Render(safeLine(s.Arg)))
		}
		return out
	}
	var L []string
	for _, t := range c.turns {
		switch {
		case t.you:
			L = append(L, st.blue.Render("you")+"  "+st.sub1.Render(safeLine(firstLine(t.text))))
		case t.err:
			L = append(L, steps(t.steps)...)
			L = append(L, st.red.Render("claude failed"))
			for _, l := range wrap(t.text) {
				L = append(L, st.red.Render(l))
			}
		default:
			L = append(L, steps(t.steps)...)
			L = append(L, st.mauve.Render("claude"))
			for _, l := range wrap(t.text) {
				L = append(L, st.sub1.Render(l))
			}
		}
		L = append(L, "")
	}
	if c.running {
		L = append(L, steps(c.steps)...)
		what := "thinking…"
		if c.dir == "" {
			what = "making the worktree…"
		}
		L = append(L, "  "+st.muted.Render(what))
	}
	return L
}

func firstLine(s string) string {
	l, _, _ := strings.Cut(s, "\n")
	return l
}

func (m *Model) renderChat(b *strings.Builder, height int) {
	c, st := m.chat(), m.st
	if c == nil {
		return
	}
	info := []string{"at " + short(c.pr.HeadOID), "read-only"}
	if c.model != "" {
		info = append([]string{c.model}, info...)
	}
	if c.cost > 0 {
		info = append(info, fmt.Sprintf("$%.2f", c.cost))
	}
	b.WriteString(fit(st.mauve.Render("claude")+" "+st.muted.Render("on")+" "+st.sub1.Render(c.pr.Key.String())+"  "+
		st.dim.Render(strings.Join(info, " · ")), m.width) + "\n")
	b.WriteString(st.dim.Render(strings.Repeat("╌", max(m.width-1, 0))) + "\n")
	inputH := 1
	if c.typing {
		inputH = c.input.Height()
	}
	bodyH := max(height-2-inputH, 1)
	L := m.chatLines(c, m.width)
	c.scroll = min(c.scroll, max(len(L)-bodyH, 0))
	start := max(len(L)-bodyH-c.scroll, 0)
	for i := 0; i < bodyH; i++ {
		if start+i < len(L) {
			b.WriteString(fit(L[start+i], m.width))
		}
		b.WriteString("\n")
	}
	if c.typing {
		b.WriteString(c.input.View() + "\n")
		return
	}
	b.WriteString(fit(st.dim.Render(fmt.Sprintf("› %s to ask more · %s to draft a review", m.firstKey("ask_more"), m.firstKey("draft_review"))), m.width) + "\n")
}
