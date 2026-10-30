package tui

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/babarot/gh-triage/internal/keymap"
	"github.com/babarot/gh-triage/internal/model"
	"github.com/babarot/gh-triage/internal/plan"
	"github.com/babarot/gh-triage/internal/runner"
)

type styleFn = lipgloss.Style

// progress is a pull request's step or result in a batch, written on its
// row over WHY and SIZE until R clears it.
type progress struct {
	text   string
	result runner.Result
	watch  bool
}

// batchState follows a batch until every pull request it acts on has a
// result, then writes one notification for it.
type batchState struct {
	kind    runner.Kind
	goes    map[model.PRKey]bool
	order   []model.PRKey
	done    map[model.PRKey]progress
	skipped []noteItem
	noted   bool
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
	if m.set.StoreProblem != "" {
		m.setStatus("merges that wait are off: "+m.set.StoreProblem, true)
	}
	if len(m.set.Resume) == 0 {
		return nil
	}
	bs := &batchState{kind: runner.Merge, goes: map[model.PRKey]bool{}, done: map[model.PRKey]progress{}}
	for _, p := range m.set.Resume {
		bs.goes[p.Key] = true
		bs.order = append(bs.order, p.Key)
		m.progress[p.Key] = progress{text: "resuming"}
	}
	id, ch := m.run.Resume(m.ctx, m.set.Resume)
	m.batches[id] = bs
	m.setStatus(fmt.Sprintf("going on with %d merges from the last run", len(m.set.Resume)), false)
	return listen(id, ch)
}

// event takes in a step of a batch.
func (m *Model) event(msg eventMsg) tea.Cmd {
	if !msg.ok {
		return nil
	}
	ev := msg.ev
	cmds := []tea.Cmd{listen(msg.batch, msg.ch)}
	m.progress[ev.Key] = progress{text: ev.Text, result: ev.Result, watch: ev.Watch}
	if ev.Approved {
		m.markApproved(ev.Key)
	}
	if ev.Result == runner.Merged {
		m.merged[ev.Key] = true
	}
	switch ev.Result {
	case runner.Approved, runner.Merged, runner.AutoMergeOn:
		cmds = append(cmds, m.doneAfterReview(ev.Key))
	}
	bs := m.batches[msg.batch]
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
		bs.noted = true
		cmds = append(cmds, m.noteBatch(bs))
	}
	return tea.Batch(cmds...)
}

func (m *Model) markApproved(k model.PRKey) {
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
	p.Reviews = append(rs, model.Review{Author: m.viewer, State: model.ReviewApproved})
	m.updatePR(p)
}

// doneAfterReview marks the pull request's notification done when the
// settings say so.
func (m *Model) doneAfterReview(k model.PRKey) tea.Cmd {
	n, ok := m.unread[k]
	if !ok || !m.set.DoneAfterReview {
		return nil
	}
	delete(m.unread, k)
	m.inboxTab().hidden[k] = true
	return func() tea.Msg { return doneMsg{n, m.src.MarkDone(m.ctx, n)} }
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
	if bs.kind == runner.Approve {
		parts = append(parts, fmt.Sprintf("approved %d", count[runner.Approved]))
	} else {
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
	verb := map[runner.Kind]string{runner.Approve: "approve", runner.Merge: "merge"}[bs.kind]
	return m.pushNote(icon, verb+" done: "+strings.Join(parts, ", "), items)
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

// note is an in-app notification: kept for the session, so a result that
// came while the user looked elsewhere can still be found.
type note struct {
	at    time.Time
	icon  string
	title string
	items []noteItem
	seen  bool
}

type noteItem struct {
	key    model.PRKey
	text   string
	result runner.Result
}

type notesView struct{ sel int }

type toastMsg struct{ n *note }

func (m *Model) pushNote(icon, title string, items []noteItem) tea.Cmd {
	n := &note{at: m.set.Now(), icon: icon, title: title, items: items}
	m.notes = append([]*note{n}, m.notes...)
	m.toast = n
	return tea.Tick(4*time.Second, func(time.Time) tea.Msg { return toastMsg{n} })
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
	case k == keymap.Back, is("notifications"), is("quit"):
		m.notesAt = nil
	case is("down"):
		v.sel = min(v.sel+1, max(len(ls)-1, 0))
	case is("up"):
		v.sel = max(v.sel-1, 0)
	case is("choose") && v.sel < len(ls):
		l := ls[v.sel]
		key := model.PRKey{}
		switch {
		case l.item != nil:
			key = l.item.key
		case len(l.n.items) > 0:
			key = l.n.items[0].key
		}
		m.notesAt = nil
		if key != (model.PRKey{}) {
			return m.jumpTo(key)
		}
	}
	return nil
}

// jumpTo puts the cursor on k in whichever tab has it, unfolding its group
// and clearing the filter.
func (m *Model) jumpTo(k model.PRKey) tea.Cmd {
	m.filter, m.filtering = "", false
	for i, t := range m.tabs {
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
				m.view = viewList
				m.pane.scroll = 0
				return m.ensure()
			}
		}
	}
	m.setStatus(fmt.Sprintf("%s is no longer in any tab", k), false)
	return nil
}

func (m *Model) renderNotes() []string {
	st := m.st
	w := min(max(m.width-8, 40), 110)
	ls := m.noteLines()
	v := m.notesAt
	var lines []string
	if len(ls) == 0 {
		lines = append(lines, st.dim.Render("nothing yet"))
	}
	h := max(m.height-8, 5)
	off := max(0, min(v.sel-h/2, len(ls)-h))
	for i := off; i < len(ls) && i < off+h; i++ {
		l := ls[i]
		cursor := " "
		if i == v.sel {
			cursor = st.blue.Render("▎")
		}
		var s string
		if l.item == nil {
			s = cursor + " " + m.noteIcon(l.n.icon) + " " + st.sub1.Render(l.n.title) + "  " + st.dim.Render(shortAge(m.set.Now().Sub(l.n.at))+" ago")
		} else {
			name := st.sub1.Render(l.item.key.Repository()) + st.muted.Render(fmt.Sprintf("#%d", l.item.key.Number))
			s = cursor + "     " + fit(name, 30) + " " + m.resultStyle(l.item.result).Render(l.item.text)
		}
		if i == v.sel {
			s = onBackground(s, st.sel, w-4)
		}
		lines = append(lines, s)
	}
	lines = append(lines, "", st.key.Render(m.firstKey("choose"))+" "+st.muted.Render("go to the pull request")+
		st.dim.Render(" · ")+st.key.Render(keymap.Back)+" "+st.muted.Render("close"))
	return box(st, "notifications", lines, w)
}

func (m *Model) noteIcon(icon string) string {
	if icon == "✗" {
		return m.st.red.Render(icon)
	}
	return m.st.green.Render(icon)
}

func (m *Model) resultStyle(r runner.Result) lipgloss.Style {
	switch r {
	case runner.Merged, runner.Approved:
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
	icon := map[runner.Result]string{runner.Running: "◌", runner.Approved: "✓", runner.Merged: "✓", runner.AutoMergeOn: "⧗",
		runner.Skipped: "–", runner.Stopped: "✗", runner.Failed: "✗"}[p.result]
	text := p.text
	if strings.HasPrefix(text, "– ") {
		text = strings.TrimPrefix(text, "– ")
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
	for _, r := range []runner.Result{runner.Merged, runner.Approved, runner.AutoMergeOn, runner.Running, runner.Skipped, runner.Stopped, runner.Failed} {
		if count[r] == 0 {
			continue
		}
		parts = append(parts, m.progressTextIcon(r, count[r]))
	}
	return strings.Join(parts, " ") + " " + m.st.dim.Render(m.firstKey("refresh")+" to clear")
}

func (m *Model) progressTextIcon(r runner.Result, n int) string {
	icon := map[runner.Result]string{runner.Running: "◌", runner.Approved: "✓", runner.Merged: "✓", runner.AutoMergeOn: "⧗",
		runner.Skipped: "–", runner.Stopped: "✗", runner.Failed: "✗"}[r]
	return m.resultStyle(r).Render(fmt.Sprintf("%s%d", icon, n))
}
