package tui

import (
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/babarot/gh-desk/internal/logging"
	"github.com/babarot/gh-desk/internal/model"
)

type tickMsg struct{}

// tick asks for the next automatic refresh.
func (m *Model) tick() tea.Cmd {
	if m.set.Refresh <= 0 {
		return nil
	}
	return m.after(m.set.Refresh, tickMsg{})
}

// autoRefresh reads the shown tab and the notifications again. It leaves
// the results on the rows; while a popup is open it waits for the next
// tick, so what is being confirmed does not change under the user.
func (m *Model) autoRefresh() tea.Cmd {
	if m.bulk != nil || m.confirm != nil || m.compose != nil || m.notesAt != nil || m.picker != nil || m.menu != nil || m.typingChat() || m.viewer == "" {
		return m.tick()
	}
	if m.rereading == nil {
		m.rereading = &rereading{by: "clock", start: m.set.Now(), tabs: m.reloadTabs()}
	}
	return tea.Batch(m.reload(), m.tick())
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
	// Checks read for ◌ and the preview's merge line may have moved on, and
	// so may the pane's description, checks and comments, read once a pull
	// request.
	clear(m.cache.info)
	clear(m.cache.details)
	clear(m.cache.comments)
	n := m.clearResults()
	tabs := m.reloadTabs()
	m.refreshing = &refreshRun{tabs: tabs, cleared: n}
	m.rereading = &rereading{by: "r", start: m.set.Now(), tabs: tabs}
	return m.reload()
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
	prs, failed := 0, 0
	for _, i := range r.tabs {
		t := m.tabs[i]
		if !m.tabDone(t) {
			return
		}
		prs += len(m.allOf(t))
		if t.failing || t.err != nil {
			failed++
		}
	}
	m.rereading = nil
	m.log.Info("refresh", logging.KeyOp, "refresh", "by", r.by, "tabs", len(r.tabs), "prs", prs, "failed", failed,
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

// reload reads the shown tab again, keeping the cursor on the pull
// request it is on, and the notifications.
func (m *Model) reload() tea.Cmd {
	t := m.tab()
	t.keep = model.PRKey{}
	if r, ok := m.current(); ok && r.kind == rowPR {
		t.keep = r.pr.Key
	}
	cmds := []tea.Cmd{m.loadTab(m.cur)}
	if !t.inbox {
		cmds = append(cmds, m.loadTab(m.inbox))
	}
	return tea.Batch(cmds...)
}

// holdCursor notes the pull request under t's cursor. The func it returns
// puts the cursor back on that pull request when a row still shows it, and
// leaves it at its line otherwise, so that rows coming and going above it
// do not move it onto another pull request.
func (m *Model) holdCursor(t *tab) func() {
	rs := m.rows(t)
	if t.sel < 0 || t.sel >= len(rs) || rs[t.sel].kind != rowPR {
		return func() {}
	}
	k := rs[t.sel].pr.Key
	return func() {
		for i, r := range m.rows(t) {
			if r.kind == rowPR && r.pr.Key == k {
				t.sel = i
				return
			}
		}
	}
}

// restoreCursor puts the cursor back on the pull request it was on before
// a reading, or on the row it was at when that one is gone.
func (m *Model) restoreCursor(t *tab) {
	if t.keep == (model.PRKey{}) {
		m.toFirstPR(t)
		return
	}
	for i, r := range m.rows(t) {
		if r.kind == rowPR && r.pr.Key == t.keep {
			t.sel = i
			return
		}
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
