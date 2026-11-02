package tui

import (
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/babarot/gh-triage/internal/model"
)

type tickMsg struct{}

// tick asks for the next automatic refresh.
func (m *Model) tick() tea.Cmd {
	if m.set.Refresh <= 0 {
		return nil
	}
	return tea.Tick(m.set.Refresh, func(time.Time) tea.Msg { return tickMsg{} })
}

// autoRefresh reads the shown tab and the notifications again. It leaves
// the results on the rows; while a popup is open it waits for the next
// tick, so what is being confirmed does not change under the user.
func (m *Model) autoRefresh() tea.Cmd {
	if m.bulk != nil || m.confirm != nil || m.compose != nil || m.notesAt != nil || m.picker != nil || m.menu != nil || m.view != viewList || m.viewer == "" {
		return m.tick()
	}
	return tea.Batch(m.reload(), m.tick())
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
	// Checks read for ◌ and the preview's merge line may have moved on.
	clear(m.cache.info)
	n := m.clearResults()
	tabs := []int{m.cur}
	if m.cur != m.inbox {
		tabs = append(tabs, m.inbox)
	}
	m.refreshing = &refreshRun{tabs: tabs, cleared: n}
	return m.reload()
}

// endRefresh forgets the reading r started once each tab it reads is
// done, heavy fields included, as the tabs' spinners are.
func (m *Model) endRefresh() {
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

// refreshActivity is the reading r started, while it lasts.
func (m *Model) refreshActivity() string {
	if m.refreshing == nil {
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
