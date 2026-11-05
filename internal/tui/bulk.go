package tui

import (
	"fmt"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/babarot/gh-desk/internal/claude"
	"github.com/babarot/gh-desk/internal/logging"
	"github.com/babarot/gh-desk/internal/model"
	"github.com/babarot/gh-desk/internal/plan"
	"github.com/babarot/gh-desk/internal/runner"
	"github.com/babarot/gh-desk/internal/tui/keymap"
)

// bulk is the confirm popup of a review, a merge or a ready for review, of
// one pull request or of every one selected. It reads each pull request
// again, decides what to do with it, and lists that, with the reason for
// every one it skips.
type bulk struct {
	kind runner.Kind
	// review is what a popup of a review sends; an approval from A has
	// no body.
	review runner.ReviewText
	items  []bulkItem
	// loading is set until every read is back; left counts them.
	loading bool
	left    int
	err     error
	// offset is the first line of the popup's body shown.
	offset         int
	includeFailing bool
	// waitJobs is set while approvals wait for the jobs at work on their
	// head, as they do unless the user says otherwise.
	waitJobs bool
	// notes is how far Claude has read the release notes of the bot pull
	// requests to merge, and notesErr why it could not.
	notes    notesState
	notesErr string
	// by is the request Claude picked the pull requests for, empty when
	// the user picked them.
	by string
	// tookOver is set when this gh-desk took the record of pending
	// merges over while the popup was open.
	tookOver bool
	// close is the comment and the branch of a close popup; nil for
	// another kind.
	close *closeForm
}

type notesState int

const (
	notesOff notesState = iota
	notesReading
	notesRead
	notesFailed
)

type bulkItem struct {
	pr model.PR
	mi model.MergeInfo
	// read is set once its read is back, and ok when it found it.
	read bool
	ok   bool
	d    plan.Decision
	// moved is set when the head is not the one the list read, so the
	// diff the user looked at is not what would be approved or merged.
	moved bool
	// running is set when another batch is still working on it.
	running bool
	// noWait is set when the merge would wait while another gh-desk
	// holds the record of pending merges.
	noWait bool
	// closing is what a close popup would do to it.
	closing plan.Close
}

// goes reports whether the item is acted on.
func (it bulkItem) goes() bool {
	if !it.ok || it.moved || it.running || it.noWait {
		return false
	}
	return it.d.Step != plan.StepSkip
}

// approves reports whether the popup sends approvals.
func (b *bulk) approves() bool {
	return b.kind == runner.Review && b.review.Event == model.EventApprove
}

// waits reports whether its approvals wait for the jobs.
func (b *bulk) waits() bool { return b.approves() && b.waitJobs }

// atWork counts the pull requests read with jobs at work on their head.
func (b *bulk) atWork() int {
	n := 0
	for _, it := range b.items {
		if it.ok && plan.JobsRunning(it.mi, plan.Trigger{}, time.Time{}) {
			n++
		}
	}
	return n
}

// verb names what the popup does, in its title and question.
func (b *bulk) verb() string { return kindVerb(b.kind, b.review.Event) }

// kindVerb names what a batch of kind does; a review by its event.
func kindVerb(kind runner.Kind, ev model.ReviewEvent) string {
	if kind == runner.Review {
		return sentWord(false, ev)
	}
	return bulkVerbs[kind]
}

// lockedOut reports whether it is skipped only because the merge would
// wait while another gh-desk holds the record of pending merges.
func (it bulkItem) lockedOut() bool {
	return it.ok && !it.moved && !it.running && it.noWait
}

// why is why it is skipped; refresh names the key that reads it again,
// and locked the gh-desk holding the record of pending merges.
func (it bulkItem) why(refresh, locked string) string {
	switch {
	case !it.ok:
		return "could not read it"
	case it.moved:
		return "changed since it was read · " + refresh + " to read again"
	case it.running:
		return "already running"
	case it.noWait:
		return locked + " · " + waitWord(it.d.Step)
	}
	return it.d.Reason
}

type bulkInfoMsg struct {
	b   *bulk
	ids []string
	got map[string]model.MergeInfo
	err error
}

// The popup reads its pull requests again a few at a time, side by side,
// so the rows fill in as they come and the whole is sooner than one read.
const (
	bulkBatch    = 5
	bulkInFlight = 10
)

// startBulk opens the popup of a merge or a ready for review for the
// selected pull requests, or for the one under the cursor when none is
// selected.
func (m *Model) startBulk(kind runner.Kind) tea.Cmd {
	if m.busy() {
		return nil
	}
	if kind == runner.Merge {
		// The gh-desk holding the record may have quit since.
		return tea.Batch(m.takeLock(), m.startBulkOf(kind, runner.ReviewText{}))
	}
	return m.startBulkOf(kind, runner.ReviewText{})
}

// startApprove opens the popup of an approval with no body: a review
// written by A, which skips the text.
func (m *Model) startApprove() tea.Cmd {
	if m.busy() {
		return nil
	}
	return m.startBulkOf(runner.Review, runner.ReviewText{Event: model.EventApprove})
}

// startBulkOf opens the popup of kind for what startBulk chose.
func (m *Model) startBulkOf(kind runner.Kind, rv runner.ReviewText) tea.Cmd {
	var prs []model.PR
	if len(m.selected) > 0 {
		for _, t := range m.tabs {
			for _, p := range m.allOf(t) {
				if m.isSelected(p.Key) && !containsKey(prs, p.Key) {
					prs = append(prs, p)
				}
			}
		}
	} else if p, _, ok := m.currentPR(); ok {
		if kind == runner.Review && rv.Event == model.EventApprove && m.own(p) {
			m.refuse("you cannot approve your own pull request")
			return nil
		}
		prs = []model.PR{p}
	}
	return m.startBulkFor(kind, rv, prs)
}

// startBulkFor opens the confirm popup for prs; rv is the review a popup
// of a review sends.
func (m *Model) startBulkFor(kind runner.Kind, rv runner.ReviewText, prs []model.PR) tea.Cmd {
	if len(prs) == 0 {
		return nil
	}
	b := &bulk{kind: kind, review: rv, loading: true, waitJobs: true}
	var ids []string
	for _, p := range prs {
		pg, inBatch := m.progress[p.Key]
		b.items = append(b.items, bulkItem{pr: p, running: inBatch && pg.result == runner.Running})
		if p.ID != "" {
			ids = append(ids, p.ID)
		}
	}
	m.bulk = b
	// Many selected: bigger batches, so no more than bulkInFlight reads
	// are on their way at once.
	size := max(bulkBatch, (len(ids)+bulkInFlight-1)/bulkInFlight)
	var cmds []tea.Cmd
	for len(ids) > 0 {
		n := min(size, len(ids))
		part := ids[:n]
		ids = ids[n:]
		b.left++
		cmds = append(cmds, func() tea.Msg {
			got, err := m.src.MergeInfo(m.ctx, part, true)
			return bulkInfoMsg{b, part, got, err}
		})
	}
	if b.left == 0 {
		b.loading = false
		for i := range b.items {
			b.items[i].read = true
		}
	}
	return tea.Batch(cmds...)
}

func containsKey(prs []model.PR, k model.PRKey) bool {
	for _, p := range prs {
		if p.Key == k {
			return true
		}
	}
	return false
}

// allOf is every pull request a tab holds, filtered or not, leaving out
// the groups the tab does not show.
func (m *Model) allOf(t *tab) []model.PR {
	if !t.inbox && !t.grouped {
		return t.prs
	}
	if !t.inbox {
		// A group left out of the tab's groups is never shown, so nothing
		// in it is selected or acted on from this tab.
		var out []model.PR
		for _, p := range t.prs {
			if slices.Contains(t.groups, plan.GroupOf(p, m.viewer)) {
				out = append(out, p)
			}
		}
		return out
	}
	var out []model.PR
	for _, n := range t.notifs {
		if p, ok := m.knownPR(n.Key); ok {
			out = append(out, p)
		}
	}
	return out
}

func (m *Model) bulkInfo(msg bulkInfoMsg) tea.Cmd {
	b := msg.b
	if m.bulk != b {
		return nil
	}
	b.left--
	b.loading = b.left > 0
	if msg.err != nil {
		b.err = msg.err
	}
	asked := map[string]bool{}
	for _, id := range msg.ids {
		asked[id] = true
	}
	for i := range b.items {
		it := &b.items[i]
		if !asked[it.pr.ID] {
			continue
		}
		it.read = true
		it.mi, it.ok = msg.got[it.pr.ID]
		if it.ok {
			m.cache.info[it.pr.Key] = it.mi
			it.moved = it.mi.State == model.StateOpen && it.mi.HeadOID != it.pr.HeadOID
		}
	}
	if b.loading {
		m.decideBulk()
		return nil
	}
	// A pull request with no ID is never read.
	for i := range b.items {
		b.items[i].read = true
	}
	m.decideBulk()
	if b.kind == runner.Close {
		return m.readBasedOn(b)
	}
	return m.readNotes(b)
}

// notesID names what Claude said of a pull request's release notes at a
// head, so another popup does not ask again.
func notesID(p model.PR) string { return p.Key.String() + "@" + p.HeadOID }

// readNotes has Claude read the release notes of the bot pull requests a
// merge popup would merge, and mark those that need care. It only marks:
// what is merged is still the user's choice.
func (m *Model) readNotes(b *bulk) tea.Cmd {
	if b.kind != runner.Merge || !m.set.ReleaseNotes || m.claude == nil {
		return nil
	}
	var ask []model.PR
	for _, it := range b.items {
		if _, known := m.cautions[notesID(it.pr)]; it.goes() && it.pr.AuthorIsBot && !known {
			ask = append(ask, it.pr)
		}
	}
	if len(ask) == 0 {
		for _, it := range b.items {
			if _, known := m.cautions[notesID(it.pr)]; known {
				b.notes = notesRead
			}
		}
		return nil
	}
	b.notes = notesReading
	return func() tea.Msg {
		var ids []string
		for _, p := range ask {
			ids = append(ids, p.ID)
		}
		bodies, err := m.src.Bodies(m.ctx, ids)
		if err != nil {
			return notesMsg{b: b, err: err}
		}
		var ns []claude.Notes
		for _, p := range ask {
			ns = append(ns, claude.Notes{Key: p.Key, Title: p.Title, Body: bodies[p.ID]})
		}
		a, err := m.claude.Once(m.ctx, "", claude.NotesPrompt(ns, m.set.Language), claude.NotesSchema)
		if err != nil {
			return notesMsg{b: b, err: err}
		}
		cs, err := claude.ParseCautions(a, ns)
		return notesMsg{b: b, asked: ask, cautions: cs, err: err}
	}
}

type notesMsg struct {
	b        *bulk
	asked    []model.PR
	cautions []claude.Caution
	err      error
}

func (m *Model) notesRead(msg notesMsg) tea.Cmd {
	if msg.err != nil {
		m.log.Warn("release notes", "err", msg.err.Error())
		msg.b.notes, msg.b.notesErr = notesFailed, msg.err.Error()
		return nil
	}
	why := map[model.PRKey]string{}
	for _, c := range msg.cautions {
		why[c.Key] = c.Why
	}
	for _, p := range msg.asked {
		m.cautions[notesID(p)] = why[p.Key]
	}
	m.log.Info("release notes read", logging.KeyOp, "release-notes", "prs", len(msg.asked), "cautions", len(why))
	msg.b.notes = notesRead
	return nil
}

func (m *Model) decideBulk() {
	b := m.bulk
	for i := range b.items {
		it := &b.items[i]
		it.noWait = false
		if !it.ok {
			continue
		}
		switch b.kind {
		case runner.Review:
			it.d = plan.ForReview(it.mi, it.pr, m.viewer, b.review.Event, b.includeFailing, m.set.Plan)
			if b.waits() {
				it.d = plan.AfterJobs(it.d, it.mi, plan.Trigger{}, time.Time{}, b.includeFailing)
				it.noWait = it.d.Step == plan.StepWait && !m.set.CanWait
			}
		case runner.Ready:
			it.d = plan.ForReady(it.mi)
		case runner.Close:
			it.closing = b.close.decide(it.mi)
			it.d = plan.Decision{Reason: it.closing.Reason, Notes: it.closing.Notes}
			if it.closing.Skip {
				it.d.Step = plan.StepSkip
			}
		default:
			it.d = plan.ForMerge(it.mi, it.pr, m.viewer, m.set.Plan)
			switch it.d.Step {
			case plan.StepWait, plan.StepUpdate, plan.StepAutoMerge:
				it.noWait = !m.set.CanWait
			}
		}
	}
}

func (m *Model) bulkKey(k string) tea.Cmd {
	b := m.bulk
	is := func(a keymap.Action) bool { return m.keys.Matches(a, k) }
	switch {
	case k == keymap.Back:
		// A review being written is under the popup, and comes back.
		if b.close != nil {
			m.unsentClose = b.close.ta.Value()
		}
		m.bulk = nil
		m.say("canceled")
	case is("down"):
		// Kept within the body when it is drawn.
		b.offset++
	case is("up"):
		b.offset = max(b.offset-1, 0)
	case is("include_failing") && b.approves() && !b.loading:
		b.includeFailing = !b.includeFailing
		m.decideBulk()
	case is("wait_for_checks") && b.approves() && b.atWork() > 0 && !b.loading:
		b.waitJobs = !b.waitJobs
		m.decideBulk()
	case is("delete_branch") && b.close != nil:
		b.close.del = !b.close.del
		m.decideBulk()
	case is("write_comment") && b.close != nil:
		return b.close.focus()
	case is("confirm") && !b.loading:
		return m.runBulk()
	}
	return nil
}

// runBulk starts the batch: what it skips leaves the selection at once and
// keeps its reason on its row; what it acts on shows its progress there.
// A review being written under the popup is closed once its batch starts;
// with nothing to do, it is left open.
func (m *Model) runBulk() tea.Cmd {
	b := m.bulk
	var items []runner.Item
	bs := &batchState{kind: b.kind, review: b.review, byClaude: b.by != "", goes: map[model.PRKey]bool{}, done: map[model.PRKey]progress{}}
	for _, it := range b.items {
		if it.goes() {
			items = append(items, runner.Item{PR: it.pr, Head: it.mi.HeadOID, AutoMerge: it.d.Step == plan.StepAutoMerge,
				Wait: b.waits(), IncludeFailing: b.includeFailing})
			bs.goes[it.pr.Key] = true
			bs.order = append(bs.order, it.pr.Key)
			continue
		}
		delete(m.selected, it.pr.Key)
		why := it.why(m.firstKey("refresh"), m.lockedBy())
		// The row says why for a while; the log keeps it.
		m.log.Info("pr skipped", logging.KeyOp, b.verb(), logging.KeyPR, it.pr.Key.String(), "why", why)
		pg := progress{text: "– " + why, result: runner.Skipped}
		if it.lockedOut() {
			// The row has room for the reason only; the rest is on the
			// status line when the cursor is on it.
			pg.text, pg.detail, pg.locked = "– "+m.lockedBy(), m.lockedDetail(it.pr.Key, it.d.Step), true
		}
		m.progress[it.pr.Key] = pg
		bs.skipped = append(bs.skipped, noteItem{key: it.pr.Key, text: "skipped: " + why, result: runner.Skipped})
	}
	m.bulk = nil
	if len(items) == 0 && b.close != nil {
		// Nothing was sent, so the comment is kept as on esc.
		m.unsentClose = b.close.ta.Value()
	}
	if len(items) == 0 {
		if m.rowResult() != "" {
			m.movedOnto()
		} else {
			m.say("nothing to do")
		}
		return nil
	}
	for _, it := range items {
		m.hold(it.PR.Key)
		m.progress[it.PR.Key] = progress{text: "queued"}
		if !b.approves() {
			delete(m.selected, it.PR.Key)
		}
	}
	var id string
	var ch <-chan runner.Event
	switch {
	case b.kind == runner.Review:
		m.compose = nil
		id, ch = m.run.StartReview(m.ctx, b.review, items)
	case b.close != nil:
		ct := b.close.text()
		bs.comment = ct.Body
		m.unsentClose = ""
		id, ch = m.run.StartClose(m.ctx, ct, items)
	default:
		id, ch = m.run.Start(m.ctx, b.kind, items)
	}
	m.addBatch(id, bs)
	return listen(id, ch)
}

// renderBulk draws the confirm popup.
func (m *Model) renderBulk() []string {
	b, st := m.bulk, m.st
	w := min(max(m.width-6, 40), 130)
	verb := b.verb()
	if b.kind == runner.Merge {
		ready, approve := false, false
		for _, it := range b.items {
			if it.ok && it.goes() {
				ready, approve = ready || it.d.Ready, approve || it.d.Approve
			}
		}
		switch {
		case ready && approve:
			verb = "ready, approve and merge"
		case ready:
			verb = "ready and merge"
		case approve:
			verb = "approve and merge"
		}
	}
	inner := w - 4
	var lines []string
	if b.tookOver {
		lines = append(lines, wrapped(st.teal, "↻ The other gh-desk quit; this one runs the merges that wait now.", inner)...)
		lines = append(lines, "")
	}
	if b.by != "" {
		lines = append(lines, hang(st.teal.Render("✧ picked by Claude")+st.muted.Render(": "), st.muted.Render("“"+safeLine(firstLine(b.by))+"”"), inner)...)
		lines = append(lines, "")
	}
	if body := b.review.Body; body != "" {
		lines = append(lines, bodyLines(st, body, inner)...)
		lines = append(lines, "")
	}
	if b.err != nil {
		lines = append(lines, wrapped(st.red, "could not read them all: "+b.err.Error(), inner)...)
	}
	goes := 0
	auto := false
	failing := 0
	for _, it := range b.items {
		if it.goes() {
			goes++
			auto = auto || it.d.Step == plan.StepAutoMerge
		}
		if it.ok && (plan.Failing(it.mi) || b.waits() && len(plan.FailedJobs(it.mi)) > 0) {
			failing++
		}
	}
	rows := make([]bulkRow, 0, len(b.items))
	for _, it := range b.items {
		mark, title := st.dim.Render("·"), st.dim.Render(it.pr.Title)
		var what string
		switch {
		case !it.read:
			// Shown from the list until it is read again. Nothing on it
			// moves: the spinner at the foot is the one the popup has.
			mark, title, what = st.dim.Render("·"), st.muted.Render(it.pr.Title), st.dim.Render("reading…")
		case it.goes():
			mark, title = st.mauve.Render("✓"), st.sub1.Render(it.pr.Title)
			switch b.kind {
			case runner.Review, runner.Ready:
				style := st.green
				if it.d.Step == plan.StepWait {
					style = m.stepStyle(it.d.Step)
				}
				what = style.Render(it.d.Reason)
			case runner.Close:
				what = m.closeWhat(it.closing)
			default:
				if it.d.Ready {
					what = st.green.Render("ready") + st.dim.Render(" → ")
				}
				if it.d.Approve {
					what += st.green.Render("approve") + st.dim.Render(" → ")
				}
				what += m.stepStyle(it.d.Step).Render(it.d.Reason) + " " + st.dim.Render("("+methodWord(it.d.Method)+")")
			}
		default:
			why := it.why(m.firstKey("refresh"), m.lockedBy())
			what = st.dim.Render("skip · ") + st.red.Render(why)
			if rest, ok := strings.CutPrefix(why, "locked"); ok && it.lockedOut() {
				what = st.dim.Render("skip · ") + st.peach.Render("locked") + st.red.Render(rest)
			}
		}
		r := bulkRow{pr: it.pr, mark: mark, title: title, what: what}
		if it.goes() {
			// On lines of their own, so a long one does not take the
			// width of the title or of what is done.
			// Every note, since each says something a yes agrees to: a
			// draft that stays ready, commits auto-merge takes later.
			for _, n := range it.d.Notes {
				r.notes = append(r.notes, bulkNote{"⚠", n, st.yellow})
			}
			// The reason is what to read before merging.
			if why := m.cautions[notesID(it.pr)]; why != "" {
				r.notes = append(r.notes, bulkNote{"⚑", why, st.peach})
			}
		}
		rows = append(rows, r)
	}
	lines = append(lines, m.layoutBulk(rows, inner, len(b.items) == 1)...)
	lines = append(lines, "")
	prose := func(s styleFn, text string) {
		lines = append(lines, wrapped(s, text, inner)...)
	}
	if slices.ContainsFunc(b.items, bulkItem.lockedOut) {
		prose(st.peach, "Merges that wait are locked by "+m.lockedHolder()+". Quit it and press "+m.firstKey("merge")+
			" again to wait here, or merge once the checks pass.")
	}
	switch b.notes {
	case notesReading:
		prose(st.dim, "Claude is reading the release notes of the bot pull requests…")
	case notesRead:
		n := 0
		for _, it := range b.items {
			if m.cautions[notesID(it.pr)] != "" && it.goes() {
				n++
			}
		}
		text := "Claude read the release notes: nothing calls for care."
		if n > 0 {
			text = fmt.Sprintf("Claude read the release notes: %d marked ⚑ call for care.", n)
		}
		prose(st.dim, text)
	case notesFailed:
		prose(st.yellow, "Claude could not read the release notes: "+safeLine(b.notesErr))
	}
	if auto {
		prose(st.dim, "Auto-merge also merges commits pushed after it is on, as GitHub does.")
	}
	if b.approves() && failing > 0 && !b.loading {
		state := "left out"
		if b.includeFailing {
			state = "included"
		}
		prose(st.dim, fmt.Sprintf("%d with failing checks are %s · %s to change", failing, state, m.firstKey("include_failing")))
	}
	if n := b.atWork(); b.approves() && n > 0 && !b.loading {
		switch {
		case b.waitJobs:
			prose(st.dim, fmt.Sprintf("%d with checks still running are approved once they pass · %s to approve now", n, m.firstKey("wait_for_checks")))
		default:
			prose(st.dim, fmt.Sprintf("%d with checks still running are approved now · %s to wait for the checks", n, m.firstKey("wait_for_checks")))
		}
	}
	switch {
	case b.loading:
	case b.kind == runner.Ready:
		prose(st.dim, "Its reviewers are asked for a review once it is ready. Each acts only on the head shown when it was read.")
	case b.close != nil:
		prose(st.dim, m.closeProse())
	case b.kind == runner.Review && !b.approves():
		prose(st.dim, "GitHub cannot take a review back. Each is sent on the head shown when it was read.")
	default:
		prose(st.dim, "GitHub cannot take an approval or a merge back. Each acts only on the head shown when it was read.")
	}
	hint := fmt.Sprintf("%s %d of %d? ", verb, goes, len(b.items))
	if len(b.items) == 1 {
		hint = verb + " this pull request? "
	}
	if b.kind == runner.Review && !b.approves() {
		// A verb only for an approval: the others are things sent.
		hint = fmt.Sprintf("send the %s to %d of %d? ", verb, goes, len(b.items))
		if len(b.items) == 1 {
			hint = "send the " + verb + "? "
		}
	}
	if goes == 0 {
		hint = "nothing to do "
	}
	if b.loading {
		// Nothing can be confirmed until every pull request is read again.
		read := 0
		for _, it := range b.items {
			if it.read {
				read++
			}
		}
		what, title := "reading the pull request again…", verb
		if n := len(b.items); n > 1 {
			what, title = fmt.Sprintf("reading %d of %d…", read, n), fmt.Sprintf("%s %d", verb, n)
		}
		foot := []string{m.steady(st.muted, what) +
			st.dim.Render(" · ") + st.key.Render(keymap.Back) + " " + st.muted.Render("cancel")}
		foot, inputAt := m.withCloseFoot(foot, inner)
		out, top := scrollBox(st, title, lines, foot, w, m.popupHeight(), b.offset, keepLines{}, m.scrollKeys())
		b.offset = top
		m.placeCloseInput(out, foot, inputAt)
		return out
	}
	ask := st.sub0.Render(hint) + st.key.Render(m.firstKey("confirm")) + " " + st.muted.Render("yes")
	if b.close != nil {
		for _, a := range []keymap.Action{"delete_branch", "write_comment"} {
			ask += st.dim.Render(" · ") + st.key.Render(m.firstKey(a)) + " " + st.muted.Render(m.keys.Help(a))
		}
	}
	ask += st.dim.Render(" · ") + st.key.Render(keymap.Back) + " " + st.muted.Render("no")
	if b.close != nil && b.close.editing {
		ask = st.key.Render(keymap.Back) + " " + st.muted.Render("done writing")
	}
	foot, inputAt := m.withCloseFoot([]string{"", ask}, inner)
	title := verb
	if len(b.items) > 1 {
		title = fmt.Sprintf("%s %d of %d", verb, goes, len(b.items))
	}
	out, top := scrollBox(st, title, lines, foot, w, m.popupHeight(), b.offset, keepLines{}, m.scrollKeys())
	b.offset = top
	m.placeCloseInput(out, foot, inputAt)
	return out
}

// withCloseFoot puts the check and the text box of a close above foot,
// and says the line of foot the text box starts on; -1 for another kind.
func (m *Model) withCloseFoot(foot []string, inner int) ([]string, int) {
	if m.bulk.close == nil {
		return foot, -1
	}
	pre, at := m.closeFoot(inner)
	return append(pre, foot...), at
}

// placeCloseInput keeps the line of the popup the text box starts on, for
// the cursor: the foot is drawn last, above the bottom edge.
func (m *Model) placeCloseInput(out, foot []string, inputAt int) {
	if inputAt >= 0 {
		f := m.bulk.close
		base := len(out) - 1 - len(foot)
		f.at, f.checkAt = base+inputAt, base+1
	}
}

// bulkRow is a pull request in the confirm popup, styled: what is done
// with it and the notes to read before it is done.
type bulkRow struct {
	pr                model.PR
	mark, title, what string
	notes             []bulkNote
}

type bulkNote struct {
	icon, text string
	style      styleFn
}

// Below these the name and the title are cut too far to tell the pull
// requests apart.
const (
	bulkNameMax  = 36
	bulkTitleMin = 20
)

// layoutBulk lays out the rows of the confirm popup inner cells wide. What
// is done is never cut: it gets its width first, the name its own up to
// bulkNameMax, and the title the rest. When the title would get less than
// bulkTitleMin, and always for a single pull request, what is done goes on
// a line under the pull request's, hung from it like its notes.
func (m *Model) layoutBulk(rows []bulkRow, inner int, single bool) []string {
	st := m.st
	nameW, whatW := 0, 0
	for _, r := range rows {
		nameW = max(nameW, ansi.StringWidth(bulkNumber(r.pr))+ansi.StringWidth(r.pr.Key.Repository()))
		whatW = max(whatW, lipgloss.Width(r.what))
	}
	nameW = min(nameW, bulkNameMax)
	titleW := inner - 2 - nameW - 1 - 2 - whatW
	under := single || titleW < bulkTitleMin
	var lines []string
	for _, r := range rows {
		head := r.mark + " " + m.bulkName(r.pr, nameW) + " "
		var subs []string
		if under {
			head += ansi.Truncate(r.title, max(inner-lipgloss.Width(head), 1), "…")
			subs = append(subs, "")
		} else {
			head += fit(r.title, titleW) + "  " + r.what
		}
		lines = append(lines, head)
		for _, n := range r.notes {
			subs = append(subs, n.icon)
		}
		for i := range subs {
			first, rest := treeLines(st, "  ", i == len(subs)-1)
			if under && i == 0 {
				lines = append(lines, first+r.what)
				continue
			}
			n := r.notes[i-len(subs)+len(r.notes)]
			for j, l := range wrapText(n.text, inner-4-ansi.StringWidth(n.icon)-1) {
				if j == 0 {
					lines = append(lines, first+n.style.Render(n.icon+" "+l))
				} else {
					lines = append(lines, rest+strings.Repeat(" ", ansi.StringWidth(n.icon)+1)+n.style.Render(l))
				}
			}
		}
	}
	return lines
}

// bulkName is the pull request's owner/repo#n w cells wide. It cuts the
// repository, never the number, which is what tells it apart.
func (m *Model) bulkName(pr model.PR, w int) string {
	repo, num := pr.Key.Repository(), bulkNumber(pr)
	if room := w - ansi.StringWidth(num); ansi.StringWidth(repo) > room {
		repo = ansi.Truncate(repo, max(room, 1), "…")
	}
	return fit(m.st.sub1.Render(repo)+m.st.muted.Render(num), w)
}

func bulkNumber(pr model.PR) string { return fmt.Sprintf("#%d", pr.Key.Number) }

// bulkVerbs name what a popup of each kind does, in its title and question.
var bulkVerbs = map[runner.Kind]string{runner.Merge: "merge", runner.Ready: "mark ready", runner.Close: "close"}

func (m *Model) stepStyle(s plan.Step) styleFn {
	switch s {
	case plan.StepMerge:
		return m.st.green
	case plan.StepAutoMerge:
		return m.st.teal
	}
	return m.st.yellow
}
