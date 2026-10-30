package tui

import (
	"context"
	"fmt"
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
	"github.com/babarot/gh-triage/internal/plan"
	"github.com/babarot/gh-triage/internal/runner"
)

// triage is the conversation with Claude about the lists, opened by C on
// the list. Claude answers with a plan of actions. The plan is checked
// against the lists each time it is shown and when it is applied, and is
// applied only when the user accepts it; approving and merging go through
// the confirm popup as ever. The conversation lasts while gh-triage runs,
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
	// stops it, and asked tells its answer from that of one stopped.
	asking bool
	since  time.Time
	cancel context.CancelFunc
	asked  int
	// seen are the pull requests as Claude was last given them.
	seen map[model.PRKey]model.PR
	// actions are the last plan's, until it is applied or closed.
	actions []claude.Action
}

// triageTurn is a request and Claude's reply, or why there is none.
type triageTurn struct {
	request string
	reply   string
	err     bool
}

type triageMsg struct {
	asked  int
	answer claude.Answer
	err    error
}

// openTriage opens the popup, ready for a request.
func (m *Model) openTriage() tea.Cmd {
	if m.claude == nil {
		m.setStatus("Claude is not available", true)
		return nil
	}
	if m.triage == nil {
		ta := textarea.New()
		ta.Placeholder = "Ask Claude to find or select pull requests"
		ta.ShowLineNumbers = false
		ta.SetHeight(3)
		m.triage = &triage{session: claude.NewSession(), input: ta}
	}
	m.triage.open = true
	return m.typeTriage()
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
	case tr.typing:
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
			tr.typing = false
			tr.input.Blur()
			return m.askTriage(q)
		}
		var cmd tea.Cmd
		tr.input, cmd = tr.input.Update(msg)
		return cmd
	case tr.asking:
		if k == keymap.Back {
			m.stopTriage()
		}
	case k == keymap.Back:
		m.closeTriage()
	case is("triage_apply") && m.applicable():
		return m.applyTriage()
	case is("triage_more"):
		return m.typeTriage()
	}
	return nil
}

// askTriage sends a request with the lists as they are now.
func (m *Model) askTriage(request string) tea.Cmd {
	tr := m.triage
	snap, seen := m.snapshot()
	tr.turns = append(tr.turns, triageTurn{request: request})
	tr.seen, tr.actions = seen, nil
	tr.asking, tr.since = true, m.set.Now()
	tr.asked++
	ctx, cancel := context.WithCancel(m.ctx)
	tr.cancel = cancel
	r := claude.Request{Session: tr.session, First: !tr.started, Prompt: claude.TriagePrompt(snap, request)}
	n := tr.asked
	m.log.Debug("triage asked", logging.KeyOp, "triage", logging.KeyClaude, tr.session, "rows", len(seen))
	return func() tea.Msg {
		defer cancel()
		a, err := m.claude.Triage(ctx, r)
		return triageMsg{asked: n, answer: a, err: err}
	}
}

// stopTriage stops the request on its way, forgets it, and closes the
// popup.
func (m *Model) stopTriage() {
	tr := m.triage
	tr.cancel()
	tr.asking, tr.cancel = false, nil
	tr.turns = tr.turns[:len(tr.turns)-1]
	if !tr.started {
		// Claude Code may have made the session before it stopped; a
		// session ID is taken once.
		tr.session = claude.NewSession()
	}
	m.closeTriage()
}

func (m *Model) triageAnswered(msg triageMsg) {
	tr := m.triage
	if tr == nil || !tr.asking || msg.asked != tr.asked {
		return
	}
	tr.asking, tr.cancel = false, nil
	t := &tr.turns[len(tr.turns)-1]
	if msg.err != nil {
		if !tr.started {
			tr.session = claude.NewSession()
		}
		t.reply, t.err = msg.err.Error(), true
		return
	}
	// Claude Code keeps the session from the first answer on.
	tr.started = true
	p, err := claude.ParseTriage(msg.answer)
	if err != nil {
		t.reply, t.err = err.Error(), true
		return
	}
	t.reply, tr.actions = p.Reply, p.Actions
	m.log.Debug("triage answered", logging.KeyOp, "triage", logging.KeyClaude, tr.session, "actions", len(p.Actions))
}

// snapshot is the lists as Claude is given them, and the pull requests in
// them by key. A pull request any tab has read in full is given as that.
func (m *Model) snapshot() (claude.Snapshot, map[model.PRKey]model.PR) {
	s := claude.Snapshot{Now: m.set.Now(), Filter: m.filter, Pane: "off"}
	if m.pane.on {
		s.Pane = paneWords[m.pane.mode]
	}
	if p, _, ok := m.currentPR(); ok {
		s.Cursor = p.Key
	}
	seen := map[model.PRKey]model.PR{}
	for i, t := range m.tabs {
		st := claude.SnapshotTab{Name: t.name, Shown: i == m.cur, Grouped: t.grouped}
		for _, g := range plan.Groups {
			if t.grouped && t.folded[g] {
				st.Folded = append(st.Folded, string(g))
			}
		}
		for _, r := range m.listed(t) {
			p := m.fullPR(r.pr)
			row := claude.SnapshotRow{PR: p, Why: m.why(r), Selected: m.isSelected(p.Key)}
			if t.grouped {
				row.Group = string(plan.GroupOf(p, m.viewer))
			}
			if rv, ok := p.ReviewBy(m.viewer); ok && m.viewer != "" {
				row.Mine = rv.State
			}
			st.Rows = append(st.Rows, row)
			seen[p.Key] = p
		}
		s.Tabs = append(s.Tabs, st)
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
	tab   int
	group plan.Group
	prs   []model.PR
	// gone are the keys of the action left out, and why.
	gone []gone
	// skip is why the whole action is left out; empty when it applies.
	skip string
}

type gone struct{ key, why string }

const noneLeft = "no pull request left"

// checkPlan checks the plan against the lists as they are now, in order:
// an action is checked on the tab and filter the ones before it leave.
func (m *Model) checkPlan() []step {
	tr := m.triage
	now := map[model.PRKey]model.PR{}
	for _, t := range m.tabs {
		for _, r := range m.listed(t) {
			now[r.pr.Key] = m.fullPR(r.pr)
		}
	}
	cur, filter := m.cur, m.filter
	var out []step
	for i, a := range tr.actions {
		s := step{Action: a, tab: cur}
		write := a.Do == claude.DoApprove || a.Do == claude.DoMerge
		switch a.Do {
		case claude.DoTab:
			s.tab = slices.IndexFunc(m.tabs, func(t *tab) bool { return strings.EqualFold(t.name, a.Tab) })
			if s.tab < 0 {
				s.skip = "no such tab"
			} else {
				cur = s.tab
			}
		case claude.DoFilter:
			filter = a.Text
		case claude.DoFold, claude.DoUnfold:
			s.group = plan.Group(a.Group)
			switch {
			case !m.tabs[cur].grouped:
				s.skip = "the tab has no groups"
			case !slices.Contains(plan.Groups, s.group):
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
			case !m.inTab(m.tabs[cur], s.prs[0].Key, filter):
				s.skip = "not in the tab shown"
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
// key Claude was not given, or whose pull request left the lists or moved
// on since, is left out. An approve or a merge leaves out what it cannot
// act on.
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
			why = "not in the lists"
		case !isNow:
			why = "no longer in the lists"
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
	return slices.ContainsFunc(m.rowsOf(t, filter, nil), func(r row) bool { return r.kind == rowPR && r.pr.Key == k })
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
		case claude.DoTab:
			cmds = append(cmds, m.switchTab(s.tab))
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
			if m.busy() {
				continue
			}
			kind := map[claude.Do]runner.Kind{claude.DoApprove: runner.Approve, claude.DoMerge: runner.Merge}[s.Do]
			cmds = append(cmds, m.startBulkFor(kind, s.prs))
			m.bulk.by = last.request
		}
	}
	if last.reply != "" {
		m.setStatus("claude: "+safeLine(firstLine(last.reply)), false)
	}
	m.log.Debug("triage applied", logging.KeyOp, "triage", logging.KeyClaude, tr.session, "actions", applied, "left_out", len(steps)-applied)
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

func (m *Model) renderTriage() []string {
	tr, st := m.triage, m.st
	w := m.triageWidth()
	inner := w - 4
	var body []string
	for _, t := range tr.turns {
		body = append(body, st.blue.Render("you")+"  "+st.sub1.Render(safeLine(t.request)))
		style := st.sub1
		if t.err {
			style = st.red
		}
		for _, l := range wrapText(t.reply, inner) {
			body = append(body, style.Render(l))
		}
		body = append(body, "")
	}
	var foot []string
	switch {
	case tr.asking:
		body = append(body, st.mauve.Render(m.spinner())+" "+st.muted.Render("thinking…")+" "+st.dim.Render(elapsed(m.set.Now().Sub(tr.since))))
		foot = append(foot, st.key.Render(keymap.Back)+" "+st.muted.Render("stop"))
	case tr.typing:
		foot = append(foot, strings.Split(tr.input.View(), "\n")...)
		foot = append(foot, m.footKeys([]keymap.Action{"accept"}, "send", "back"))
	default:
		for i, s := range m.checkPlan() {
			body = append(body, m.stepLine(i, s))
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
	// The latest is kept when the conversation is longer than the screen.
	if room := max(m.height-6-len(foot), 3); len(body) > room {
		body = body[len(body)-room:]
	}
	if len(tr.turns) == 0 && !tr.asking {
		body = append(body, st.dim.Render("Ask what to show or select: \"bots with passing checks\", \"what has waited longest\"."), "")
	}
	return box(st, "claude", append(body, foot...), w)
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

// stepLine is a step of the plan, numbered, with what is left out of it.
func (m *Model) stepLine(i int, s step) string {
	st := m.st
	words := st.teal.Render(m.stepWords(s))
	if s.skip != "" {
		words = st.dim.Render(m.stepWords(s))
	}
	line := st.dim.Render(fmt.Sprintf("%d ", i+1)) + words
	for _, g := range s.gone {
		line += "  " + st.red.Render("✗ "+safeLine(g.key)) + " " + st.muted.Render(g.why)
	}
	// With nothing left, the keys left out already say why.
	if s.skip != "" && (s.skip != noneLeft || len(s.gone) == 0) {
		line += "  " + st.red.Render("✗ "+s.skip)
	}
	return line
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
	case claude.DoTab:
		return "tab " + safeLine(s.Tab)
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
