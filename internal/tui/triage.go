package tui

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/babarot/gh-desk/internal/claude"
	"github.com/babarot/gh-desk/internal/logging"
	"github.com/babarot/gh-desk/internal/model"
	"github.com/babarot/gh-desk/internal/plan"
	"github.com/babarot/gh-desk/internal/runner"
	"github.com/babarot/gh-desk/internal/tui/askbox"
	"github.com/babarot/gh-desk/internal/tui/chat"
	"github.com/babarot/gh-desk/internal/tui/keymap"
)

// triage is the conversation with Claude about the lists, opened by C on
// the list. Claude answers with a plan of actions. The plan is checked
// against the lists each time it is shown and when it is applied, and is
// applied only when the user accepts it; approving and merging go through
// the confirm popup as ever. The conversation lasts while gh-desk runs,
// and the popup opens and closes over it.
type triage struct {
	open    bool
	session string
	// started is set once Claude Code has the session.
	started bool
	turns   []triageTurn
	input   textarea.Model
	typing  bool
	// asking is set while a request is on its way, sent at since; cancel
	// stops it, and stopping is set once the user stopped it, until its
	// end comes back. asked tells its answer from that of an earlier one.
	asking   bool
	stopping bool
	since    time.Time
	cancel   context.CancelFunc
	asked    int
	// tab is the tab the last request was about, by its place; a plan is
	// applied there only.
	tab int
	// restart is set by a first press of chat_restart, until the next key.
	restart bool
	// seen are the pull requests as Claude was last given them.
	seen map[model.PRKey]model.PR
	// actions are the last plan's, until it is applied or closed.
	actions []claude.Action
	// top is the first line of the conversation shown once the user
	// scrolls it; until then, and again after each request, the latest is
	// shown (follow).
	top    int
	follow bool
	// model is the model of the last answer.
	model string
	// history are the questions asked, the newest first, and recall the
	// place among them. askedID is the ID of the last question, and kept
	// closes once it is written.
	history []askbox.Entry
	recall  askbox.Recall
	search  *search
	menu    askbox.Menu
	askedID string
	kept    chan struct{}
}

// triageTurn is a request and Claude's reply, or why there is none.
type triageTurn struct {
	request string
	reply   string
	err     bool
	// stopped is set when the user stopped the request.
	stopped bool
	// cost is what the reply cost, and took how long it took.
	cost float64
	took time.Duration
}

type triageMsg struct {
	asked  int
	answer claude.Answer
	err    error
}

// openTriage opens the popup, ready for a request.
func (m *Model) openTriage() tea.Cmd {
	var load tea.Cmd
	if m.claude == nil {
		m.refuse("Claude is not available")
		return nil
	}
	if m.triage == nil {
		ta := newInput(m.st)
		ta.Placeholder = "Ask Claude to find or select pull requests"
		ta.ShowLineNumbers = false
		ta.SetHeight(3)
		m.triage = &triage{session: claude.NewSession(), input: ta, follow: true}
		load = m.loadHistory()
	}
	tr := m.triage
	tr.open = true
	// A request still on its way, or a plan that came while the popup was
	// closed, is shown as it stands.
	if tr.asking || len(tr.actions) > 0 {
		return load
	}
	return tea.Batch(load, m.typeTriage())
}

func (m *Model) typeTriage() tea.Cmd {
	tr := m.triage
	tr.typing = true
	tr.input.SetWidth(m.triageWidth() - 4)
	return tr.input.Focus()
}

// closeTriage closes the popup and drops the plan not applied; the
// conversation stays.
func (m *Model) closeTriage() {
	tr := m.triage
	tr.open, tr.typing, tr.actions = false, false, nil
	tr.input.Blur()
}

func (m *Model) triageWidth() int { return min(max(m.width-8, 40), 110) }

func (m *Model) triageKey(msg tea.KeyPressMsg) tea.Cmd {
	tr, k := m.triage, msg.String()
	is := func(a keymap.Action) bool { return m.keys.Matches(a, k) }
	switch {
	case tr.typing && tr.search != nil:
		return m.searchKey(&tr.search, &tr.input, &tr.recall, tr.history, msg)
	case tr.typing && len(m.askMenu()) > 0:
		return m.askMenuKey(msg)
	case tr.typing:
		armed := tr.restart
		tr.restart = false
		switch {
		case k == keymap.Back:
			tr.typing = false
			tr.input.Blur()
			if len(tr.turns) == 0 {
				m.closeTriage()
			}
			return nil
		case is("accept"):
			q := strings.TrimSpace(tr.input.Value())
			if q == "" {
				return nil
			}
			tr.input.Reset()
			tr.recall.Leave()
			tr.typing = false
			tr.input.Blur()
			return m.askTriage(q)
		case m.recallKey(&tr.recall, &tr.input, tr.history, msg):
			return nil
		case is("history_search"):
			return m.openSearch(&tr.search, &tr.input, tr.history)
		case is("complete"):
			completeAsk(&tr.recall, &tr.input, claude.TriageQuestions(), tr.history)
			return nil
		case is("expand") && msg.Text == "" && askGhost(&tr.input, &tr.recall, claude.TriageQuestions(), tr.history) != "":
			completeAsk(&tr.recall, &tr.input, claude.TriageQuestions(), tr.history)
			return nil
		case is("chat_restart"):
			switch {
			case len(tr.turns) == 0:
				m.refuse("nothing has been asked yet")
			case !armed:
				tr.restart = true
				m.say(fmt.Sprintf("press %s again to start over", m.firstKey("chat_restart")))
			default:
				m.restartTriage()
			}
			return nil
		}
		return editAsk(&tr.recall, &tr.input, msg)
	case tr.asking:
		switch {
		case k == keymap.Back && !tr.stopping:
			m.stopTriage()
		case k == keymap.Back, is("chat_close"):
			m.closeTriage()
		}
	case k == keymap.Back:
		m.closeTriage()
	case is("triage_apply") && m.applicable():
		return m.applyTriage()
	case is("triage_more"):
		return m.typeTriage()
	case is("down"):
		// Kept within the conversation when it is drawn.
		tr.top++
		tr.follow = false
	case is("up"):
		tr.top = max(tr.top-1, 0)
		tr.follow = false
	}
	return nil
}

// askTriage sends a request with the lists as they are now.
func (m *Model) askTriage(request string) tea.Cmd {
	tr := m.triage
	if tr.asking {
		return nil
	}
	snap, seen := m.snapshot()
	tr.turns = append(tr.turns, triageTurn{request: request})
	tr.seen, tr.actions, tr.tab = seen, nil, m.cur
	tr.asking, tr.stopping, tr.since = true, false, m.set.Now()
	tr.follow = true
	tr.asked++
	ctx, cancel := context.WithCancel(m.ctx)
	tr.cancel = cancel
	r := claude.Request{Session: tr.session, First: !tr.started, Prompt: claude.TriagePrompt(snap, request)}
	n := tr.asked
	m.log.Debug("triage asked", logging.KeyOp, "triage", logging.KeyClaude, tr.session, "rows", len(seen))
	ask := func() tea.Msg {
		defer cancel()
		a, err := m.claude.Triage(ctx, r)
		return triageMsg{asked: n, answer: a, err: err}
	}
	return tea.Batch(ask, m.keepQuestion(request))
}

// stopTriage stops the request on its way. It is still asking until its
// end comes back: Claude Code takes a moment to stop, and a request sent
// meanwhile would resume the session it still holds.
func (m *Model) stopTriage() {
	tr := m.triage
	tr.stopping = true
	tr.cancel()
	m.log.Info("triage stopped", logging.KeyOp, "triage", logging.KeyClaude, tr.session)
}

// restartTriage forgets the conversation and starts a session of its own.
func (m *Model) restartTriage() {
	tr := m.triage
	tr.turns, tr.actions, tr.model = nil, nil, ""
	tr.session, tr.started = claude.NewSession(), false
	tr.top, tr.follow = 0, true
}

// triageAnswered takes in the end of a request, which may come while the
// popup is closed.
func (m *Model) triageAnswered(msg triageMsg) tea.Cmd {
	tr := m.triage
	if tr == nil || !tr.asking || msg.asked != tr.asked {
		return nil
	}
	stopped := tr.stopping
	tr.asking, tr.stopping, tr.cancel = false, false, nil
	t := &tr.turns[len(tr.turns)-1]
	t.took = m.set.Now().Sub(tr.since)
	if stopped {
		// Claude Code may have made the session before it stopped; a
		// session ID is taken once.
		if !tr.started {
			tr.session = claude.NewSession()
		}
		t.stopped = true
		if tr.open {
			return m.typeTriage()
		}
		return nil
	}
	if !tr.open {
		m.say(fmt.Sprintf("claude is done: %s shows it", m.firstKey("triage")))
	}
	if msg.err != nil {
		if !tr.started {
			tr.session = claude.NewSession()
		}
		t.reply, t.err = msg.err.Error(), true
		return nil
	}
	// Claude Code keeps the session from the first answer on.
	tr.started = true
	p, err := claude.ParseTriage(msg.answer)
	if err != nil {
		t.reply, t.err = err.Error(), true
		return nil
	}
	t.reply, t.cost, tr.actions = p.Reply, p.Cost, p.Actions
	if p.Model != "" {
		tr.model = p.Model
	}
	keep := m.keepReply(p.Reply)
	m.log.Debug("triage answered", logging.KeyOp, "triage", logging.KeyClaude, tr.session, "actions", len(p.Actions))
	return keep
}

// snapshot is the tab shown as Claude is given it, and the pull requests
// in it by key. A triage is about that tab only: what Claude is not given,
// it cannot name. A pull request any tab has read in full is given as that.
func (m *Model) snapshot() (claude.Snapshot, map[model.PRKey]model.PR) {
	t := m.tab()
	s := claude.Snapshot{Now: m.set.Now(), Tab: t.name, Grouped: t.grouped, Filter: m.filter, Pane: "off"}
	if t.ruleFilter != nil {
		s.RuleFilter = ruleNames(t.ruleFilter)
	}
	if m.pane.on {
		s.Pane = paneWords[m.pane.mode]
	}
	if p, _, ok := m.currentPR(); ok {
		s.Cursor = p.Key
	}
	for _, g := range t.groups {
		if t.folded[g] {
			s.Folded = append(s.Folded, string(g))
		}
	}
	seen := map[model.PRKey]model.PR{}
	for _, r := range m.listed(t) {
		p := m.fullPR(r.pr)
		// The rules match the pull request's own times, not the
		// notification's time an inbox row shows.
		own, ok := m.knownPR(r.pr.Key)
		if !ok {
			own = r.pr
		}
		row := claude.SnapshotRow{PR: p, Why: m.why(r), Selected: m.isSelected(p.Key),
			Narrowed: t.ruleFilter != nil && !m.passes(own, "", t.ruleFilter, s.Now), Excluded: r.ex.excluded()}
		if t.grouped {
			row.Group = string(plan.GroupOf(p, m.viewer))
		}
		if rv, ok := p.ReviewBy(m.viewer); ok && m.viewer != "" {
			row.Mine = rv.State
		}
		s.Rows = append(s.Rows, row)
		seen[p.Key] = p
	}
	return s, seen
}

// fullPR is the pull request of a row as any tab has read it in full: an
// inbox row of a pull request no list holds has only what its notification
// says. The row's time stays, as its AGE shows it.
func (m *Model) fullPR(p model.PR) model.PR {
	if known, ok := m.knownPR(p.Key); ok {
		known.Updated = p.Updated
		return known
	}
	return p
}

var paneWords = map[paneMode]string{paneOverview: "overview", paneDiff: "diff"}

// step is an action of the plan as it would be applied now: what it acts
// on, or why it is left out.
type step struct {
	claude.Action
	group plan.Group
	prs   []model.PR
	// gone are the keys of the action left out, and why.
	gone []gone
	// skip is why the whole action is left out; empty when it applies.
	skip string
}

type gone struct{ key, why string }

const noneLeft = "no pull request left"

// checkPlan checks the plan against the tab shown as it is now, in order:
// an action is checked on the filter the ones before it leave.
func (m *Model) checkPlan() []step {
	tr, t := m.triage, m.tab()
	if m.cur != tr.tab {
		return nil
	}
	now := map[model.PRKey]model.PR{}
	for _, r := range m.listed(t) {
		now[r.pr.Key] = m.fullPR(r.pr)
	}
	filter := m.filter
	var out []step
	for i, a := range tr.actions {
		s := step{Action: a}
		write := a.Do == claude.DoApprove || a.Do == claude.DoMerge
		switch a.Do {
		case claude.DoFilter:
			filter = a.Text
		case claude.DoFold, claude.DoUnfold:
			s.group = plan.Group(a.Group)
			switch {
			case !t.grouped:
				s.skip = "the tab has no groups"
			case !slices.Contains(t.groups, s.group):
				s.skip = "no such group"
			}
		case claude.DoSelect, claude.DoApprove, claude.DoMerge:
			s.prs, s.gone = m.resolve(a.Keys, now, a.Do)
			switch {
			case write && i != len(tr.actions)-1:
				s.skip = "only last"
			case len(s.prs) == 0:
				s.skip = noneLeft
			}
		case claude.DoCursor:
			s.prs, s.gone = m.resolve([]string{a.Key}, now, a.Do)
			switch {
			case len(s.gone) > 0:
				s.skip = s.gone[0].why
				s.gone = nil
			case !m.inTab(t, s.prs[0].Key, filter):
				s.skip = "hidden by the filter"
			}
		case claude.DoClearSelection:
		case claude.DoPane:
			if a.Mode != "off" && a.Mode != "overview" && a.Mode != "diff" {
				s.skip = "no such pane"
			}
		default:
			s.skip = "unknown action"
		}
		out = append(out, s)
	}
	return out
}

// resolve finds the pull requests keys name for do, as Claude saw them: a
// key Claude was not given, or whose pull request left the tab or moved on
// since, is left out. An approve or a merge leaves out what it cannot act
// on.
func (m *Model) resolve(keys []string, now map[model.PRKey]model.PR, do claude.Do) ([]model.PR, []gone) {
	write := do == claude.DoApprove || do == claude.DoMerge
	var prs []model.PR
	var out []gone
	for _, raw := range keys {
		why := ""
		k, err := model.ParsePRKey(strings.TrimSpace(raw))
		seen, wasSeen := m.triage.seen[k]
		cur, isNow := now[k]
		switch {
		case err != nil:
			why = "not a pull request"
		case !wasSeen:
			why = "not in this tab"
		case !isNow:
			why = "no longer in this tab"
		case cur.HeadOID != seen.HeadOID:
			why = "changed since Claude read it"
		case write && seen.ID == "":
			why = "not read yet"
		case do == claude.DoApprove && m.own(seen):
			why = "your own"
		case containsKey(prs, k):
			continue
		}
		if why != "" {
			out = append(out, gone{raw, why})
			continue
		}
		prs = append(prs, seen)
	}
	return prs, out
}

// inTab reports whether t shows k under filter, its group unfolded.
func (m *Model) inTab(t *tab, k model.PRKey, filter string) bool {
	return slices.ContainsFunc(m.rowsOf(t, filter, t.ruleFilter, nil), func(r row) bool { return r.kind == rowPR && r.pr.Key == k })
}

// applicable reports whether the plan has an action that applies.
func (m *Model) applicable() bool {
	return slices.ContainsFunc(m.checkPlan(), func(s step) bool { return s.skip == "" })
}

// applyTriage applies the plan, checked again now, and closes the popup;
// an approve or a merge opens the confirm popup last.
func (m *Model) applyTriage() tea.Cmd {
	tr := m.triage
	steps := m.checkPlan()
	last := tr.turns[len(tr.turns)-1]
	m.closeTriage()
	var cmds []tea.Cmd
	applied := 0
	for _, s := range steps {
		if s.skip != "" {
			continue
		}
		applied++
		switch s.Do {
		case claude.DoFilter:
			m.filter = s.Text
			m.tab().sel, m.tab().offset = 0, 0
		case claude.DoFold, claude.DoUnfold:
			m.tab().folded[s.group] = s.Do == claude.DoFold
		case claude.DoSelect:
			for _, p := range s.prs {
				if !m.isSelected(p.Key) {
					m.selected[p.Key] = byClaude
				}
			}
		case claude.DoClearSelection:
			clear(m.selected)
		case claude.DoCursor:
			cmds = append(cmds, m.cursorTo(s.prs[0]))
		case claude.DoPane:
			m.pane.on = s.Mode != "off"
			if m.pane.on {
				m.pane.mode = map[string]paneMode{"overview": paneOverview, "diff": paneDiff}[s.Mode]
				m.pane.scroll = 0
				cmds = append(cmds, m.ensure())
			}
		case claude.DoApprove, claude.DoMerge:
			if s.Do == claude.DoApprove {
				cmds = append(cmds, m.startBulkFor(runner.Review, runner.ReviewText{Event: model.EventApprove}, s.prs))
			} else {
				cmds = append(cmds, m.startBulkFor(runner.Merge, runner.ReviewText{}, s.prs))
			}
			m.bulk.by = last.request
		}
	}
	if last.reply != "" {
		m.say("claude: " + safeLine(firstLine(last.reply)))
	}
	m.log.Info("triage applied", logging.KeyOp, "triage", logging.KeyClaude, tr.session, "actions", applied, "left_out", len(steps)-applied)
	return tea.Batch(cmds...)
}

// cursorTo puts the cursor on p in the tab shown, unfolding its group.
func (m *Model) cursorTo(p model.PR) tea.Cmd {
	t := m.tab()
	if t.grouped {
		t.folded[plan.GroupOf(p, m.viewer)] = false
	}
	for i, r := range m.rows(t) {
		if r.kind == rowPR && r.pr.Key == p.Key {
			return m.moveTo(i)
		}
	}
	return nil
}

// renderTriage draws the popup and says on which of its rows the text box
// starts, -1 when it is not shown.
func (m *Model) renderTriage() ([]string, int) {
	tr, st := m.triage, m.st
	w := m.triageWidth()
	inner := w - 4
	body := m.triageTurnLines(min(inner, chatMaxWidth))
	var foot []string
	switch {
	case tr.asking:
		what := "thinking…"
		if tr.stopping {
			what = "stopping…"
		}
		body = append(body, spaces(chatMargin)+m.working(st.muted, what)+" "+st.dim.Render(elapsed(m.set.Now().Sub(tr.since))))
		back := "stop"
		if tr.stopping {
			back = "back"
		}
		foot = append(foot, m.footKeys([]keymap.Action{"chat_close"}, "", back))
	case tr.search != nil:
		foot = append(foot, strings.Split(tr.search.input.View(), "\n")...)
		foot = append(foot, m.searchLines(tr.search, tr.history, inner, searchRows(m.popupHeight()-3, triageSearchRows))...)
		foot = append(foot, m.footKeys([]keymap.Action{"accept", "history_search"}, "fill in", "cancel"))
	case tr.typing:
		menu := m.askMenu()
		ghost := askGhost(&tr.input, &tr.recall, claude.TriageQuestions(), tr.history)
		if len(menu) > 0 {
			ghost = ""
		}
		foot = append(foot, m.askLines(&tr.input, ghost)...)
		if len(menu) > 0 {
			foot = append(foot, m.menuLines(menu, inner)...)
			foot = append(foot, m.footKeys([]keymap.Action{"accept"}, "fill in", "close"))
			break
		}
		as := []keymap.Action{"accept"}
		if ghost != "" || nextSuggestion(claude.TriageQuestions(), tr.input.Value()) != "" {
			as = append(as, "complete")
		}
		if len(tr.history) > 0 {
			as = append(as, "history_search")
		}
		keys := m.footKeys(as, "send", "back")
		if hint := m.recallHint(&tr.recall, tr.history); hint != "" {
			keys = hint + st.dim.Render(" · ") + keys
		}
		foot = append(foot, keys)
	default:
		for i, s := range m.checkPlan() {
			num := fmt.Sprintf("%d ", i+1)
			words, left := m.stepLine(s)
			body = append(body, hang(st.dim.Render(num), words, inner)...)
			// What is left out of the step hangs from it, one to a line.
			for j, l := range left {
				first, rest := treeLines(st, spaces(len(num)), j == len(left)-1)
				body = append(body, hangWith(first, rest+"  ", l, inner)...)
			}
		}
		if m.cur != tr.tab && len(tr.actions) > 0 {
			body = append(body, st.red.Render("✗ this plan is for the "+m.tabs[tr.tab].name+" tab, and applies there"))
		}
		if len(tr.actions) > 0 {
			body = append(body, "")
		}
		var as []keymap.Action
		if m.applicable() {
			as = append(as, "triage_apply")
		}
		foot = append(foot, m.footKeys(append(as, "triage_more"), "", "close"))
	}
	if len(tr.turns) == 0 && !tr.asking {
		body = append(m.triageWelcome(m.popupHeight()-2-len(foot)), body...)
	}
	// The latest is shown until the user scrolls up.
	top := tr.top
	if tr.follow {
		top = len(body)
	}
	keys := ""
	if !tr.typing && !tr.asking {
		keys = m.scrollKeys()
	}
	about := m.tab()
	if tr.asking || len(tr.actions) > 0 {
		about = m.tabs[tr.tab]
	}
	title := "claude · " + about.name
	out, top := scrollBox(st, title, body, foot, w, m.popupHeight(), top, keepLines{}, keys)
	info := "plan only"
	if tr.model != "" {
		info = tr.model + " · " + info
	}
	out[0] = boxTop(st, title, info, w)
	tr.top = top
	at := -1
	if tr.typing {
		// The text box opens the foot, above the bottom edge.
		at = len(out) - 1 - len(foot)
	}
	return out, at
}

// triageTurnLines are the conversation, laid out as the claude pane lays
// out one, wrapped to textW.
func (m *Model) triageTurnLines(textW int) []string {
	tr := m.triage
	var L []string
	for i, t := range tr.turns {
		L = append(L, m.questionLines(t.request, textW)...)
		L = append(L, "")
		if i == len(tr.turns)-1 && tr.asking {
			break
		}
		switch {
		case t.stopped:
			L = append(L, spaces(chatMargin)+m.st.dim.Render("⏺ stopped"))
		case t.err:
			L = append(L, m.failedLines(t.reply, textW)...)
		case t.reply != "":
			L = append(L, m.answerLines(t.reply, textW)...)
		}
		if info := turnInfo(chat.Turn{Cost: t.cost}, t.took); info != "" && !t.err && !t.stopped {
			L = append(L, "", spaces(chatIndent)+m.st.dim.Render(info))
		}
		L = append(L, "")
	}
	return L
}

// triageWelcome is what the popup shows while nothing has been asked, in at
// most height lines: Clawd beside what the tab holds, and the questions
// complete puts in the text box. The questions give way first; when
// even Clawd does not fit, nothing is shown.
func (m *Model) triageWelcome(height int) []string {
	st, t := m.st, m.tab()
	sep := st.dim.Render(" · ")
	selected := strconv.Itoa(len(m.selected))
	filter := "none"
	if m.filter != "" {
		filter = safeLine(m.filter)
	}
	where := st.dim.Render("filter ") + st.muted.Render(filter) + sep + st.dim.Render("selected ") + st.muted.Render(selected)
	if p, _, ok := m.currentPR(); ok {
		where += sep + st.dim.Render("cursor ") + st.muted.Render(fmt.Sprintf("%s#%d", m.repoName(p), p.Key.Number))
	}
	// What a triage is given, as it counts it: without what the tab leaves
	// out.
	var prs []model.PR
	for _, r := range m.listed(t) {
		prs = append(prs, r.pr)
	}
	right := []string{
		st.title.Render(t.name) + "  " + st.dim.Render(pullRequests(len(prs))),
		st.dim.Render(strings.Join(m.groupCounts(t, prs), " · ")),
		where,
	}
	top, head := m.clawdBeside(stand(pDefault), clawdW, right)
	asks := m.tryLines(claude.TriageQuestions(), m.triage.input.Value())
	for _, L := range [][]string{
		append(append([]string{top}, head...), append(asks, "")...),
		append(append([]string{top}, head...), ""),
	} {
		if len(L) <= height {
			return L
		}
	}
	return nil
}

func pullRequests(n int) string {
	if n == 1 {
		return "1 pull request"
	}
	return strconv.Itoa(n) + " pull requests"
}

// footKeys are the keys of a popup's last line; accept, when given, is
// what the first action is called there, and back what esc does.
func (m *Model) footKeys(as []keymap.Action, accept, back string) string {
	var parts []string
	for _, a := range as {
		help := m.keys.Help(a)
		if a == "accept" && accept != "" {
			help = accept
		}
		parts = append(parts, m.st.key.Render(m.firstKey(a))+" "+m.st.muted.Render(help))
	}
	parts = append(parts, m.st.key.Render(keymap.Back)+" "+m.st.muted.Render(back))
	return strings.Join(parts, m.st.dim.Render(" · "))
}

// stepLine is a step of the plan, and what is left out of it, a line each;
// the caller numbers the step.
func (m *Model) stepLine(s step) (string, []string) {
	st := m.st
	words := st.teal.Render(m.stepWords(s))
	if s.skip != "" {
		words = st.dim.Render(m.stepWords(s))
	}
	var left []string
	for _, g := range s.gone {
		left = append(left, st.red.Render("✗ "+safeLine(g.key))+" "+st.muted.Render(g.why))
	}
	// With nothing left, the keys left out already say why.
	if s.skip != "" && (s.skip != noneLeft || len(s.gone) == 0) {
		left = append(left, st.red.Render("✗ "+s.skip))
	}
	return words, left
}

// stepWords says what a step does.
func (m *Model) stepWords(s step) string {
	names := func() string {
		var ns []string
		for _, p := range s.prs {
			ns = append(ns, m.repoName(p)+fmt.Sprintf("#%d", p.Key.Number))
		}
		return strings.Join(ns, " ")
	}
	switch s.Do {
	case claude.DoSelect, claude.DoApprove, claude.DoMerge:
		w := strings.TrimSpace(string(s.Do) + " " + names())
		if s.Do != claude.DoSelect {
			w += " → confirm"
		}
		return w
	case claude.DoFilter:
		if s.Text == "" {
			return "clear the filter"
		}
		return fmt.Sprintf("filter %q", safeLine(s.Text))
	case claude.DoFold, claude.DoUnfold:
		return string(s.Do) + " " + safeLine(s.Group)
	case claude.DoClearSelection:
		return "clear the selection"
	case claude.DoCursor:
		if len(s.prs) == 0 {
			return "cursor " + safeLine(s.Key)
		}
		return "cursor " + names()
	case claude.DoPane:
		if s.Mode == "off" {
			return "hide the pane"
		}
		return "pane " + safeLine(s.Mode)
	}
	return safeLine(string(s.Do))
}

// wrapText is s made safe and wrapped to width.
func wrapText(s string, width int) []string {
	if s == "" {
		return nil
	}
	return strings.Split(ansi.Wrap(safe(s), max(width, 10), " "), "\n")
}
