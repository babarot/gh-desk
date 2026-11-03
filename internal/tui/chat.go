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
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/babarot/gh-triage/internal/claude"
	"github.com/babarot/gh-triage/internal/model"
	"github.com/babarot/gh-triage/internal/runner"
	"github.com/babarot/gh-triage/internal/tui/chat"
	"github.com/babarot/gh-triage/internal/tui/keymap"
)

// Claude asks Claude Code about pull requests. The app package passes the
// claude package's types.
type Claude interface {
	// Prepare makes the worktree of the pull request at head.
	Prepare(ctx context.Context, key model.PRKey, head string) (string, error)
	Ask(ctx context.Context, r claude.Request, progress func(claude.Step)) (claude.Answer, error)
	// Interactive is Claude Code resuming a session, to run in the
	// terminal.
	Interactive(pr model.PRKey, dir, session string) *exec.Cmd
	// Once asks with no tools and no session kept, in dir, or where the
	// implementation picks when dir is empty.
	Once(ctx context.Context, dir, prompt, schema string) (claude.Answer, error)
	// Triage asks for a plan of what to show and select in the lists, in
	// a session kept where the implementation picks.
	Triage(ctx context.Context, r claude.Request) (claude.Answer, error)
}

// chatScreen is a conversation with Claude as the screen shows it: the
// conversation, which internal/tui/chat holds and changes, and what is
// the screen's own.
type chatScreen struct {
	*chat.Conversation
	// unseen is set when an answer came while the screen was elsewhere,
	// until the conversation is opened.
	unseen bool
	// scroll is how many lines up from the end the screen shows.
	scroll int
	// draft is a review drafted while the screen was elsewhere, opened
	// when it is shown again.
	draft  *claude.Draft
	typing bool
	input  textarea.Model
	back   view
	// restart is set by a first press of chat_restart, until the next key.
	restart bool
}

// chatStatus puts what a conversation says went wrong on the status line.
type chatStatus struct{ m *Model }

func (s chatStatus) Fail(text string)   { s.m.fail(text) }
func (s chatStatus) Refuse(text string) { s.m.refuse(text) }

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
		ta.SetWidth(chatInputWidth(m.width))
		c = &chatScreen{Conversation: m.chatState.New(p), input: ta}
		m.chats[p.Key] = c
	}
	// The pull request as the list has it now, whose head may have moved.
	c.SetPR(p)
	c.back, c.unseen = back, false
	m.chatKey, m.view = p.Key, viewChat
	// Probed before reading: a lock found free means what is read is
	// what the other gh-triage left.
	m.chatState.Probe(c.Conversation)
	m.chatState.Sync(c.Conversation)
	if len(c.Turns()) == 0 && !c.Running() && c.Elsewhere() == "" {
		return m.ask(c, "Review this pull request.", "", false)
	}
	if d := c.draft; d != nil {
		c.draft = nil
		return m.draftCompose(c, *d)
	}
	return nil
}

// chat is the conversation on screen, nil when none was opened.
func (m *Model) chat() *chatScreen { return m.chats[m.chatKey] }

// watchEvery is how often a conversation on screen is read again while
// another gh-triage works on it. A constant: reading is a stat and a small
// file.
const watchEvery = time.Second

// watchChat reads the conversation on screen again while another
// gh-triage works on it, as the spinner turns, so its answer shows when
// it comes. It only reads; nothing is written on a timer.
func (m *Model) watchChat() {
	c := m.chat()
	if m.view != viewChat || c == nil || c.Elsewhere() == "" || m.set.Now().Sub(c.Probed()) < watchEvery {
		return
	}
	// Probed first: found free, the reading after it has the answer.
	m.chatState.Probe(c.Conversation)
	m.chatState.Sync(c.Conversation)
}

// ask sends a question under the conversation's lock; the first one
// carries the pull request, and one asked after the head moved says so.
// The worktree is moved to the head shown before each question, as
// another gh-triage or gh triage gc may have moved or removed it.
func (m *Model) ask(c *chatScreen, shown, prompt string, draft bool) tea.Cmd {
	moved, ok := m.chatState.Begin(c.Conversation, shown)
	if !ok {
		return nil
	}
	c.scroll = 0
	first, dir, key, pr, session := !c.Started(), c.Dir(), c.PR().Key, c.PR(), c.Session()
	ctx, cancel := context.WithCancel(m.ctx)
	ch := make(chan tea.Msg, 16)
	schema := ""
	if draft {
		schema = claude.DraftSchema
	}
	m.chatState.Asking().Go(func() {
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
		a, err := m.claude.Ask(ctx, claude.Request{PR: key, Dir: dir, Session: session, First: first, Prompt: prompt, Schema: schema},
			func(s claude.Step) { send(chatStepMsg{key: key, step: s, ch: ch}) })
		send(chatAnswerMsg{key: key, answer: a, err: err, draft: draft})
	})
	return listenChat(ch)
}

func (m *Model) chatStep(msg chatStepMsg) tea.Cmd {
	if c, ok := m.chats[msg.key]; ok {
		c.AddStep(msg.step)
	}
	return listenChat(msg.ch)
}

func (m *Model) chatPrepared(msg chatPreparedMsg) tea.Cmd {
	if c, ok := m.chats[msg.key]; ok && c.Prepared(msg.dir, msg.session) {
		m.say("the session kept was started in another cache; this question starts a new one")
	}
	return listenChat(msg.ch)
}

func (m *Model) chatAnswer(msg chatAnswerMsg) tea.Cmd {
	c, ok := m.chats[msg.key]
	if !ok {
		return nil
	}
	c.Finish()
	c.unseen = m.view != viewChat || m.chatKey != msg.key
	add := func(text string, err bool) {
		cost := 0.0
		if !err {
			cost = msg.answer.Cost
		}
		c.Reply(text, err, cost, m.set.Now())
	}
	// Kept with the answer, then the session is free for another
	// gh-triage.
	defer m.chatState.Keep(c.Conversation)
	if msg.err != nil {
		// A session not started is made anew at the next question, as
		// Claude Code may have taken its ID before failing.
		if m.ctx.Err() == nil {
			add(msg.err.Error(), true)
		}
		return nil
	}
	c.Answered(msg.answer)
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
func (m *Model) draftCompose(c *chatScreen, d claude.Draft) tea.Cmd {
	ev := d.Event
	pr := c.PR()
	if m.own(pr) || pr.State != model.StateOpen {
		ev = model.EventComment
	}
	cmd := m.startCompose(pr, ev)
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
			if c.Running() {
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
		case c.Running():
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
		c.input.SetWidth(chatInputWidth(m.width))
		return c.input.Focus()
	case is("draft_review"):
		return m.ask(c, "Draft the review.", claude.DraftPrompt, true)
	case is("chat_approve"):
		if m.own(c.PR()) {
			m.refuse("you cannot approve your own pull request")
			return nil
		}
		if m.busy() {
			return nil
		}
		return m.startBulkFor(runner.Approve, []model.PR{c.PR()})
	case is("open_claude"):
		switch {
		case c.Running():
			m.refuse("wait for Claude to answer first")
		case !m.chatState.Lock(c.Conversation):
		default:
			m.chatState.Reload(c.Conversation)
			if !c.Started() {
				c.Unlock()
				m.refuse("the conversation has not started")
				return nil
			}
			// Held while Claude Code has the session, so no other
			// gh-triage resumes it meanwhile.
			key := c.PR().Key
			return tea.ExecProcess(m.claude.Interactive(key, c.Dir(), c.Session()), func(err error) tea.Msg { return claudeExitMsg{key: key, err: err} })
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
		c.Unlock()
	}
	if msg.err != nil {
		m.fail("Claude Code: " + msg.err.Error())
		return nil
	}
	m.say("back from Claude Code; what was said there is not shown here")
	return nil
}

// restartChat starts the conversation over and asks for a review again.
func (m *Model) restartChat(c *chatScreen) tea.Cmd {
	if !m.chatState.Restart(c.Conversation) {
		return nil
	}
	c.draft, c.scroll = nil, 0
	return m.ask(c, "Review this pull request.", "", false)
}

// The chat is laid out as Claude Code lays out a conversation: a question
// on a band from the left edge, and Claude's lines after a margin, each
// answer led by ⏺ with its text under the text after it.
const (
	chatMargin = 2
	chatIndent = 4
	// chatMaxWidth is the widest a line of text is wrapped at, however
	// wide the terminal.
	chatMaxWidth = 100
	// chatLiveSteps is how many of the tools used so far are shown while
	// Claude answers.
	chatLiveSteps = 3
)

// chatLines are the conversation, wrapped to width, with Claude's answers
// drawn from their Markdown.
func (m *Model) chatLines(c *chatScreen, width int) []string {
	st := m.st
	textW := min(width, chatMaxWidth)
	margin, indent := spaces(chatMargin), spaces(chatIndent)
	var L []string
	// lead puts ⏺ before the first line that has text, and the others
	// under its text.
	lead := func(lines []string, mark lipgloss.Style) {
		first := true
		for _, l := range lines {
			switch {
			case l == "":
				L = append(L, "")
			case first:
				L = append(L, margin+mark.Render("⏺")+" "+l)
				first = false
			default:
				L = append(L, indent+l)
			}
		}
	}
	steps := func(ss []claude.Step, live bool) {
		if s := m.stepLines(c, ss, live, width); len(s) > 0 {
			L = append(L, s...)
			L = append(L, "")
		}
	}
	head := ""
	// asked is when the question an answer is to was asked.
	var asked time.Time
	turns := c.Turns()
	for i, t := range turns {
		switch {
		case t.You:
			if head != "" && t.Head != "" && t.Head != head {
				L = append(L, st.dim.Render("── head moved "+short(head)+" → "+short(t.Head)+" ──"), "")
			}
			if t.Head != "" {
				head = t.Head
			}
			asked = t.At
			L = append(L, m.questionLines(t.Text, width, textW)...)
			if i == len(turns)-1 && !c.Running() {
				// A question with no answer is on its way in another
				// gh-triage, or was cut short when one quit.
				if c.Elsewhere() != "" {
					L = append(L, "", margin+m.working(st.muted, "answering in another gh-triage ("+c.Elsewhere()+")…"))
				} else {
					L = append(L, "", margin+st.dim.Render("interrupted: gh-triage was closed before the answer"))
				}
			}
		case t.Err:
			steps(t.Steps, false)
			var ls []string
			for i, para := range strings.Split(strings.TrimRight(safe(t.Text), " \t\n"), "\n") {
				if i == 0 {
					para = "claude failed: " + para
				}
				for _, l := range wrapSpans([]span{{text: para, style: st.red}}, max(textW-chatIndent, 10), 0) {
					ls = append(ls, renderSpans(l))
				}
			}
			lead(ls, st.red)
		default:
			steps(t.Steps, false)
			// An answer is Markdown, drawn as a description is.
			text := zeroWidth.Replace(mdLinkRe.ReplaceAllString(safe(t.Text), "$1"))
			var ls []string
			for _, l := range m.markdownLines(strings.TrimRight(text, " \t\n"), max(textW-chatIndent, 10)) {
				ls = append(ls, l.text)
			}
			lead(ls, st.text)
			var took time.Duration
			if !asked.IsZero() && t.At.After(asked) {
				took = t.At.Sub(asked)
			}
			if info := turnInfo(t, took); info != "" {
				L = append(L, "", indent+st.dim.Render(info))
			}
		}
		L = append(L, "")
	}
	if c.Running() {
		steps(c.Steps(), true)
		what := "thinking…"
		if c.Dir() == "" {
			what = "making the worktree…"
		}
		L = append(L, margin+m.working(st.muted, what)+" "+st.dim.Render(elapsed(m.set.Now().Sub(c.Since()))), "")
	}
	return L
}

// questionLines are a question in full, on a band as wide as the screen.
func (m *Model) questionLines(text string, width, textW int) []string {
	st := m.st
	var out []string
	for i, para := range strings.Split(strings.TrimRight(safe(text), " \t\n"), "\n") {
		for j, l := range wrapSpans([]span{{text: para, style: st.text}}, max(textW-2, 10), 0) {
			lead := "  "
			if i == 0 && j == 0 {
				lead = st.dim.Render(">") + " "
			}
			out = append(out, onBackground(lead+renderSpans(l), st.band, width))
		}
	}
	return out
}

// stepLines are the tools Claude used: while it answers, the last few
// with what each was used on; once it has, one line that counts them.
func (m *Model) stepLines(c *chatScreen, ss []claude.Step, live bool, width int) []string {
	if len(ss) == 0 {
		return nil
	}
	st := m.st
	margin, mark := spaces(chatMargin), st.mauve.Render("⏺")+" "
	if !live {
		var names []string
		count := map[string]int{}
		for _, s := range ss {
			n := safeLine(s.Tool)
			if count[n] == 0 {
				names = append(names, n)
			}
			count[n]++
		}
		parts := make([]string, len(names))
		for i, n := range names {
			parts[i] = fmt.Sprintf("%s ×%d", n, count[n])
		}
		return []string{margin + mark + st.text.Render(tools(len(ss))) + "  " + st.dim.Render(strings.Join(parts, " · "))}
	}
	var out []string
	if k := len(ss) - chatLiveSteps; k > 0 {
		out = append(out, margin+st.dim.Render(fmt.Sprintf("… %d earlier", k)))
		ss = ss[k:]
	}
	for _, s := range ss {
		tool := safeLine(s.Tool)
		line := margin + mark + st.bold.Render(tool)
		// The room for the argument is what is left after the tool and
		// its parentheses.
		room := width - chatMargin - 2 - ansi.StringWidth(tool) - 2
		if arg := stepArg(c.Dir(), safeLine(s.Arg), room); arg != "" {
			line += st.dim.Render("(" + arg + ")")
		}
		out = append(out, line)
	}
	return out
}

// stepArg is what a tool was used on, w cells wide at most: a path in the
// worktree from the worktree, and a path cut in the middle so its file
// name stays.
func stepArg(dir, arg string, w int) string {
	path := strings.HasPrefix(arg, "/")
	if dir != "" && strings.HasPrefix(arg, dir+"/") {
		arg = arg[len(dir)+1:]
	}
	if w <= 0 || ansi.StringWidth(arg) <= w {
		return arg
	}
	if !path {
		return ansi.Truncate(arg, w, "…")
	}
	i := strings.LastIndex(arg, "/")
	tail := arg[i:]
	if i <= 0 || ansi.StringWidth(tail)+1 >= w {
		return ansi.Truncate(arg[i+1:], w, "…")
	}
	return ansi.Truncate(arg[:i], w-ansi.StringWidth(tail)-1, "") + "…" + tail
}

// turnInfo is what an answer took: the tools, the time and the cost.
func turnInfo(t chat.Turn, took time.Duration) string {
	var parts []string
	if n := len(t.Steps); n > 0 {
		parts = append(parts, tools(n))
	}
	if took > 0 {
		parts = append(parts, elapsed(took))
	}
	if t.Cost > 0 {
		parts = append(parts, fmt.Sprintf("$%.2f", t.Cost))
	}
	return strings.Join(parts, " · ")
}

func tools(n int) string {
	if n == 1 {
		return "1 tool"
	}
	return fmt.Sprintf("%d tools", n)
}

// chatInputWidth is the width of the text box in its box, as wide as the
// text of the chat.
func chatInputWidth(width int) int { return max(min(width, chatMaxWidth)-4, 10) }

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
		if c.Running() && (m.view != viewChat || m.chatKey != k) {
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
	at := "at " + short(c.PR().HeadOID)
	if c.Head() != "" && c.Head() != c.PR().HeadOID {
		at += " (was " + short(c.Head()) + ")"
	}
	info := []string{at, "read-only"}
	if c.Model() != "" {
		info = append([]string{c.Model()}, info...)
	}
	if c.Cost() > 0 {
		info = append(info, fmt.Sprintf("$%.2f", c.Cost()))
	}
	b.WriteString(fit(st.mauve.Render("claude")+" "+st.muted.Render("on")+" "+st.sub1.Render(c.PR().Key.String())+"  "+
		st.dim.Render(strings.Join(info, " · ")), m.width) + "\n")
	b.WriteString(st.dim.Render(strings.Repeat("╌", max(m.width-1, 0))) + "\n")
	inputH := 0
	if c.typing {
		// The box's top and bottom edges.
		inputH = c.input.Height() + 2
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
		top := strings.Count(b.String(), "\n")
		box := box(st, "ask claude", strings.Split(c.input.View(), "\n"), min(m.width, chatMaxWidth))
		b.WriteString(strings.Join(box, "\n") + "\n")
		cur := c.input.Cursor()
		if cur != nil {
			// Inside the box: after its left edge, under its top edge.
			cur.X += 2
			cur.Y += top + 1
		}
		return cur
	}
	return nil
}
