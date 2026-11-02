package tui

import (
	"fmt"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/babarot/gh-triage/internal/claude"
	"github.com/babarot/gh-triage/internal/keymap"
	"github.com/babarot/gh-triage/internal/model"
	"github.com/babarot/gh-triage/internal/plan"
	"github.com/babarot/gh-triage/internal/runner"
)

// bulk is the confirm popup of an approve, a merge or a ready for review,
// of one pull request
// or of every one selected. It reads each pull request again, decides what
// to do with it, and lists that, with the reason for every one it skips.
type bulk struct {
	kind  runner.Kind
	items []bulkItem
	// loading is set until every read is back; left counts them.
	loading bool
	left    int
	err     error
	// offset is the first line of the popup's body shown.
	offset         int
	includeFailing bool
	// notes is how far Claude has read the release notes of the bot pull
	// requests to merge, and notesErr why it could not.
	notes    notesState
	notesErr string
	// by is the request Claude picked the pull requests for, empty when
	// the user picked them.
	by string
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
	// noWait is set when the merge would wait while another gh-triage
	// holds the record of pending merges.
	noWait bool
}

// goes reports whether the item is acted on.
func (it bulkItem) goes(kind runner.Kind) bool {
	switch {
	case !it.ok || it.moved || it.running || it.noWait:
		return false
	case kind == runner.Approve:
		return it.d.Approve
	}
	return it.d.Step != plan.StepSkip
}

// why is why it is skipped; refresh names the key that reads it again.
func (it bulkItem) why(refresh string) string {
	switch {
	case !it.ok:
		return "could not read it"
	case it.moved:
		return "changed since it was read · " + refresh + " to read again"
	case it.running:
		return "already running"
	case it.noWait:
		return "would wait, and another gh-triage keeps the waiting merges"
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

// startBulk opens the popup for the selected pull requests, or for the one
// under the cursor when none is selected.
func (m *Model) startBulk(kind runner.Kind) tea.Cmd {
	if m.busy() {
		return nil
	}
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
		if kind == runner.Approve && m.own(p) {
			m.setStatus("you cannot approve your own pull request", true)
			return nil
		}
		prs = []model.PR{p}
	}
	return m.startBulkFor(kind, prs)
}

// startBulkFor opens the confirm popup for prs.
func (m *Model) startBulkFor(kind runner.Kind, prs []model.PR) tea.Cmd {
	if len(prs) == 0 {
		return nil
	}
	b := &bulk{kind: kind, loading: true}
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
		if _, known := m.cautions[notesID(it.pr)]; it.goes(b.kind) && it.pr.AuthorIsBot && !known {
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
	msg.b.notes = notesRead
	return nil
}

func (m *Model) decideBulk() {
	b := m.bulk
	for i := range b.items {
		it := &b.items[i]
		if !it.ok {
			continue
		}
		switch b.kind {
		case runner.Approve:
			it.d = plan.ForApprove(it.mi, it.pr, m.viewer, b.includeFailing, m.set.Plan)
		case runner.Ready:
			it.d = plan.ForReady(it.mi)
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
		m.bulk = nil
		m.setStatus("canceled", false)
	case is("down"):
		// Kept within the body when it is drawn.
		b.offset++
	case is("up"):
		b.offset = max(b.offset-1, 0)
	case is("include_failing") && b.kind == runner.Approve && !b.loading:
		b.includeFailing = !b.includeFailing
		m.decideBulk()
	case is("confirm") && !b.loading:
		return m.runBulk()
	}
	return nil
}

// runBulk starts the batch: what it skips leaves the selection at once and
// keeps its reason on its row; what it acts on shows its progress there.
func (m *Model) runBulk() tea.Cmd {
	b := m.bulk
	var items []runner.Item
	bs := &batchState{kind: b.kind, goes: map[model.PRKey]bool{}, done: map[model.PRKey]progress{}}
	for _, it := range b.items {
		if it.goes(b.kind) {
			items = append(items, runner.Item{PR: it.pr, Head: it.mi.HeadOID, AutoMerge: it.d.Step == plan.StepAutoMerge})
			bs.goes[it.pr.Key] = true
			bs.order = append(bs.order, it.pr.Key)
			continue
		}
		delete(m.selected, it.pr.Key)
		why := it.why(m.firstKey("refresh"))
		m.progress[it.pr.Key] = progress{text: "– " + why, result: runner.Skipped}
		bs.skipped = append(bs.skipped, noteItem{key: it.pr.Key, text: "skipped: " + why, result: runner.Skipped})
	}
	m.bulk = nil
	if len(items) == 0 {
		m.setStatus("nothing to do", false)
		return nil
	}
	for _, it := range items {
		m.progress[it.PR.Key] = progress{text: "queued"}
		if b.kind != runner.Approve {
			delete(m.selected, it.PR.Key)
		}
	}
	id, ch := m.run.Start(m.ctx, b.kind, items)
	m.batches[id] = bs
	verb := map[runner.Kind]string{runner.Approve: "approving", runner.Merge: "approving and merging", runner.Ready: "marking ready"}[b.kind]
	m.setStatus(fmt.Sprintf("%s %d pull requests · progress shows on their rows", verb, len(items)), false)
	return listen(id, ch)
}

// renderBulk draws the confirm popup.
func (m *Model) renderBulk() []string {
	b, st := m.bulk, m.st
	w := min(max(m.width-6, 40), 130)
	verb := bulkVerbs[b.kind]
	if b.kind == runner.Merge {
		for _, it := range b.items {
			if it.ok && it.d.Approve && it.goes(b.kind) {
				verb = "approve and merge"
				break
			}
		}
	}
	inner := w - 4
	var lines []string
	if b.by != "" {
		lines = append(lines, hang(st.teal.Render("✧ picked by Claude")+st.muted.Render(": "), st.muted.Render("“"+safeLine(firstLine(b.by))+"”"), inner)...)
		lines = append(lines, "")
	}
	if b.err != nil {
		lines = append(lines, wrapped(st.red, "could not read them all: "+b.err.Error(), inner)...)
	}
	goes := 0
	auto := false
	failing := 0
	for _, it := range b.items {
		if it.goes(b.kind) {
			goes++
			auto = auto || it.d.Step == plan.StepAutoMerge
		}
		if it.ok && plan.Failing(it.mi) {
			failing++
		}
	}
	rows := make([]bulkRow, 0, len(b.items))
	for _, it := range b.items {
		mark, title := st.dim.Render("·"), st.dim.Render(it.pr.Title)
		var what string
		switch {
		case !it.read:
			// Shown from the list until it is read again.
			mark, title, what = st.dim.Render(m.spinner()), st.muted.Render(it.pr.Title), st.dim.Render("reading…")
		case it.goes(b.kind):
			mark, title = st.mauve.Render("✓"), st.sub1.Render(it.pr.Title)
			switch b.kind {
			case runner.Approve:
				what = st.green.Render("approve")
			case runner.Ready:
				what = st.green.Render(it.d.Reason)
			default:
				if it.d.Approve {
					what = st.green.Render("approve") + st.dim.Render(" → ")
				}
				what += m.stepStyle(it.d.Step).Render(it.d.Reason) + " " + st.dim.Render("("+methodWord(it.d.Method)+")")
			}
		default:
			what = st.dim.Render("skip · ") + st.red.Render(it.why(m.firstKey("refresh")))
		}
		r := bulkRow{pr: it.pr, mark: mark, title: title, what: what}
		if it.goes(b.kind) {
			// On lines of their own, so a long one does not take the
			// width of the title or of what is done.
			if len(it.d.Notes) > 0 {
				r.notes = append(r.notes, bulkNote{"⚠", it.d.Notes[0], st.yellow})
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
	switch b.notes {
	case notesReading:
		prose(st.dim, "Claude is reading the release notes of the bot pull requests…")
	case notesRead:
		n := 0
		for _, it := range b.items {
			if m.cautions[notesID(it.pr)] != "" && it.goes(b.kind) {
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
	if b.kind == runner.Approve && failing > 0 && !b.loading {
		state := "left out"
		if b.includeFailing {
			state = "included"
		}
		prose(st.dim, fmt.Sprintf("%d with failing checks are %s · %s to change", failing, state, m.firstKey("include_failing")))
	}
	switch {
	case b.loading:
	case b.kind == runner.Ready:
		prose(st.dim, "Its reviewers are asked for a review once it is ready. Each acts only on the head shown when it was read.")
	default:
		prose(st.dim, "GitHub cannot take an approval or a merge back. Each acts only on the head shown when it was read.")
	}
	hint := fmt.Sprintf("%s %d of %d? ", verb, goes, len(b.items))
	if len(b.items) == 1 {
		hint = verb + " this pull request? "
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
		what := "reading the pull request again…"
		if len(b.items) > 1 {
			what = fmt.Sprintf("reading %d pull requests again… %d of %d read", len(b.items), read, len(b.items))
		}
		foot := []string{st.mauve.Render(m.spinner()) + " " + st.muted.Render(what) +
			st.dim.Render(" · ") + st.key.Render(keymap.Back) + " " + st.muted.Render("cancel")}
		title := verb + " " + m.spinner()
		if len(b.items) > 1 {
			title = fmt.Sprintf("%s %s of %d", verb, m.spinner(), len(b.items))
		}
		out, top := scrollBox(st, title, lines, foot, w, m.popupHeight(), b.offset, keepLines{}, m.scrollKeys())
		b.offset = top
		return out
	}
	foot := []string{"", st.sub0.Render(hint) + st.key.Render(m.firstKey("confirm")) + " " + st.muted.Render("yes") +
		st.dim.Render(" · ") + st.key.Render(keymap.Back) + " " + st.muted.Render("no")}
	title := verb
	if len(b.items) > 1 {
		title = fmt.Sprintf("%s %d of %d", verb, goes, len(b.items))
	}
	out, top := scrollBox(st, title, lines, foot, w, m.popupHeight(), b.offset, keepLines{}, m.scrollKeys())
	b.offset = top
	return out
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
			first, rest := "  ├ ", "  │ "
			if i == len(subs)-1 {
				first, rest = "  └ ", "    "
			}
			first, rest = st.border.Render(first), st.border.Render(rest)
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
var bulkVerbs = map[runner.Kind]string{runner.Approve: "approve", runner.Merge: "merge", runner.Ready: "mark ready"}

func (m *Model) stepStyle(s plan.Step) styleFn {
	switch s {
	case plan.StepMerge:
		return m.st.green
	case plan.StepAutoMerge:
		return m.st.teal
	}
	return m.st.yellow
}
