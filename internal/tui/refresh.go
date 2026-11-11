package tui

import (
	"fmt"
	"slices"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/babarot/gh-desk/internal/logging"
	"github.com/babarot/gh-desk/internal/model"
)

type tickMsg struct{}

// tick asks for the next automatic refresh.
func (m *Model) tick() tea.Cmd {
	if m.set.Refresh <= 0 || m.set.Offline {
		return nil
	}
	return m.after(m.set.Refresh, tickMsg{})
}

// autoRefresh reads the shown tab and the notifications again. It leaves
// the results on the rows but those of reviews shown long enough; while a
// popup is open it waits for the next tick, so what is being confirmed
// does not change under the user.
func (m *Model) autoRefresh() tea.Cmd {
	// Rules approve whatever is open: they change no row under the user.
	auto := m.autoApprove()
	if m.bulk != nil || m.compose != nil || m.notesAt != nil || m.picker != nil || m.chooser != nil || m.menu != nil || m.typingChat() || m.viewer == "" {
		return tea.Batch(auto, m.tick())
	}
	m.fadeResults()
	if m.rereading == nil {
		m.rereading = &rereading{by: "clock", start: m.set.Now(), tabs: m.reloadTabs()}
	}
	return tea.Batch(m.reload(), m.tick(), auto)
}

// rereading is a reading of the lists again, by r or by the clock, kept
// until it ends so that it is logged once with how long it took.
type rereading struct {
	by    string
	start time.Time
	tabs  []int
}

// reloadTabs are the tabs reload reads: the shown one and the inbox.
func (m *Model) reloadTabs() []int {
	tabs := []int{m.cur}
	if m.cur != m.inbox {
		tabs = append(tabs, m.inbox)
	}
	return tabs
}

// refreshRun is a reading r started: the tabs it reads, and how many
// results it put away.
type refreshRun struct {
	tabs    []int
	cleared int
}

// refresh is r: read the shown tab and the notifications again, and put
// away the results on the rows.
func (m *Model) refresh() tea.Cmd {
	if m.set.Offline {
		m.refuse("the preview reads nothing: it shows the lists last read")
		return nil
	}
	// Checks read for ◌ and the preview's merge line may have moved on, and
	// so may the pane's description, checks and comments, read once a pull
	// request.
	clear(m.cache.info)
	clear(m.cache.infoRead)
	clear(m.cache.unknown)
	clear(m.cache.details)
	clear(m.cache.comments)
	n := m.clearResults()
	// The rows kept for the cursor are put away with the results.
	for _, t := range m.tabs {
		if t.gone {
			t.at, t.seen, t.gone = nil, false, false
		}
	}
	tabs := m.reloadTabs()
	m.refreshing = &refreshRun{tabs: tabs, cleared: n}
	m.rereading = &rereading{by: "r", start: m.set.Now(), tabs: tabs}
	return tea.Batch(m.reload(), m.autoApprove())
}

// endRefresh forgets the reading r started once each tab it reads is
// done, heavy fields included, as the tabs' spinners are, and logs a
// reading again, by r or the clock, when it ends.
func (m *Model) endRefresh() {
	m.endRereading()
	if m.refreshing == nil {
		return
	}
	for _, i := range m.refreshing.tabs {
		if !m.tabDone(m.tabs[i]) {
			return
		}
	}
	m.refreshing = nil
}

// endRereading logs a reading of the lists again once each tab it reads is
// done: who started it, what it read and how long it took.
func (m *Model) endRereading() {
	r := m.rereading
	if r == nil {
		return
	}
	prs, failed, excluded := 0, 0, 0
	for _, i := range r.tabs {
		t := m.tabs[i]
		if !m.tabDone(t) {
			return
		}
		prs += len(m.allOf(t))
		// Rows the rules leave out, so the log tells why a row is not seen.
		excluded += m.excludedOf(t).n
		if t.failing || t.err != nil {
			failed++
		}
	}
	m.rereading = nil
	m.log.Info("refresh", logging.KeyOp, "refresh", "by", r.by, "tabs", len(r.tabs), "prs", prs, "excluded", excluded, "failed", failed,
		logging.KeyDurationMS, m.set.Now().Sub(r.start).Milliseconds())
}

// refreshActivity is the reading r started, while it lasts.
func (m *Model) refreshActivity() string {
	if m.refreshing == nil {
		return ""
	}
	if text, _, _ := m.tabActivity(); text != "" {
		// The reading of the shown tab says it, with the results cleared.
		return ""
	}
	text := "refreshing…"
	if n := m.refreshing.cleared; n > 0 {
		text += fmt.Sprintf(" · %d results cleared", n)
	}
	return text
}

// reload reads the shown tab again, and the notifications. The cursor
// stays on its pull request (ADR-115).
func (m *Model) reload() tea.Cmd {
	cmds := []tea.Cmd{m.loadTab(m.cur)}
	if !m.tab().inbox {
		cmds = append(cmds, m.loadTab(m.inbox))
	}
	return tea.Batch(cmds...)
}

// settle puts t's cursor back on the pull request the user has seen it on
// after the rows changed under it, rows coming and going above it or
// sorting again. When none of the rows is that pull request any more, its
// row stays where the cursor is, if left is set or it stays already;
// otherwise, and before the user has seen it, the cursor stays at its
// line, on whatever row is there (ADR-115).
func (m *Model) settle(t *tab, left bool) {
	if t.at == nil || !t.seen {
		m.moved(t)
		return
	}
	was := t.gone
	t.gone = false
	rs := m.rows(t)
	if i := slices.IndexFunc(rs, isPR(t.at.pr.Key)); i >= 0 {
		t.sel, t.at = i, &rs[i]
		return
	}
	if (left || was) && m.stays(t) {
		t.gone = true
		t.sel = min(max(t.sel, 0), len(rs))
		t.goneAt = t.sel
		return
	}
	m.moved(t)
}

// moved takes the row under t's cursor as the one it is on, where the
// user or something they did put it.
func (m *Model) moved(t *tab) {
	rs := m.rows(t)
	if len(rs) == 0 {
		t.at, t.seen, t.gone = nil, false, false
		return
	}
	t.sel = min(max(t.sel, 0), len(rs)-1)
	r := rs[t.sel]
	if r.gone {
		return
	}
	t.at, t.seen = nil, false
	if t.gone {
		// The row kept is let go: the rows below it move up a line.
		t.gone = false
		if r.kind == rowPR {
			t.sel = slices.IndexFunc(m.rows(t), isPR(r.pr.Key))
		} else {
			t.sel = slices.IndexFunc(m.rows(t), func(o row) bool { return o.kind == rowGroup && o.section == r.section })
		}
	}
	if r.kind == rowPR {
		t.at = &r
	}
}

// stays reports whether the row of the pull request t's cursor is on may
// stay after it left the rows: not when what the user did leaves it out,
// put away with d or h, filtered out by what is typed or by the scope
// chosen, or in a section folded.
func (m *Model) stays(t *tab) bool {
	p := t.at.pr
	if t.hidden[p.Key] || !matches(p, m.filter) || !m.inScope(p) {
		return false
	}
	return !t.foldedAt(m.sectionOf(t, p))
}

// firstRows puts the cursor of a tab just read on its first pull request,
// unless it is on one already.
func (m *Model) firstRows(t *tab) {
	if t.at == nil {
		m.toFirstPR(t)
	}
}

// dropGone unselects the pull requests a finished reading no longer has,
// and says so.
func (m *Model) dropGone(t *tab) tea.Cmd {
	in := map[model.PRKey]bool{}
	for _, tt := range m.tabs {
		for _, p := range m.allOf(tt) {
			in[p.Key] = true
		}
	}
	var gone []noteItem
	for k := range m.selected {
		if !in[k] {
			delete(m.selected, k)
			gone = append(gone, noteItem{key: k, text: "left the list (merged or closed elsewhere)"})
		}
	}
	if len(gone) == 0 {
		return nil
	}
	return m.pushNote("✗", fmt.Sprintf("%d selected pull requests left the list", len(gone)), gone)
}
