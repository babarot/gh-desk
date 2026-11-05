package tui

import (
	"fmt"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/babarot/gh-desk/internal/model"
	"github.com/babarot/gh-desk/internal/plan"
	"github.com/babarot/gh-desk/internal/runner"
	"github.com/babarot/gh-desk/internal/tui/keymap"
	"github.com/babarot/gh-desk/internal/tui/statusline"
)

type styleFn = lipgloss.Style

// progress is a pull request's step or result in a batch, written on its
// row over WHY and SIZE until a refresh clears it.
type progress struct {
	text   string
	result runner.Result
	watch  bool
	// detail is the whole of what text says in a few words, for the
	// status line; empty when text is all there is.
	detail string
	// locked is set on a merge skipped because another gh-desk holds
	// the record of pending merges; its row starts with the word.
	locked bool
}

// heldSort is where a row sorts while it shows a result: by what it was
// when its batch started, so that what the batch changes on it, such as a
// draft made ready, does not move it away from where it was being watched.
type heldSort struct {
	draft   bool
	updated time.Time
}

// hold keeps where the row of k sorts now, unless it is kept already.
func (m *Model) hold(k model.PRKey) {
	if _, ok := m.held[k]; ok {
		return
	}
	if p, ok := m.knownPR(k); ok {
		m.held[k] = heldSort{draft: p.Draft, updated: p.Updated}
	}
}

// batchState follows a batch until every pull request it acts on has a
// result, then writes one notification for it.
type batchState struct {
	kind runner.Kind
	// review is what a batch of a review sends.
	review runner.ReviewText
	// byClaude is set on a batch of a plan of Claude's, which acts on the
	// pull requests it names, never on the selection.
	byClaude bool
	// resumed is set on the merges the last run was waiting for.
	resumed bool
	goes    map[model.PRKey]bool
	order   []model.PRKey
	done    map[model.PRKey]progress
	skipped []noteItem
	noted   bool
	// comment is what a close batch writes before closing, shown in the
	// pane once GitHub took it (ADR-067).
	comment string
	// lanes are the steps of each pull request, for the progress pane.
	lanes map[model.PRKey]*lane
	// started and ended are when the batch started and when its last
	// pull request ended.
	started, ended time.Time
}

// addBatch keeps a batch that started, and shows it in the log viewer's
// progress pane if the viewer is open.
func (m *Model) addBatch(id string, bs *batchState) {
	bs.started = m.set.Now()
	m.batches[id] = bs
	m.batchOrder = append(m.batchOrder, id)
	if m.logv != nil {
		m.logv.batches = append(m.logv.batches, id)
	}
}

// lane is a pull request's steps in a batch: those done, the one it is
// on, and those the last forecast puts ahead.
type lane struct {
	done []laneStep
	cur  *laneStep
	next []runner.Step
	// checks are what the wait it is on is for.
	checks []string
}

// laneStep is a step begun at at, which took took once done.
type laneStep struct {
	step runner.Step
	at   time.Time
	took time.Duration
}

// track takes a runner's event into the pull request's lane: a forecast
// replaces the steps ahead, a step begun ends the one before, and an end
// ends the one it was on. A wait that changes what it waits on is a new
// step, so the one before stays among those done.
func (bs *batchState) track(ev runner.Event, now time.Time) {
	if bs.lanes == nil {
		bs.lanes = map[model.PRKey]*lane{}
	}
	l := bs.lanes[ev.Key]
	if l == nil {
		l = &lane{}
		bs.lanes[ev.Key] = l
	}
	switch {
	case ev.Forecast:
		l.next = ev.Plan
	case ev.Step != "":
		if l.cur == nil || l.cur.step != ev.Step {
			l.finish(now)
			l.cur = &laneStep{step: ev.Step, at: now}
			if i := slices.IndexFunc(l.next, func(s runner.Step) bool { return s == ev.Step || s.Waits() && ev.Step.Waits() }); i >= 0 {
				l.next = l.next[i+1:]
			}
		}
		// Check names come from GitHub; cleaned as they arrive.
		l.checks = nil
		for _, c := range ev.Checks {
			l.checks = append(l.checks, safeLine(c))
		}
	case ev.Result.Final():
		l.finish(now)
		l.next = nil
	}
}

// finish ends the step the lane is on.
func (l *lane) finish(now time.Time) {
	if l.cur == nil {
		return
	}
	l.cur.took = now.Sub(l.cur.at)
	l.done = append(l.done, *l.cur)
	l.cur, l.checks = nil, nil
}

type eventMsg struct {
	batch string
	ev    runner.Event
	ok    bool
	ch    <-chan runner.Event
}

func listen(batch string, ch <-chan runner.Event) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-ch
		return eventMsg{batch, ev, ok, ch}
	}
}

// resume goes on with the merges the last run was waiting for.
func (m *Model) resume() tea.Cmd {
	var note tea.Cmd
	if m.set.StoreProblem != "" {
		note = m.failNoted("merges that wait are off: "+m.set.StoreProblem, model.PRKey{})
	}
	if len(m.set.Resume) == 0 {
		return note
	}
	bs := &batchState{kind: runner.Merge, resumed: true, goes: map[model.PRKey]bool{}, done: map[model.PRKey]progress{}}
	for _, p := range m.set.Resume {
		bs.goes[p.Key] = true
		bs.order = append(bs.order, p.Key)
		m.hold(p.Key)
		m.progress[p.Key] = progress{text: "resuming"}
	}
	id, ch := m.run.Resume(m.ctx, m.set.Resume)
	m.addBatch(id, bs)
	return tea.Batch(note, listen(id, ch))
}

// event takes in a step of a batch.
func (m *Model) event(msg eventMsg) tea.Cmd {
	if !msg.ok {
		return nil
	}
	ev := msg.ev
	cmds := []tea.Cmd{listen(msg.batch, msg.ch)}
	if bs := m.batches[msg.batch]; bs != nil && !ev.Watch {
		bs.track(ev, m.set.Now())
	}
	if ev.Forecast {
		// It says only what is ahead; the row keeps what it was doing.
		return tea.Batch(cmds...)
	}
	// The runner's words can carry what GitHub said, such as an error;
	// cleaned as they arrive, before the row, the status line or the
	// progress pane draws them.
	ev.Text = safeLine(ev.Text)
	pg := progress{text: ev.Text, result: ev.Result, watch: ev.Watch}
	if ev.Locked {
		pg.text, pg.detail, pg.locked = m.lockedBy(), m.lockedDetail(ev.Key, ""), true
	}
	m.hold(ev.Key)
	m.progress[ev.Key] = pg
	if ev.Approved {
		m.markReviewed(ev.Key, model.ReviewApproved)
	}
	if ev.Result == runner.Merged {
		m.merged[ev.Key] = true
	}
	if ev.Result == runner.Readied || ev.Readied {
		m.markReady(ev.Key)
	}
	if ev.Result == runner.Closed || ev.Closed {
		m.markClosed(ev.Key)
	}
	bs := m.batches[msg.batch]
	if ev.Commented && bs != nil {
		cmds = append(cmds, m.addSentComment(ev.Key, bs.comment))
	}
	if ev.Result == runner.Reviewed && bs != nil {
		m.markReviewed(ev.Key, reviewStates[bs.review.Event])
	}
	switch ev.Result {
	case runner.Approved, runner.Reviewed, runner.Merged, runner.AutoMergeOn:
		cmds = append(cmds, m.doneAfterReview(ev.Key))
	}
	if bs == nil {
		return tea.Batch(cmds...)
	}
	if ev.Watch {
		icon, title := "✓", "merged by auto-merge"
		if ev.Result != runner.Merged {
			icon, title = "✗", "auto-merge stopped: "+ev.Text
		}
		cmds = append(cmds, m.pushNote(icon, title, []noteItem{{key: ev.Key, text: ev.Text, result: ev.Result}}))
		return tea.Batch(cmds...)
	}
	if ev.Result.Final() {
		p := m.progress[ev.Key]
		if ev.Method != "" {
			// The row says merged; the notification says how.
			p.text += " (" + methodWord(ev.Method) + ")"
		}
		bs.done[ev.Key] = p
	}
	if !bs.noted && len(bs.done) == len(bs.goes) {
		bs.noted, bs.ended = true, m.set.Now()
		cmds = append(cmds, m.noteBatch(bs))
	}
	return tea.Batch(cmds...)
}

// reviewStates are what a review of each kind leaves as the viewer's.
var reviewStates = map[model.ReviewEvent]model.ReviewState{
	model.EventApprove: model.ReviewApproved, model.EventComment: model.ReviewCommented, model.EventRequestChanges: model.ReviewChangesRequested,
}

// markReviewed shows the viewer's review on the pull request, in place of
// the one before.
func (m *Model) markReviewed(k model.PRKey, state model.ReviewState) {
	p, ok := m.knownPR(k)
	if !ok {
		return
	}
	var rs []model.Review
	for _, r := range p.Reviews {
		if r.Author != m.viewer {
			rs = append(rs, r)
		}
	}
	p.Reviews = append(rs, model.Review{Author: m.viewer, State: state})
	m.updatePR(p)
}

// markReady shows a pull request marked ready for review as no longer a
// draft.
func (m *Model) markReady(k model.PRKey) {
	if p, ok := m.knownPR(k); ok {
		p.Draft = false
		m.updatePR(p)
	}
}

// doneAfterReview marks the pull request's notification done when the
// settings say so.
func (m *Model) doneAfterReview(k model.PRKey) tea.Cmd {
	n, ok := m.unread[k]
	if !ok || !m.set.MarkDone {
		return nil
	}
	delete(m.unread, k)
	m.inboxTab().hidden[k] = true
	return func() tea.Msg { return doneMsg{n: n, err: m.src.MarkDone(m.ctx, n)} }
}

func (m *Model) noteBatch(bs *batchState) tea.Cmd {
	count := map[runner.Result]int{}
	var items []noteItem
	for _, k := range bs.order {
		p := bs.done[k]
		count[p.result]++
		items = append(items, noteItem{key: k, text: p.text, result: p.result})
	}
	items = append(items, bs.skipped...)
	var parts []string
	switch {
	case bs.kind == runner.Review && bs.review.Event == model.EventApprove:
		parts = append(parts, fmt.Sprintf("approved %d", count[runner.Approved]))
	case bs.kind == runner.Review:
		parts = append(parts, fmt.Sprintf("sent %d", count[runner.Reviewed]))
	case bs.kind == runner.Ready:
		parts = append(parts, fmt.Sprintf("marked ready %d", count[runner.Readied]))
	case bs.kind == runner.Close:
		parts = append(parts, fmt.Sprintf("closed %d", count[runner.Closed]))
	default:
		parts = append(parts, fmt.Sprintf("merged %d", count[runner.Merged]))
		if n := count[runner.AutoMergeOn]; n > 0 {
			parts = append(parts, fmt.Sprintf("%d on auto-merge", n))
		}
	}
	if n := count[runner.Stopped] + count[runner.Failed]; n > 0 {
		parts = append(parts, fmt.Sprintf("%d stopped", n))
	}
	if n := len(bs.skipped) + count[runner.Skipped]; n > 0 {
		parts = append(parts, fmt.Sprintf("%d skipped", n))
	}
	icon := "✓"
	if count[runner.Stopped]+count[runner.Failed] > 0 {
		icon = "✗"
	}
	title := kindVerb(bs.kind, bs.review.Event) + " done: " + strings.Join(parts, ", ")
	if bs.kind == runner.Review && !bs.byClaude && m.keepUnsent(bs) {
		title += fmt.Sprintf(" · those not sent are selected; %s to send it again", m.firstKey("review"))
	}
	return m.pushNote(icon, title, items)
}

// keepUnsent keeps a review that the runner did not send to some of its
// pull requests (it failed, stopped on a moved head, or was skipped when
// decided again): only those are left selected and the text comes back
// in the next review, so none is sent it twice. It reports whether any
// was left.
func (m *Model) keepUnsent(bs *batchState) bool {
	var left []model.PRKey
	for _, k := range bs.order {
		switch bs.done[k].result {
		case runner.Failed, runner.Stopped, runner.Skipped:
			left = append(left, k)
		}
	}
	if len(left) == 0 {
		return false
	}
	clear(m.selected)
	for _, k := range left {
		m.selected[k] = byHand
	}
	rv := bs.review
	m.unsentReview = &rv
	return true
}

// methodWord names a merge method as people say it.
func methodWord(m model.MergeMethod) string {
	switch m {
	case model.MethodMerge:
		return "merge commit"
	case model.MethodSquash:
		return "squash"
	case model.MethodRebase:
		return "rebase"
	}
	return strings.ToLower(string(m))
}

// unusualMethod names the method a merge takes when it is not the first
// of merge.methods: the repository does not allow that one. The first is
// the one the user expects, and is not named.
func (m *Model) unusualMethod(d plan.Decision) string {
	if d.Method == "" || len(m.set.Plan.Methods) > 0 && d.Method == m.set.Plan.Methods[0] {
		return ""
	}
	return methodWord(d.Method)
}

// mergeHow says the steps a merge takes, one after another: ready and
// approve when it takes them, then the rest, with the merge method when it
// is not the usual one.
func (m *Model) mergeHow(d plan.Decision) string {
	var steps []string
	if d.Ready {
		steps = append(steps, "ready")
	}
	if d.Approve {
		steps = append(steps, "approve")
	}
	how := strings.Join(append(steps, d.Reason), " → ")
	if w := m.unusualMethod(d); w != "" {
		how += " (" + w + ")"
	}
	return how
}

// note is an in-app notification: kept for the session, so a result that
// came while the user looked elsewhere can still be found.
type note struct {
	at    time.Time
	icon  string
	title string
	items []noteItem
	seen  bool
	// chat is the pull request whose conversation the note tells of, so
	// opening that conversation sees it; zero for any other note.
	chat model.PRKey
}

type noteItem struct {
	key    model.PRKey
	text   string
	result runner.Result
}

type notesView struct {
	sel int
	// top is the first line shown, kept by renderNotes so sel is in sight.
	top int
	pointer
}

type toastMsg struct{ n *note }

// toastFor is how long a toast shows, as long as information on the
// status line.
const toastFor = statusline.InfoFor

func (m *Model) pushNote(icon, title string, items []noteItem) tea.Cmd {
	n := &note{at: m.set.Now(), icon: icon, title: title, items: items}
	m.notes = append([]*note{n}, m.notes...)
	m.toast = n
	return m.after(toastFor, toastMsg{n})
}

// takeNoted gives the toasts' timers of the notes pushed where no command
// could be returned.
func (m *Model) takeNoted() tea.Cmd {
	if len(m.noted) == 0 {
		return nil
	}
	cmd := tea.Batch(m.noted...)
	m.noted = nil
	return cmd
}

// configNote tells of the config file's warnings, which did not stop
// gh-desk, in a notification.
func (m *Model) configNote() tea.Cmd {
	ws := m.set.ConfigWarnings
	if len(ws) == 0 {
		return nil
	}
	items := make([]noteItem, len(ws))
	for i, w := range ws {
		items[i] = noteItem{text: w}
	}
	noun := "warnings"
	if len(ws) == 1 {
		noun = "warning"
	}
	return m.pushNote("!", fmt.Sprintf("the config file has %d %s", len(ws), noun), items)
}

func (m *Model) unseen() int {
	n := 0
	for _, x := range m.notes {
		if !x.seen {
			n++
		}
	}
	return n
}

// seeChatNotes marks seen the notes that tell of the conversation of k,
// as it is opened.
func (m *Model) seeChatNotes(k model.PRKey) {
	for _, n := range m.notes {
		if n.chat == k {
			n.seen = true
		}
	}
	if m.toast != nil && m.toast.chat == k {
		m.toast = nil
	}
}

func (m *Model) openNotes() {
	m.notesAt = &notesView{}
	m.toast = nil
	for _, n := range m.notes {
		n.seen = true
	}
}

// noteLines flattens the notes for the cursor: a note's title, then its
// pull requests.
type noteLine struct {
	n    *note
	item *noteItem
}

func (m *Model) noteLines() []noteLine {
	var out []noteLine
	for _, n := range m.notes {
		out = append(out, noteLine{n: n})
		for j := range n.items {
			out = append(out, noteLine{n: n, item: &n.items[j]})
		}
	}
	return out
}

func (m *Model) notesKey(k string) tea.Cmd {
	ls := m.noteLines()
	v := m.notesAt
	is := func(a keymap.Action) bool { return m.keys.Matches(a, k) }
	switch {
	case k == keymap.Back, is("events"), is("quit"):
		m.notesAt = nil
	case is("down"):
		v.sel = min(v.sel+1, max(len(ls)-1, 0))
	case is("up"):
		v.sel = max(v.sel-1, 0)
	case is("choose"):
		return m.chooseNote()
	}
	return nil
}

// chooseNote goes to the pull request of the line under the cursor: an
// item's, or a note's first. A note of a conversation opens it there, as
// what it tells of is in the conversation.
func (m *Model) chooseNote() tea.Cmd {
	ls, v := m.noteLines(), m.notesAt
	if v.sel >= len(ls) {
		return nil
	}
	l := ls[v.sel]
	key := model.PRKey{}
	switch {
	case l.item != nil:
		key = l.item.key
	case len(l.n.items) > 0:
		key = l.n.items[0].key
	}
	m.notesAt = nil
	if key == (model.PRKey{}) {
		return nil
	}
	cmd := m.jumpTo(key)
	if p, _, ok := m.currentPR(); ok && p.Key == key && l.n.chat == key {
		return tea.Batch(cmd, m.openChat(p))
	}
	return cmd
}

// jumpTo puts the cursor on k in whichever tab has it, unfolding its group
// and clearing the filter.
func (m *Model) jumpTo(k model.PRKey) tea.Cmd {
	m.filter, m.filtering = "", false
	for i, t := range m.tabs {
		t.ruleFilter = nil
		if t.grouped {
			for _, p := range t.prs {
				if p.Key == k {
					t.folded[plan.GroupOf(p, m.viewer)] = false
				}
			}
		}
		for j, r := range m.rows(t) {
			if r.kind == rowPR && r.pr.Key == k {
				m.cur, t.sel = i, j
				m.pane.scroll = 0
				return m.ensure()
			}
		}
	}
	m.say(fmt.Sprintf("%s is no longer in any tab", k))
	return nil
}

func (m *Model) renderNotes() []string {
	st := m.st
	w := min(max(m.width-8, 40), 110)
	inner := w - 4
	ls := m.noteLines()
	v := m.notesAt
	var body []string
	if len(ls) == 0 {
		body = append(body, st.dim.Render("nothing yet"))
	}
	owner := noItems(len(body))
	var keep keepLines
	for i, l := range ls {
		cursor := " "
		if i == v.sel {
			cursor = st.blue.Render("▎")
		}
		var item []string
		if l.item == nil {
			item = hang(cursor+" "+m.noteIcon(l.n.icon)+" ", st.sub1.Render(safeLine(l.n.title))+"  "+st.dim.Render(shortAge(m.set.Now().Sub(l.n.at))+" ago"), inner)
		} else {
			// Hung from its note, under the icon.
			first, rest := treeLines(st, cursor+" ", l.item == &l.n.items[len(l.n.items)-1])
			if l.item.key == (model.PRKey{}) {
				// An item about no pull request, as a warning of the config file.
				item = hangWith(first, rest, st.yellow.Render(safeLine(l.item.text)), inner)
			} else {
				name := fit(st.sub1.Render(l.item.key.Repository())+st.muted.Render(fmt.Sprintf("#%d", l.item.key.Number)), 30) + " "
				item = hangWith(first+name, rest+spaces(31), m.resultStyle(l.item.result).Render(safeLine(l.item.text)), inner)
			}
		}
		if i == v.sel {
			for j := range item {
				item[j] = onBackground(item[j], st.sel, inner)
			}
			keep = keepLines{len(body), len(item)}
		}
		body = append(body, item...)
		for range item {
			owner = append(owner, i)
		}
	}
	foot := []string{"", st.key.Render(m.firstKey("choose")) + " " + st.muted.Render("go to the pull request") +
		st.dim.Render(" · ") + st.key.Render(keymap.Back) + " " + st.muted.Render("close")}
	out, top := scrollBox(st, "notifications", body, foot, w, m.popupHeight(), v.top, keep, "")
	v.top = top
	v.items = boxItems(owner, top, len(out), len(foot))
	return out
}

func (m *Model) noteIcon(icon string) string {
	switch icon {
	case "✗":
		return m.st.red.Render(icon)
	case "!":
		return m.st.yellow.Render(icon)
	}
	return m.st.green.Render(icon)
}

func (m *Model) resultStyle(r runner.Result) lipgloss.Style {
	switch r {
	case runner.Merged, runner.Approved, runner.Readied, runner.Closed:
		return m.st.green
	case runner.AutoMergeOn:
		return m.st.teal
	case runner.Skipped, runner.Stopped, runner.Failed:
		return m.st.red
	}
	return m.st.yellow
}

// progressText is a row's progress, short enough for the cells it takes.
func (m *Model) progressText(p progress) string {
	icon := map[runner.Result]string{runner.Running: "◌", runner.Approved: "✓", runner.Merged: "✓", runner.Readied: "✓", runner.Closed: "✓", runner.AutoMergeOn: "⧗",
		runner.Skipped: "–", runner.Stopped: "✗", runner.Failed: "✗"}[p.result]
	text := p.text
	if after, ok := strings.CutPrefix(text, "– "); ok {
		text = after
	}
	if rest, ok := strings.CutPrefix(text, "locked"); ok && p.locked {
		// The word first, in its own color: the row is cut short, and the
		// word is why.
		return m.resultStyle(p.result).Render(icon+" ") + m.st.peach.Render("locked") + m.resultStyle(p.result).Render(rest)
	}
	return m.resultStyle(p.result).Render(icon + " " + text)
}

// clearResults puts away the results: merged pull requests leave the list,
// rows show WHY and SIZE again. Steps still running stay.
func (m *Model) clearResults() int {
	n := 0
	for k, p := range m.progress {
		if p.result == runner.Running {
			continue
		}
		delete(m.progress, k)
		n++
	}
	for k := range m.merged {
		for _, t := range m.tabs {
			for i := 0; i < len(t.prs); i++ {
				if t.prs[i].Key == k {
					t.prs = append(t.prs[:i], t.prs[i+1:]...)
					i--
				}
			}
		}
		delete(m.merged, k)
	}
	for k := range m.held {
		if _, shown := m.progress[k]; !shown {
			delete(m.held, k)
		}
	}
	return n
}

// tally is the header's count of results on rows.
func (m *Model) tally() string {
	if len(m.progress) == 0 {
		return ""
	}
	count := map[runner.Result]int{}
	for _, p := range m.progress {
		count[p.result]++
	}
	var parts []string
	for _, r := range []runner.Result{runner.Merged, runner.Approved, runner.Readied, runner.Closed, runner.AutoMergeOn, runner.Running, runner.Skipped, runner.Stopped, runner.Failed} {
		if count[r] == 0 {
			continue
		}
		parts = append(parts, m.progressTextIcon(r, count[r]))
	}
	return strings.Join(parts, " ") + " " + m.st.dim.Render(m.firstKey("refresh")+" to clear")
}

func (m *Model) progressTextIcon(r runner.Result, n int) string {
	icon := map[runner.Result]string{runner.Running: "◌", runner.Approved: "✓", runner.Merged: "✓", runner.Readied: "✓", runner.Closed: "✓", runner.AutoMergeOn: "⧗",
		runner.Skipped: "–", runner.Stopped: "✗", runner.Failed: "✗"}[r]
	return m.resultStyle(r).Render(fmt.Sprintf("%s%d", icon, n))
}
