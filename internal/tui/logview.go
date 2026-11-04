package tui

import (
	"fmt"
	"log/slog"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/babarot/gh-desk/internal/logging"
	"github.com/babarot/gh-desk/internal/model"
	"github.com/babarot/gh-desk/internal/tui/keymap"
)

// The log viewer shows the log's records as they are written (ADR-058).
// While it is open it takes the keys, as the claude pane does (ADR-060).

// The log viewer's width is two thirds of the screen, kept between these;
// it is drawn over the list's right side while the list keeps at least
// logMinList columns, and in the bottom pane's place otherwise.
const (
	logMinWidth = 60
	logMaxWidth = 100
	logMinList  = 40
)

// logRedraw is how often an open log viewer draws again, to show what was
// written since.
const logRedraw = 250 * time.Millisecond

// logHidden are the attributes not shown: the same on every line of a run.
var logHidden = map[string]bool{logging.KeyRun: true, logging.KeyVersion: true}

// logView is the log viewer while it is open.
type logView struct {
	// pr is the pull request under the cursor when it opened, which f
	// narrows the log to; the cursor does not move while it is open.
	pr   model.PRKey
	only bool
	// follow shows the newest records as they come; otherwise the view
	// stays with end records before its last line.
	follow bool
	end    int
	// gen tells this opening's redraws from an earlier one's.
	gen int
}

type logTickMsg struct{ gen int }

// logWidth is the log viewer's width, and whether it is drawn over the
// list rather than in the bottom pane's place.
func (m *Model) logWidth() (int, bool) {
	w := min(max(m.width*2/3, logMinWidth), logMaxWidth, m.width)
	return w, m.width-w >= logMinList
}

// logInPane reports whether the open log viewer takes the bottom pane's
// place.
func (m *Model) logInPane() bool {
	if m.logv == nil {
		return false
	}
	_, over := m.logWidth()
	return !over
}

// openLog opens the log viewer, or says why it cannot.
func (m *Model) openLog() tea.Cmd {
	if m.set.LogRecords == nil {
		m.refuse("there is no log to show in this run")
		return nil
	}
	if _, over := m.logWidth(); !over && m.layoutFor(true).pane == 0 {
		m.refuse("no room for the log here: the screen is too short")
		return nil
	}
	var pr model.PRKey
	if p, _, ok := m.currentPR(); ok {
		pr = p.Key
	}
	m.logGen++
	m.logv = &logView{pr: pr, follow: true, gen: m.logGen}
	return m.after(logRedraw, logTickMsg{m.logGen})
}

func (m *Model) closeLog() { m.logv = nil }

// logTick draws again while the log viewer this tick is of stays open.
func (m *Model) logTick(msg logTickMsg) tea.Cmd {
	if m.logv == nil || m.logv.gen != msg.gen {
		return nil
	}
	return m.after(logRedraw, msg)
}

// logKey handles a key while the log viewer is open. It takes every key;
// what the list does is not done meanwhile.
func (m *Model) logKey(k string) tea.Cmd {
	v := m.logv
	is := func(a keymap.Action) bool { return m.keys.Matches(a, k) }
	page := max(m.logInner()/2, 1)
	switch {
	case k == keymap.Back, is("log"):
		m.closeLog()
	case is("log_this_pr"):
		if v.pr == (model.PRKey{}) {
			m.refuse("no pull request was under the cursor")
			return nil
		}
		v.only, v.follow = !v.only, true
	case is("up"):
		m.scrollLog(-1)
	case is("down"):
		m.scrollLog(1)
	case is("page_up"):
		m.scrollLog(-page)
	case is("page_down"):
		m.scrollLog(page)
	case is("top"):
		v.follow, v.end = false, 1
	case is("bottom"):
		v.follow = true
	}
	return nil
}

// scrollLog moves the log by n records, back when n is negative. Moving
// back stops following; reaching the newest follows again.
func (m *Model) scrollLog(n int) {
	v := m.logv
	all := len(m.logRecords())
	if v.follow {
		if n >= 0 {
			return
		}
		v.follow, v.end = false, all
	}
	v.end = min(max(v.end+n, 1), all)
	if v.end >= all {
		v.follow = true
	}
}

// logRecords are the records the log viewer shows.
func (m *Model) logRecords() []logging.Record {
	all := m.set.LogRecords.All()
	v := m.logv
	if !v.only {
		return all
	}
	key := v.pr.String()
	var out []logging.Record
	for _, r := range all {
		if p, ok := r.Attr(logging.KeyPR); ok && p == key {
			out = append(out, r)
		}
	}
	return out
}

// logInner is how many lines of records the log viewer holds.
func (m *Model) logInner() int {
	if m.logInPane() {
		return max(m.layout().pane-2, 1)
	}
	// Over the list: from under the header to above the status line.
	return max(m.height-3-2, 1)
}

// renderLog is the log viewer's box, w wide and h lines high.
func (m *Model) renderLog(w, h int) []string {
	st, v := m.st, m.logv
	inner := w - 4
	right := "following"
	if !v.follow {
		right = "paused · " + m.firstKey("bottom") + " to follow"
	}
	if v.only {
		right += " · " + v.pr.String() + " only"
	}
	title := st.mauve.Bold(true).Render("log")
	top := st.border.Render("╭ ") + title + " "
	tail := " " + st.dim.Render(right) + st.border.Render(" ╮")
	if lipgloss.Width(top)+lipgloss.Width(tail) > w {
		tail = st.border.Render(" ╮")
	}
	fill := w - lipgloss.Width(top) - lipgloss.Width(tail)
	out := []string{top + st.border.Render(strings.Repeat("─", max(fill, 0))) + tail}
	body := m.logBody(inner, h-2)
	for i := 0; i < h-2; i++ {
		s := ""
		if i < len(body) {
			s = body[i]
		}
		out = append(out, st.border.Render("│ ")+fit(s, inner)+st.border.Render(" │"))
	}
	return append(out, st.border.Render("╰"+strings.Repeat("─", max(w-2, 0))+"╯"))
}

// logBody is the lines of the records shown, width wide, at most n: the
// newest while following, those up to where it was stopped otherwise,
// filled from after them when they do not fill it.
func (m *Model) logBody(width, n int) []string {
	recs := m.logRecords()
	end := len(recs)
	if !m.logv.follow {
		end = min(m.logv.end, len(recs))
	}
	var lines []string
	start := end
	for start > 0 {
		r := m.logLines(recs[start-1], width)
		if len(lines)+len(r) > n && len(lines) > 0 {
			break
		}
		lines = append(r, lines...)
		start--
	}
	for i := end; i < len(recs) && len(lines) < n; i++ {
		lines = append(lines, m.logLines(recs[i], width)...)
	}
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return lines
}

// logWord is a piece of a record's line in one style.
type logWord struct {
	style lipgloss.Style
	text  string
}

// logLines is a record as the log viewer draws it: the time, the level,
// the message and the attributes, wrapped at a space under the message.
func (m *Model) logLines(r logging.Record, width int) []string {
	st := m.st
	lead := st.dim.Render(r.Time.Local().Format("15:04:05")) + " " + m.levelStyle(r.Level).Render(fmt.Sprintf("%-5s", r.Level.String())) + " "
	leadW := lipgloss.Width(lead)
	msg := st.text
	if r.Level >= slog.LevelWarn {
		msg = m.levelStyle(r.Level)
	}
	words := []logWord{{msg, r.Msg}}
	for _, a := range r.Attrs {
		if logHidden[a.Key] {
			continue
		}
		val := a.Value.String()
		if strings.ContainsAny(val, " \t\n\"") || val == "" {
			val = fmt.Sprintf("%q", val)
		}
		vs := st.muted
		switch a.Key {
		case logging.KeyPR:
			vs = st.text
			if m.logv.pr != (model.PRKey{}) && val == m.logv.pr.String() {
				vs = st.blue
			}
		case logging.KeyStep:
			vs = st.mauve
		case "err":
			vs = st.red
		}
		words = append(words, logWord{st.dim, " " + a.Key + "="}, logWord{vs, val})
	}
	return wrapWords(lead, leadW, words, width)
}

func (m *Model) levelStyle(l slog.Level) lipgloss.Style {
	switch {
	case l >= slog.LevelError:
		return m.st.red
	case l >= slog.LevelWarn:
		return m.st.yellow
	case l >= slog.LevelInfo:
		return m.st.blue
	}
	return m.st.dim
}

// wrapWords lays words after lead within width. A line breaks before a
// word that starts with a space and does not fit; the lines after the
// first start under the message, two columns in. A word longer than a line
// is cut.
func wrapWords(lead string, leadW int, words []logWord, width int) []string {
	indent := spaces(leadW + 2)
	var lines []string
	line, used, room := lead, 0, width-leadW
	for _, w := range words {
		text := w.text
		for text != "" {
			tw := ansi.StringWidth(text)
			if used+tw <= room {
				line += w.style.Render(text)
				used += tw
				break
			}
			if used > 0 && strings.HasPrefix(text, " ") && tw-1 <= width-leadW-2 {
				lines = append(lines, line)
				line, used, room = indent, 0, width-leadW-2
				text = text[1:]
				continue
			}
			if room-used <= 0 && used == 0 {
				// Too narrow for anything; drop the rest.
				break
			}
			// Cut what does not fit on this line, at a space in it when
			// there is one, such as in a quoted error.
			cut := ansi.Truncate(text, room-used, "")
			if i := strings.LastIndex(cut, " "); i > 0 {
				cut = cut[:i]
			}
			if cut == "" {
				lines = append(lines, line)
				line, used, room = indent, 0, width-leadW-2
				continue
			}
			line += w.style.Render(cut)
			text = strings.TrimPrefix(strings.TrimPrefix(text, cut), " ")
			lines = append(lines, line)
			line, used, room = indent, 0, width-leadW-2
		}
	}
	return append(lines, line)
}

// logAt reports whether x, y is on the log viewer.
func (m *Model) logAt(x, y int) bool {
	if m.logv == nil {
		return false
	}
	if m.logInPane() {
		return y >= m.paneTop(m.layout()) && y < m.height-2
	}
	w, _ := m.logWidth()
	return x >= m.width-w && y >= 1 && y < m.height-2
}

// logMouse takes a mouse message while the log viewer is open: the wheel
// on it scrolls it and a click on it does nothing; pointing anywhere else
// closes it, and reports false so the message goes on to what is there
// (ADR-060).
func (m *Model) logMouse(msg tea.MouseMsg) bool {
	if m.logv == nil {
		return false
	}
	mo := msg.Mouse()
	if !m.logAt(mo.X, mo.Y) {
		m.closeLog()
		return false
	}
	if w, ok := msg.(tea.MouseWheelMsg); ok {
		switch w.Button {
		case tea.MouseWheelUp:
			m.scrollLog(-wheelLines)
		case tea.MouseWheelDown:
			m.scrollLog(wheelLines)
		}
	}
	return true
}
