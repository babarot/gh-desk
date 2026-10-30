package tui

import (
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// The mouse points: it picks a tab or a row, opens a row with a double
// click, scrolls, and resizes the preview pane by its top edge. Nothing
// that writes to GitHub is done with it. See ADR-012.

const (
	// defaultPaneHeight is the pane's lines when neither the config nor
	// the last launch gives them.
	defaultPaneHeight = 14
	// minPaneHeight is the fewest lines the pane is shown with.
	minPaneHeight = 6
	// doubleClick is how soon a second click on the same row opens it.
	doubleClick = 500 * time.Millisecond
	// wheelLines is how far a turn of the wheel scrolls.
	wheelLines = 3
)

// click is a left click on a row of the list.
type click struct {
	tab, row int
	at       time.Time
}

// mouse routes a mouse message. A popup or the filter being typed takes
// no clicks; the help goes with one.
func (m *Model) mouse(msg tea.MouseMsg) tea.Cmd {
	switch msg := msg.(type) {
	case tea.MouseMotionMsg:
		if m.dragging {
			// The pane ends over the status and key lines, so its top
			// border at y leaves it this many lines inside.
			m.setPaneHeight(m.height - 4 - msg.Y)
		}
		return nil
	case tea.MouseReleaseMsg:
		if m.dragging {
			m.dragging = false
			return m.savePane()
		}
		return nil
	}
	if m.bulk != nil || m.notesAt != nil || m.picker != nil || m.confirm != nil || m.filtering || m.view == viewCompose {
		return nil
	}
	if m.help {
		if _, ok := msg.(tea.MouseClickMsg); ok {
			m.help = false
		}
		return nil
	}
	switch msg := msg.(type) {
	case tea.MouseWheelMsg:
		return m.wheel(msg)
	case tea.MouseClickMsg:
		if msg.Button != tea.MouseLeft {
			return nil
		}
		switch m.view {
		case viewList:
			return m.listClick(msg.X, msg.Y)
		case viewPR:
			if msg.Y == m.top()+1 {
				if i := spanAt(m.prTabWidths(), 3, msg.X); i >= 0 {
					m.prv.tab, m.prv.scroll = i, 0
				}
			}
		}
	}
	return nil
}

// top is the first line under the header and the filter.
func (m *Model) top() int {
	if m.filtering || m.filter != "" {
		return 2
	}
	return 1
}

func (m *Model) wheel(msg tea.MouseWheelMsg) tea.Cmd {
	step := 0
	switch msg.Button {
	case tea.MouseWheelDown:
		step = 1
	case tea.MouseWheelUp:
		step = -1
	default:
		return nil
	}
	switch m.view {
	case viewPR:
		m.prv.scroll = max(0, m.prv.scroll+step*wheelLines)
	case viewChat:
		// A chat's scroll counts up from its end.
		if c := m.chat(); c != nil {
			c.scroll = max(0, c.scroll-step*wheelLines)
		}
	case viewList:
		l := m.layout()
		if top := m.paneTop(l); l.pane > 0 && msg.Y >= top {
			m.pane.scroll = max(0, m.pane.scroll+step*wheelLines)
			return nil
		}
		return m.moveTo(m.tab().sel + step)
	}
	return nil
}

// paneTop is the line of the pane's top border.
func (m *Model) paneTop(l layout) int { return m.top() + 1 + l.list }

func (m *Model) listClick(x, y int) tea.Cmd {
	if y == 0 {
		var ws []int
		for i := range m.tabs {
			ws = append(ws, lipgloss.Width(m.tabLabel(i)))
		}
		if i := spanAt(ws, 2, x-lipgloss.Width(m.headerLead())); i >= 0 {
			return m.switchTab(i)
		}
		return nil
	}
	l := m.layout()
	if top := m.paneTop(l); l.pane > 0 && y == top {
		// The border carries the pane's tabs; anywhere else on it starts
		// a resize.
		if p, _, ok := m.currentPR(); ok {
			ws := []int{lipgloss.Width(" " + m.diffTabName(p) + " "), lipgloss.Width(" description ")}
			if i := spanAt(ws, 1, x-2); i >= 0 {
				if mode := []paneMode{paneDiff, paneBody}[i]; mode != m.pane.mode {
					m.pane.toggleMode()
					return m.ensure()
				}
				return nil
			}
		}
		m.dragging = true
		return nil
	}
	i, ok := m.rowAt(y, l)
	if !ok {
		return nil
	}
	now := m.set.Now()
	double := m.lastClick.tab == m.cur && m.lastClick.row == i && now.Sub(m.lastClick.at) < doubleClick
	m.lastClick = click{tab: m.cur, row: i, at: now}
	cmd := m.moveTo(i)
	if double {
		m.lastClick = click{}
		if r, ok := m.current(); ok {
			return tea.Batch(cmd, m.openRow(r))
		}
	}
	return cmd
}

// rowAt is the row of the list on line y, as renderList draws it.
func (m *Model) rowAt(y int, l layout) (int, bool) {
	t := m.tab()
	first := m.top() + 1
	if t.err != nil {
		first++
	}
	if !t.loaded && t.loading || y < first || y >= m.top()+1+l.list {
		return 0, false
	}
	i := t.offset + y - first
	return i, i < len(m.rows(t))
}

// spanAt is the index of the span x falls in, for spans of widths ws set
// gap apart from 0; -1 when it falls in none.
func spanAt(ws []int, gap, x int) int {
	at := 0
	for i, w := range ws {
		if x >= at && x < at+w {
			return i
		}
		at += w + gap
	}
	return -1
}

// resizePane sets the pane's height from a key and keeps it.
func (m *Model) resizePane(h int) tea.Cmd {
	if !m.pane.on || m.layout().pane == 0 {
		return nil
	}
	m.setPaneHeight(h)
	return m.savePane()
}

// setPaneHeight sets the pane's height within what the screen has room
// for: the list keeps a third of the lines, as layout leaves it.
func (m *Model) setPaneHeight(h int) {
	rest := m.height - 3 - m.top()
	m.pane.height = max(min(h, rest-rest/3-2), minPaneHeight)
}

// savePane keeps the pane's height for the next launch, away from the
// screen; a failure only costs the next launch the height.
func (m *Model) savePane() tea.Cmd {
	if m.set.Layout == nil {
		return nil
	}
	h, lay, log := m.pane.height, m.set.Layout, m.log
	return func() tea.Msg {
		if err := lay.SavePreviewHeight(h); err != nil {
			log.Warn("could not keep the pane's height", "err", err.Error())
		}
		return nil
	}
}
