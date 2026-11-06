package tui

import (
	"cmp"
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/babarot/gh-desk/internal/model"
	"github.com/babarot/gh-desk/internal/tui/keymap"
)

// The mouse points: it picks a tab or a row, folds a group with a click,
// opens a pull request with a double click, opens a page in the browser
// with a second click on its link, scrolls, resizes the preview pane by
// its top edge, and picks in a popup or closes it. Nothing that writes to
// GitHub is done with it. See ADR-012, ADR-035 and ADR-053.

const (
	// minPaneHeight is the fewest lines the pane is shown with.
	minPaneHeight = 6
	// doubleClick is how soon a second click on the same row opens it.
	doubleClick = 500 * time.Millisecond
	// wheelLines is how far a turn of the wheel scrolls.
	wheelLines = 3
	// armedFor is how long a link clicked once waits for the click that
	// opens it.
	armedFor = 3 * time.Second
)

// spot is a link on the screen: the page it opens, and the cells x to
// x+w of line y it is drawn in. row is the row of the list it is on, -1
// off the list.
type spot struct {
	url     string
	x, y, w int
	row     int
}

// armed is a link clicked once, which a click on it before until opens.
type armed struct {
	spot
	until time.Time
}

// disarmMsg comes when a link clicked once may have stopped waiting.
type disarmMsg struct{}

// click is a left click on a row of the list.
type click struct {
	tab, row int
	at       time.Time
}

// mouse routes a mouse message. A popup takes it before what is behind
// it; see popupMouse. The filter being typed takes none.
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
	// Anything but a click on the link waiting for its second click
	// leaves it.
	prev := m.armed
	m.armed = nil
	if cmd, ok := m.popupMouse(msg); ok {
		return cmd
	}
	if m.filtering {
		return nil
	}
	if m.logMouse(msg) {
		return nil
	}
	// Pointing at the list or the header above the claude pane goes back
	// to them, as the list takes no keys while the pane is shown.
	var back tea.Cmd
	if m.claudeShown() && msg.Mouse().Y < m.paneTop(m.layout()) {
		back = m.closeChat()
	}
	switch msg := msg.(type) {
	case tea.MouseWheelMsg:
		return tea.Batch(back, m.wheel(msg))
	case tea.MouseClickMsg:
		if msg.Button != tea.MouseLeft {
			return back
		}
		return tea.Batch(back, m.listClick(msg.X, msg.Y, prev))
	}
	return back
}

// top is the first line under the header and the filter.
func (m *Model) top() int {
	if m.filterLine() {
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
	l := m.layout()
	if top := m.paneTop(l); l.pane > 0 && msg.Y >= top {
		if !m.claudeOpen {
			m.pane.scroll = max(0, m.pane.scroll+step*wheelLines)
		} else if c := m.chat(); c != nil {
			// A chat's scroll counts up from its end.
			c.scroll = max(0, c.scroll-step*wheelLines)
		}
		return nil
	}
	return m.moveTo(m.tab().sel + step)
}

// paneTop is the line of the pane's top border.
func (m *Model) paneTop(l layout) int { return m.top() + 1 + l.list }

// listClick is a click on the list screen. One on a link waits for a
// second there to open its page; see clickLink.
func (m *Model) listClick(x, y int, prev *armed) tea.Cmd {
	if s, ok := m.linkUnder(x, y); ok {
		m.lastClick = click{}
		var cmd tea.Cmd
		if s.row >= 0 {
			cmd = m.moveTo(s.row)
		}
		return tea.Batch(cmd, m.clickLink(s, prev))
	}
	if y == 0 {
		if x0, w, ok := m.eventsAt(); ok && x >= x0 && x < x0+w {
			m.lastClick = click{}
			m.openNotes()
			return nil
		}
		from, to := m.shownTabs()
		var ws []int
		for i := from; i < to; i++ {
			ws = append(ws, lipgloss.Width(m.tabLabel(i)))
		}
		if i := spanAt(ws, len(tabGap), x-lipgloss.Width(m.headerLead())-lipgloss.Width(tabsBefore(from))); i >= 0 {
			return m.switchTab(from + i)
		}
		return nil
	}
	l := m.layout()
	if top := m.paneTop(l); l.pane > 0 && y == top {
		// The border carries the pane's tabs; anywhere else on it starts
		// a resize.
		if p, _, ok := m.currentPR(); ok && !m.claudeOpen {
			ws := []int{lipgloss.Width(" overview "), lipgloss.Width(" " + m.diffTabName(p) + " ")}
			if i := spanAt(ws, len(tabGap), x-2); i >= 0 {
				if mode := []paneMode{paneOverview, paneDiff}[i]; mode != m.pane.mode {
					m.pane.toggleMode()
					return m.ensure()
				}
				return nil
			}
		}
		m.dragging = true
		return nil
	}
	// A click on a file tab of the diff shows it, and one on the header
	// of a part, or of a file of the diff, folds it.
	if top := m.paneTop(l); l.pane > 0 && y > top {
		if m.claudeOpen {
			return nil
		}
		r := y - top - 1
		if m.pane.mode == paneDiff && r == m.pane.tabRow {
			// The pane's border and a space come before its lines.
			m.clickFileTab(x - 2)
			return nil
		}
		if r < len(m.pane.heads) && m.pane.heads[r] != "" {
			if m.pane.mode == paneOverview {
				m.pane.folded[m.pane.heads[r]] = !m.pane.folded[m.pane.heads[r]]
			} else if p, _, ok := m.currentPR(); ok {
				m.foldFile(p, m.pane.heads[r])
			}
		}
		return nil
	}
	i, ok := m.rowAt(y, l)
	if !ok {
		return nil
	}
	// A click on a group folds or unfolds it, as one on a header of the
	// pane does.
	if rs := m.rows(m.tab()); rs[i].kind == rowGroup {
		m.lastClick = click{}
		cmd := m.moveTo(i)
		return tea.Batch(cmd, m.openRow(rs[i]))
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

// clickLink opens the page of the link s when it is the one prev waits
// on, and otherwise makes s wait for a second click, so that a stray
// click does not throw the user out to the browser.
func (m *Model) clickLink(s spot, prev *armed) tea.Cmd {
	now := m.set.Now()
	if prev != nil && prev.spot == s && now.Before(prev.until) {
		return m.browseURL(s.url)
	}
	m.armed = &armed{spot: s, until: now.Add(armedFor)}
	return m.after(armedFor, disarmMsg{})
}

// disarm lets go of a link whose wait is over.
func (m *Model) disarm() {
	if m.armed != nil && !m.set.Now().Before(m.armed.until) {
		m.armed = nil
	}
}

// drawArmed draws "open?" over the link waiting for its second click, while
// the same link is still where it was clicked.
func (m *Model) drawArmed(lines []string) []string {
	a := m.armed
	if a == nil || !m.set.Now().Before(a.until) || a.y >= len(lines) {
		return lines
	}
	if s, ok := m.linkUnder(a.x, a.y); !ok || s != a.spot {
		return lines
	}
	// On the background of what it is drawn over, as place leaves none.
	style := m.st.peach.Bold(true)
	switch {
	case a.y == 0:
		style = style.Background(m.st.header.GetBackground())
	case a.row >= 0:
		rs := m.rows(m.tab())
		if bg, ok := m.rowBackground(rs[a.row].pr.Key, a.row == m.tab().sel); ok {
			style = style.Background(bg.GetBackground())
		}
	}
	// Only the word is underlined, not the rest of the link's cells.
	label := ansi.Truncate("open?", a.w, "…")
	lines[a.y] = place(lines[a.y], style.Underline(true).Render(label)+style.Render(spaces(a.w-lipgloss.Width(label))), a.x)
	return lines
}

// linkUnder is the link drawn at x, y of the list screen: the viewer in
// the header, a link of the pane, or a pull request's name or author in
// the list.
func (m *Model) linkUnder(x, y int) (spot, bool) {
	if y == 0 {
		lead := lipgloss.Width(m.st.app.Render("desk") + m.st.dim.Render(" // "))
		if name := m.viewerName(); name != "" && spanAt([]int{lipgloss.Width(name)}, 0, x-lead) == 0 {
			return spot{url: m.webURL(m.viewer), x: lead, y: y, w: lipgloss.Width(name), row: -1}, true
		}
		return spot{}, false
	}
	l := m.layout()
	if top := m.paneTop(l); l.pane > 0 && y >= top {
		r := y - top - 1
		if y == top && !m.claudeOpen {
			if p, _, ok := m.currentPR(); ok {
				for _, k := range m.hintLinks(p, lipgloss.Width(m.paneTabs(p))) {
					if spanAt([]int{k.w}, 0, x-k.x) == 0 {
						return spot{url: k.url, x: k.x, y: y, w: k.w, row: -1}, true
					}
				}
			}
			return spot{}, false
		}
		if y == top || m.claudeOpen || m.pane.mode == paneDiff && r == m.pane.tabRow || r >= len(m.pane.links) {
			return spot{}, false
		}
		for _, k := range m.pane.links[r] {
			// The pane's border and a space come before its lines.
			if spanAt([]int{k.w}, 0, x-2-k.x) == 0 {
				return spot{url: k.url, x: 2 + k.x, y: y, w: k.w, row: -1}, true
			}
		}
		return spot{}, false
	}
	i, ok := m.rowAt(y, l)
	if !ok {
		return spot{}, false
	}
	if s, ok := m.linkAt(i, x); ok {
		s.y, s.row = y, i
		return s, true
	}
	return spot{}, false
}

// linkAt is the link of row i at x: its pull request's name, its author
// or its checks, as the PR, AUTHOR and CI columns draw them.
func (m *Model) linkAt(i, x int) (spot, bool) {
	t := m.tab()
	rs := m.rows(t)
	if i >= len(rs) || rs[i].kind != rowPR {
		return spot{}, false
	}
	p := rs[i].pr
	at := 0
	for _, c := range m.columns(t) {
		switch c.id {
		case "pr":
			// The cursor and the mark come before the name.
			name := fmt.Sprintf("%s#%d", m.repoName(p), p.Key.Number)
			w := min(lipgloss.Width(name), c.width-4)
			if spanAt([]int{w}, 0, x-at-4) == 0 {
				return spot{url: m.prURL(p), x: at + 4, w: w}, true
			}
		case "author":
			a := m.author(p)
			w := min(lipgloss.Width(a.text), c.width)
			if a.url != "" && spanAt([]int{w}, 0, x-at) == 0 {
				return spot{url: a.url, x: at, w: w}, true
			}
		case "ci":
			// The mark stands for the checks once they are read.
			w := min(lipgloss.Width(m.cell("ci", rs[i], false, i)), c.width)
			if p.HeavyRead() && p.Checks != "" && spanAt([]int{w}, 0, x-at) == 0 {
				return spot{url: m.prURL(p) + "/checks", x: at, w: w}, true
			}
		}
		at += c.width + 2
	}
	return spot{}, false
}

// prURL is a pull request's web page.
func (m *Model) prURL(p model.PR) string {
	return cmp.Or(p.URL, m.webURL(fmt.Sprintf("%s/pull/%d", p.Key.Repository(), p.Key.Number)))
}

// webURL is a page on the GitHub host.
func (m *Model) webURL(path string) string {
	return "https://" + cmp.Or(m.set.Host, "github.com") + "/" + path
}

// rowAt is the row of the list on line y, as renderList draws it.
func (m *Model) rowAt(y int, l layout) (int, bool) {
	t := m.tab()
	first := m.top() + 1
	if t.err != nil {
		first++
	}
	if !t.loaded && t.loading || y < first || y >= m.top()+1+m.rowLines(t, l.list) {
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

// popupBox is where the popup shown was last drawn.
type popupBox struct{ x, y, w, h int }

// boxAt is where overlay draws popup from line top of a screen width wide.
func boxAt(popup []string, top, width int) popupBox {
	if len(popup) == 0 {
		return popupBox{}
	}
	w := lipgloss.Width(popup[0])
	return popupBox{x: max((width-w)/2, 0), y: top, w: w, h: len(popup)}
}

func (b popupBox) has(x, y int) bool {
	return x >= b.x && x < b.x+b.w && y >= b.y && y < b.y+b.h
}

// pointer is what the mouse keeps of a popup with a cursor: the item each
// line of its box shows, and the item the last mouse input clicked.
type pointer struct {
	// items are by the line of the box, -1 on one that shows none.
	items []int
	// clicked is the item of the last click, while hit says the last
	// mouse input was that click.
	clicked int
	hit     bool
}

// noItems is n lines of a popup's body that show no item.
func noItems(n int) []int {
	out := make([]int, n)
	for i := range out {
		out[i] = -1
	}
	return out
}

// boxItems are the items of the lines of a box scrollBox drew h lines tall
// with a foot of feet lines, from the owners of its body's lines and the
// first line it drew: the top edge, the body shown, then the foot.
func boxItems(owner []int, top, h, feet int) []int {
	items := []int{-1}
	for j := top; j < len(owner) && len(items) < h-1-feet; j++ {
		items = append(items, owner[j])
	}
	return items
}

// popupMouse gives mouse input to the popup shown, in the order keys go to
// them; ok is false with none. Each takes it as listMouse or scrollMouse
// does, and acts within ADR-012: what writes to GitHub stays on its key.
// The text being written takes none, so a click does not throw it away.
func (m *Model) popupMouse(msg tea.MouseMsg) (cmd tea.Cmd, ok bool) {
	switch {
	case m.bulk != nil:
		if m.bulk.close != nil {
			if cmd, ok := m.closeMouse(msg); ok {
				return cmd, true
			}
		}
		if m.scrollMouse(msg, &m.bulk.offset) {
			return m.bulkKey(keymap.Back), true
		}
	case m.notesAt != nil:
		v := m.notesAt
		n := len(m.noteLines())
		if run, closed := m.listMouse(msg, &v.pointer, &v.sel, n); closed {
			m.notesAt = nil
		} else if run {
			return m.chooseNote(), true
		}
	case m.picker != nil:
		v := m.picker
		if run, closed := m.listMouse(msg, &v.pointer, &v.sel, len(m.rules())); closed {
			m.picker = nil
		} else if run {
			return m.chooseRules(false), true
		}
	case m.menu != nil:
		return m.menuMouse(msg), true
	case m.triage != nil && m.triage.open:
		tr := m.triage
		top := tr.top
		if m.scrollMouse(msg, &tr.top) {
			// A request on its way is stopped by its key, not a click.
			if !tr.asking {
				m.closeTriage()
			}
		} else if tr.top != top {
			tr.follow = false
		}
	case m.help && m.relBox != nil:
		if msg, ok := msg.(tea.MouseWheelMsg); ok {
			switch msg.Button {
			case tea.MouseWheelDown:
				m.scrollReleaseNotes(wheelLines)
			case tea.MouseWheelUp:
				m.scrollReleaseNotes(-wheelLines)
			}
		}
	case m.help:
		switch msg := msg.(type) {
		case tea.MouseClickMsg:
			if msg.Button == tea.MouseLeft {
				m.helpClick(msg.X, msg.Y)
			}
		case tea.MouseWheelMsg:
			switch msg.Button {
			case tea.MouseWheelDown:
				m.scrollHelp(wheelLines)
			case tea.MouseWheelUp:
				m.scrollHelp(-wheelLines)
			}
		}
	case m.compose != nil:
	default:
		return nil, false
	}
	return nil, true
}

// listMouse is mouse input on a popup with a cursor at *sel over n items.
// The wheel moves the cursor, and a click on an item puts the cursor on
// it; one more click on it gives run, so that a stray click runs nothing.
// A click outside the popup gives closed.
func (m *Model) listMouse(msg tea.MouseMsg, p *pointer, sel *int, n int) (run, closed bool) {
	hit := p.hit
	p.hit = false
	switch msg := msg.(type) {
	case tea.MouseWheelMsg:
		switch msg.Button {
		case tea.MouseWheelDown:
			*sel = min(*sel+1, max(n-1, 0))
		case tea.MouseWheelUp:
			*sel = max(*sel-1, 0)
		}
	case tea.MouseClickMsg:
		if msg.Button != tea.MouseLeft {
			return false, false
		}
		if !m.popupAt.has(msg.X, msg.Y) {
			return false, true
		}
		r := msg.Y - m.popupAt.y
		if r >= len(p.items) || p.items[r] < 0 || p.items[r] >= n {
			return false, false
		}
		i := p.items[r]
		run = hit && p.clicked == i && *sel == i
		*sel, p.clicked, p.hit = i, i, !run
	}
	return run, false
}

// scrollMouse is mouse input on a popup that scrolls from line *top: the
// wheel scrolls it, and a click outside it gives closed.
func (m *Model) scrollMouse(msg tea.MouseMsg, top *int) (closed bool) {
	switch msg := msg.(type) {
	case tea.MouseWheelMsg:
		switch msg.Button {
		case tea.MouseWheelDown:
			// Kept within the body when it is drawn.
			*top += wheelLines
		case tea.MouseWheelUp:
			*top = max(*top-wheelLines, 0)
		}
	case tea.MouseClickMsg:
		return msg.Button == tea.MouseLeft && !m.popupAt.has(msg.X, msg.Y)
	}
	return false
}
