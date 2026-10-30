package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/babarot/gh-triage/internal/keymap"
	"github.com/babarot/gh-triage/internal/model"
	"github.com/babarot/gh-triage/internal/plan"
	"github.com/babarot/gh-triage/internal/runner"
)

// bulk is the confirm popup of an approve or a merge, of one pull request
// or of every one selected. It reads each pull request again, decides what
// to do with it, and lists that, with the reason for every one it skips.
type bulk struct {
	kind           runner.Kind
	items          []bulkItem
	loading        bool
	err            error
	offset         int
	includeFailing bool
}

type bulkItem struct {
	pr model.PR
	mi model.MergeInfo
	ok bool
	d  plan.Decision
	// moved is set when the head is not the one the list read, so the
	// diff the user looked at is not what would be approved or merged.
	moved bool
}

// goes reports whether the item is acted on.
func (it bulkItem) goes(kind runner.Kind) bool {
	switch {
	case !it.ok || it.moved:
		return false
	case kind == runner.Approve:
		return it.d.Approve
	}
	return it.d.Step != plan.StepSkip
}

func (it bulkItem) why() string {
	switch {
	case !it.ok:
		return "could not read it"
	case it.moved:
		return "changed since it was read · R to read again"
	}
	return it.d.Reason
}

type bulkInfoMsg struct {
	b   *bulk
	got map[string]model.MergeInfo
	err error
}

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
				if m.selected[p.Key] && !containsKey(prs, p.Key) {
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
	if len(prs) == 0 {
		return nil
	}
	b := &bulk{kind: kind, loading: true}
	var ids []string
	for _, p := range prs {
		b.items = append(b.items, bulkItem{pr: p})
		if p.ID != "" {
			ids = append(ids, p.ID)
		}
	}
	m.bulk = b
	return func() tea.Msg {
		got, err := m.src.MergeInfo(m.ctx, ids, true)
		return bulkInfoMsg{b, got, err}
	}
}

func containsKey(prs []model.PR, k model.PRKey) bool {
	for _, p := range prs {
		if p.Key == k {
			return true
		}
	}
	return false
}

// allOf is every pull request a tab holds, filtered or not.
func (m *Model) allOf(t *tab) []model.PR {
	if !t.inbox {
		return t.prs
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
	b.loading, b.err = false, msg.err
	for i := range b.items {
		it := &b.items[i]
		it.mi, it.ok = msg.got[it.pr.ID]
		if it.ok {
			m.cache.info[it.pr.Key] = it.mi
			it.moved = it.mi.State == model.StateOpen && it.mi.HeadOID != it.pr.HeadOID
		}
	}
	m.decideBulk()
	return nil
}

func (m *Model) decideBulk() {
	b := m.bulk
	for i := range b.items {
		it := &b.items[i]
		if !it.ok {
			continue
		}
		if b.kind == runner.Approve {
			it.d = plan.ForApprove(it.mi, it.pr, m.viewer, b.includeFailing, m.set.Plan)
		} else {
			it.d = plan.ForMerge(it.mi, it.pr, m.viewer, m.set.Plan)
		}
	}
}

const bulkRows = 12

func (m *Model) bulkKey(k string) tea.Cmd {
	b := m.bulk
	is := func(a keymap.Action) bool { return m.keys.Matches(a, k) }
	switch {
	case k == keymap.Back:
		m.bulk = nil
		m.setStatus("cancelled", false)
	case is("down"):
		b.offset = min(b.offset+1, max(len(b.items)-bulkRows, 0))
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
		p := progress{text: "– " + it.why(), result: runner.Skipped}
		m.progress[it.pr.Key] = p
		bs.skipped = append(bs.skipped, noteItem{key: it.pr.Key, text: "skipped: " + it.why(), result: runner.Skipped})
	}
	m.bulk = nil
	if len(items) == 0 {
		m.setStatus("nothing to do", false)
		return nil
	}
	for _, it := range items {
		m.progress[it.PR.Key] = progress{text: "queued"}
		if b.kind == runner.Merge {
			delete(m.selected, it.PR.Key)
		}
	}
	id, ch := m.run.Start(m.ctx, b.kind, items)
	m.batches[id] = bs
	verb := map[runner.Kind]string{runner.Approve: "approving", runner.Merge: "approving and merging"}[b.kind]
	m.setStatus(fmt.Sprintf("%s %d pull requests · progress shows on their rows", verb, len(items)), false)
	return listen(id, ch)
}

// renderBulk draws the confirm popup.
func (m *Model) renderBulk() []string {
	b, st := m.bulk, m.st
	w := min(max(m.width-6, 40), 130)
	verb := map[runner.Kind]string{runner.Approve: "approve", runner.Merge: "merge"}[b.kind]
	if b.kind == runner.Merge {
		for _, it := range b.items {
			if it.ok && it.d.Approve && it.goes(b.kind) {
				verb = "approve and merge"
				break
			}
		}
	}
	var lines []string
	if b.loading {
		return box(st, verb, []string{st.dim.Render(fmt.Sprintf("reading %d pull requests…", len(b.items)))}, w)
	}
	if b.err != nil {
		lines = append(lines, st.red.Render("could not read them all: "+b.err.Error()))
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
	end := min(len(b.items), b.offset+bulkRows)
	for _, it := range b.items[b.offset:end] {
		mark, title := st.dim.Render("·"), st.dim.Render(it.pr.Title)
		var what string
		if it.goes(b.kind) {
			mark, title = st.mauve.Render("✓"), st.sub1.Render(it.pr.Title)
			switch {
			case b.kind == runner.Approve:
				what = st.green.Render("approve")
			default:
				if it.d.Approve {
					what = st.green.Render("approve") + st.dim.Render(" → ")
				}
				what += m.stepStyle(it.d.Step).Render(it.d.Reason) + " " + st.dim.Render("("+strings.ToLower(string(it.d.Method))+")")
			}
		} else {
			what = st.dim.Render("skip · ") + st.red.Render(it.why())
		}
		if len(it.d.Notes) > 0 && it.goes(b.kind) {
			what += " " + st.yellow.Render("⚠ "+it.d.Notes[0])
		}
		name := fit(st.sub1.Render(it.pr.Key.Repository())+st.muted.Render(fmt.Sprintf("#%d", it.pr.Key.Number)), 30)
		lines = append(lines, mark+" "+name+" "+fit(title, max(w-80, 20))+"  "+what)
	}
	lines = append(lines, "")
	if auto {
		lines = append(lines, st.dim.Render("Auto-merge also merges commits pushed after it is on, as GitHub does."))
	}
	if b.kind == runner.Approve && failing > 0 {
		state := "left out"
		if b.includeFailing {
			state = "included"
		}
		lines = append(lines, st.dim.Render(fmt.Sprintf("%d with failing checks are %s · %s to change", failing, state, m.firstKey("include_failing"))))
	}
	lines = append(lines, st.dim.Render("GitHub cannot take an approval or a merge back. Each acts only on the head shown when it was read."))
	hint := fmt.Sprintf("%s %d of %d? ", verb, goes, len(b.items))
	if len(b.items) == 1 {
		hint = verb + " this pull request? "
	}
	if goes == 0 {
		hint = "nothing to do "
	}
	scroll := ""
	if len(b.items) > bulkRows {
		scroll = st.dim.Render(fmt.Sprintf("  %d–%d of %d · %s/%s", b.offset+1, end, len(b.items), m.firstKey("down"), m.firstKey("up")))
	}
	lines = append(lines, "", st.sub0.Render(hint)+st.key.Render(m.firstKey("confirm"))+" "+st.muted.Render("yes")+
		st.dim.Render(" · ")+st.key.Render(keymap.Back)+" "+st.muted.Render("no")+scroll)
	title := verb
	if len(b.items) > 1 {
		title = fmt.Sprintf("%s %d of %d", verb, goes, len(b.items))
	}
	return box(st, title, lines, w)
}

func (m *Model) stepStyle(s plan.Step) styleFn {
	switch s {
	case plan.StepMerge:
		return m.st.green
	case plan.StepAutoMerge:
		return m.st.teal
	}
	return m.st.yellow
}
