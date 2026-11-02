package tui

import (
	"context"
	"fmt"
	"os/exec"
	"slices"
	"strings"
	"time"

	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/babarot/gh-triage/internal/claude"
	"github.com/babarot/gh-triage/internal/keymap"
	"github.com/babarot/gh-triage/internal/logging"
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
	// Triage asks for a plan of what to show and select in the lists, in
	// a session kept where the implementation picks.
	Triage(ctx context.Context, r claude.Request) (claude.Answer, error)
}

// chat is a conversation with Claude about one pull request. It stays
// when the screen is left, and goes on when it is opened again; when kept
// (ADR-031), also after gh-triage quits and in another gh-triage.
type chat struct {
	pr model.PR
	// session is made when a question starts it, as an ID Claude Code
	// may have taken is not used again.
	session string
	// dir is the worktree, once made.
	dir string
	// started is set once Claude Code has the session.
	started bool
	// head is the head of the pull request at the last question.
	head  string
	turns []turn
	// running is the question on its way, and steps what Claude did for
	// it so far.
	running bool
	steps   []claude.Step
	cancel  context.CancelFunc
	// since is when the question on its way was asked.
	since time.Time
	// unseen is set when an answer came while the screen was elsewhere,
	// until the conversation is opened.
	unseen bool
	cost   float64
	model  string
	// scroll is how many lines up from the end the screen shows.
	scroll int
	// draft is a review drafted while the screen was elsewhere, opened
	// when it is shown again.
	draft  *claude.Draft
	typing bool
	input  textarea.Model
	back   view

	// persist is set while the conversation is kept; a write that failed
	// clears it for this run. rev is the rev of the last write that
	// succeeded, 0 when none has.
	persist bool
	rev     int
	// lock releases the conversation's lock, held while Claude Code works
	// on its session: a question on its way, or Claude Code in the
	// terminal.
	lock func()
	// elsewhere names the gh-triage that holds the lock, when another one
	// does, as last found at probed.
	elsewhere string
	probed    time.Time
	// restart is set by a first press of chat_restart, until the next key.
	restart bool
}

// turn is a question or an answer.
type turn struct {
	you   bool
	text  string
	steps []claude.Step
	err   bool
	// head is the head of the pull request a question was asked at.
	head string
	at   time.Time
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
		// session is a new session, when the one kept was started in a
		// worktree that is no longer where worktrees are made.
		session string
		ch      <-chan tea.Msg
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

// openChat opens the conversation about p: the one in this gh-triage, or
// the one kept, or a new one started with a review.
func (m *Model) openChat(p model.PR) tea.Cmd {
	if m.claude == nil {
		m.refuse("Claude is not available")
		return nil
	}
	back := m.view
	if back == viewChat {
		back = viewList
	}
	c, ok := m.chats[p.Key]
	if !ok {
		ta := newInput()
		ta.Placeholder = "Ask Claude"
		ta.ShowLineNumbers = false
		ta.SetHeight(3)
		ta.SetWidth(max(m.width-4, 20))
		c = &chat{input: ta, persist: m.set.Chats != nil}
		m.chats[p.Key] = c
	}
	// The pull request as the list has it now, whose head may have moved.
	c.pr, c.back, c.unseen = p, back, false
	m.chatKey, m.view = p.Key, viewChat
	// Probed before reading: a lock found free means what is read is
	// what the other gh-triage left.
	m.probeChat(c)
	m.syncChat(c)
	if len(c.turns) == 0 && !c.running && c.elsewhere == "" {
		return m.ask(c, "Review this pull request.", "", false)
	}
	if d := c.draft; d != nil {
		c.draft = nil
		return m.draftCompose(c, *d)
	}
	return nil
}

func (m *Model) chat() *chat { return m.chats[m.chatKey] }

// ask sends a question under the conversation's lock; the first one
// carries the pull request, and one asked after the head moved says so.
// The worktree is moved to the head shown before each question, as
// another gh-triage or gh triage gc may have moved or removed it.
func (m *Model) ask(c *chat, shown, prompt string, draft bool) tea.Cmd {
	if c.running {
		m.refuse("Claude is still answering")
		return nil
	}
	if !m.lockChat(c) {
		return nil
	}
	m.reloadChat(c)
	if !c.started {
		c.session = claude.NewSession()
	}
	first, dir, key, pr, session := !c.started, c.dir, c.pr.Key, c.pr, c.session
	moved := ""
	if c.started && c.head != "" && c.head != pr.HeadOID {
		moved = c.head
	}
	c.turns = append(c.turns, turn{you: true, text: shown, head: pr.HeadOID, at: m.set.Now()})
	c.head = pr.HeadOID
	c.running, c.steps, c.scroll, c.since = true, nil, 0, m.set.Now()
	// Kept before it is answered, so a question cut short by quitting is
	// still there.
	m.saveChat(c)
	ctx, cancel := context.WithCancel(m.ctx)
	c.cancel = cancel
	ch := make(chan tea.Msg, 16)
	schema := ""
	if draft {
		schema = claude.DraftSchema
	}
	m.asking.Add(1)
	go func() {
		defer m.asking.Done()
		defer close(ch)
		defer cancel()
		send := func(msg tea.Msg) {
			select {
			case ch <- msg:
			case <-ctx.Done():
			}
		}
		d, err := m.claude.Prepare(ctx, key, pr.HeadOID)
		if err != nil {
			send(chatAnswerMsg{key: key, err: fmt.Errorf("could not make the worktree: %w", err)})
			return
		}
		renewed := false
		if !first && dir != "" && d != dir {
			// Claude Code resumes a session only where it started.
			first, session, moved, renewed = true, claude.NewSession(), "", true
		}
		if d != dir {
			msg := chatPreparedMsg{key: key, dir: d, ch: ch}
			if renewed {
				msg.session = session
			}
			send(msg)
		}
		dir = d
		switch {
		case first:
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
			// What was asked goes after the review a new session begins
			// with.
			if prompt != "" {
				prompt = claude.ReviewPrompt(pr, d, diff) + "\n" + prompt
			} else {
				prompt = claude.ReviewPrompt(pr, d, diff)
			}
		case moved != "":
			diff, err := m.src.Diff(ctx, pr)
			if err != nil {
				send(chatAnswerMsg{key: key, err: fmt.Errorf("could not read the diff: %w", err)})
				return
			}
			prompt = claude.HeadMovedPrompt(moved, pr.HeadOID, diff) + prompt
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
		if msg.session != "" {
			c.session, c.started = msg.session, false
			m.say("the session kept was started in another cache; this question starts a new one")
		}
	}
	return listenChat(msg.ch)
}

func (m *Model) chatAnswer(msg chatAnswerMsg) tea.Cmd {
	c, ok := m.chats[msg.key]
	if !ok {
		return nil
	}
	c.running, c.cancel = false, nil
	c.unseen = m.view != viewChat || m.chatKey != msg.key
	add := func(text string, err bool) {
		c.turns = append(c.turns, turn{text: text, steps: c.steps, err: err, at: m.set.Now()})
		c.steps = nil
	}
	// Kept with the answer, then the session is free for another
	// gh-triage.
	defer func() {
		m.saveChat(c)
		c.unlockChat()
	}()
	if msg.err != nil {
		// A session not started is made anew at the next question, as
		// Claude Code may have taken its ID before failing.
		if m.ctx.Err() == nil {
			add(msg.err.Error(), true)
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
			add(err.Error(), true)
			return nil
		}
		add("Drafted a review ("+kindWords[d.Event]+").", false)
		if m.view != viewChat || m.chatKey != msg.key {
			c.draft = &d
			m.say(fmt.Sprintf("the review of %s is drafted; open Claude to see it", msg.key))
			return nil
		}
		return m.draftCompose(c, d)
	}
	add(msg.answer.Text, false)
	if m.view != viewChat || m.chatKey != msg.key {
		m.say(fmt.Sprintf("Claude answered on %s", msg.key))
	}
	return nil
}

// draftCompose opens the review popup with Claude's draft.
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
				m.refuse("Claude is still answering")
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
	armed := c.restart
	c.restart = false
	switch {
	case is("chat_restart"):
		switch {
		case c.running:
			m.refuse("wait for Claude to answer first")
		case !armed:
			c.restart = true
			m.say(fmt.Sprintf("press %s again to start over: the conversation goes from gh-triage, and Claude Code keeps its session", m.firstKey("chat_restart")))
		default:
			return m.restartChat(c)
		}
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
	case is("chat_more"):
		c.typing = true
		c.input.SetWidth(max(m.width-4, 20))
		return c.input.Focus()
	case is("draft_review"):
		return m.ask(c, "Draft the review.", claude.DraftPrompt, true)
	case is("chat_approve"):
		if m.own(c.pr) {
			m.refuse("you cannot approve your own pull request")
			return nil
		}
		if m.busy() {
			return nil
		}
		return m.startBulkFor(runner.Approve, []model.PR{c.pr})
	case is("open_claude"):
		switch {
		case c.running:
			m.refuse("wait for Claude to answer first")
		case !m.lockChat(c):
		default:
			m.reloadChat(c)
			if !c.started {
				c.unlockChat()
				m.refuse("the conversation has not started")
				return nil
			}
			// Held while Claude Code has the session, so no other
			// gh-triage resumes it meanwhile.
			key := c.pr.Key
			return tea.ExecProcess(m.claude.Interactive(c.dir, c.session), func(err error) tea.Msg { return claudeExitMsg{key: key, err: err} })
		}
	}
	return nil
}

// claudeExitMsg is Claude Code's exit after O.
type claudeExitMsg struct {
	key model.PRKey
	err error
}

func (m *Model) claudeExited(msg claudeExitMsg) tea.Cmd {
	if c, ok := m.chats[msg.key]; ok {
		c.unlockChat()
	}
	if msg.err != nil {
		m.fail("Claude Code: " + msg.err.Error())
		return nil
	}
	m.say("back from Claude Code; what was said there is not shown here")
	return nil
}

// restartChat starts the conversation over, under its lock, which the
// first question takes on. What is kept is replaced rather than removed,
// so its rev goes on rising and another gh-triage sees its own as older;
// Claude Code keeps the old session.
func (m *Model) restartChat(c *chat) tea.Cmd {
	if !m.lockChat(c) {
		return nil
	}
	m.reloadChat(c)
	rev := c.rev
	c.load(ChatRecord{Key: c.pr.Key, Rev: rev})
	c.draft, c.scroll = nil, 0
	m.saveChat(c)
	m.log.Info("conversation started over", logging.KeyPR, c.pr.Key.String())
	return m.ask(c, "Review this pull request.", "", false)
}

// chatLines are the conversation, wrapped to width, with Claude's answers
// drawn from their Markdown.
func (m *Model) chatLines(c *chat, width int) []string {
	st := m.st
	wrap := func(s string) []string {
		var out []string
		for l := range strings.SplitSeq(ansi.Wrap(safe(s), max(width-2, 10), " "), "\n") {
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
	head := ""
	for i, t := range c.turns {
		switch {
		case t.you:
			if head != "" && t.head != "" && t.head != head {
				L = append(L, st.dim.Render("── head moved "+short(head)+" → "+short(t.head)+" ──"), "")
			}
			if t.head != "" {
				head = t.head
			}
			L = append(L, st.blue.Render("you")+"  "+st.sub1.Render(safeLine(firstLine(t.text))))
			if i == len(c.turns)-1 && !c.running {
				// A question with no answer is on its way in another
				// gh-triage, or was cut short when one quit.
				if c.elsewhere != "" {
					L = append(L, "", "  "+m.working(st.muted, "answering in another gh-triage ("+c.elsewhere+")…"))
				} else {
					L = append(L, "", "  "+st.dim.Render("interrupted: gh-triage was closed before the answer"))
				}
			}
		case t.err:
			L = append(L, steps(t.steps)...)
			L = append(L, st.red.Render("claude failed"))
			for _, l := range wrap(t.text) {
				L = append(L, st.red.Render(l))
			}
		default:
			L = append(L, steps(t.steps)...)
			L = append(L, st.mauve.Render("claude"))
			// An answer is Markdown, drawn as a description is.
			text := zeroWidth.Replace(mdLinkRe.ReplaceAllString(safe(t.text), "$1"))
			for _, l := range m.markdownLines(strings.TrimRight(text, " \t\n"), max(width-2, 10)) {
				if l.text == "" {
					L = append(L, "")
					continue
				}
				L = append(L, "  "+l.text)
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
		L = append(L, "  "+m.working(st.muted, what)+" "+st.dim.Render(elapsed(m.set.Now().Sub(c.since))))
	}
	return L
}

// elapsed is how long a question has waited, as the chat shows it.
func elapsed(d time.Duration) string {
	s := int(d.Seconds())
	if s < 60 {
		return fmt.Sprintf("%ds", s)
	}
	return fmt.Sprintf("%dm%02ds", s/60, s%60)
}

// claudeActivity is Claude answering about a pull request whose
// conversation is not shown.
func (m *Model) claudeActivity() string {
	var away []model.PRKey
	for k, c := range m.chats {
		if c.running && (m.view != viewChat || m.chatKey != k) {
			away = append(away, k)
		}
	}
	if len(away) == 0 {
		return ""
	}
	slices.SortFunc(away, func(a, b model.PRKey) int { return strings.Compare(a.String(), b.String()) })
	what := "Claude is answering on " + away[0].String()
	if n := len(away) - 1; n > 0 {
		what += fmt.Sprintf(" and %d more", n)
	}
	return what + "…"
}

func firstLine(s string) string {
	l, _, _ := strings.Cut(s, "\n")
	return l
}

// renderChat draws the conversation and returns where the cursor of the
// text box is on the screen, nil when the box is not shown.
func (m *Model) renderChat(b *strings.Builder, height int) *tea.Cursor {
	c, st := m.chat(), m.st
	if c == nil {
		return nil
	}
	at := "at " + short(c.pr.HeadOID)
	if c.head != "" && c.head != c.pr.HeadOID {
		at += " (was " + short(c.head) + ")"
	}
	info := []string{at, "read-only"}
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
	for i := range bodyH {
		if start+i < len(L) {
			b.WriteString(fit(L[start+i], m.width))
		}
		b.WriteString("\n")
	}
	if c.typing {
		cur := c.input.Cursor()
		if cur != nil {
			cur.Y += strings.Count(b.String(), "\n")
		}
		b.WriteString(c.input.View() + "\n")
		return cur
	}
	b.WriteString(fit(st.dim.Render(fmt.Sprintf("› %s to ask more · %s to draft a review", m.firstKey("chat_more"), m.firstKey("draft_review"))), m.width) + "\n")
	return nil
}
