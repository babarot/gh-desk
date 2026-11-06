package tui

import (
	"fmt"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/babarot/gh-desk/internal/model"
	"github.com/babarot/gh-desk/internal/runner"
	"github.com/babarot/gh-desk/internal/tui/statusline"
)

// batchWords names what a batch does while it runs. A merge is named by
// its goal: whether it marks a pull request ready or approves it first
// differs between pull requests and is shown on their lanes.
var batchWords = map[runner.Kind]string{runner.Merge: "merging", runner.Ready: "marking ready", runner.Close: "closing", runner.Comment: "commenting"}

// reviewingWords say what a batch of reviews of each kind is doing.
var reviewingWords = map[model.ReviewEvent]string{model.EventApprove: "approving", model.EventComment: "sending review comments",
	model.EventRequestChanges: "requesting changes"}

// The status line (ADR-025) is decided by internal/statusline, whose state
// only its methods change. Events are said with say, refuse and fail, and
// only for what already happened; work in progress is listed in
// activities and built from the state, never written into the line.

// say tells of something that happened, such as a write that went through.
func (m *Model) say(text string) { m.line.Say(m.set.Now(), text) }

// refuse tells why the key pressed cannot act here. It goes at the next
// key.
func (m *Model) refuse(text string) { m.line.Refuse(m.set.Now(), text) }

// fail tells of something that went wrong. A failure that may arrive
// while the user is not looking goes into the notifications as well; see
// failNoted.
func (m *Model) fail(text string) { m.line.Fail(m.set.Now(), text) }

// failNoted says a failure and keeps it in the notifications, with the
// pull request it is about when there is one.
func (m *Model) failNoted(text string, k model.PRKey) tea.Cmd {
	m.fail(text)
	return m.pushNote("✗", text, []noteItem{{key: k, text: text}})
}

// movedOnto marks the row the cursor has just moved onto, so its result
// goes before the work in progress for a moment.
func (m *Model) movedOnto() { m.line.MovedOnto(m.set.Now(), m.rowResult() != "") }

// shownStatus is what the status line shows now.
func (m *Model) shownStatus() (statusline.Kind, string) {
	return m.line.Shown(m.set.Now(), m.activities(), m.rowResult())
}

// statusText is the status line as drawn.
func (m *Model) statusText() string {
	kind, text := m.shownStatus()
	switch kind {
	case statusline.Failure:
		return m.st.red.Render(text)
	case statusline.Refusal:
		return m.st.peach.Render(text)
	case statusline.Activity:
		return m.activityText()
	}
	return m.st.muted.Render(text)
}

// activityText is the work in progress as drawn: led by the spinner, but
// for the reading of the shown tab, which is drawn last with a bar of how
// far it is when it has one.
func (m *Model) activityText() string {
	text, full, cells := m.tabActivity()
	if cells == 0 {
		return m.working(m.st.muted, strings.Join(m.activities(), " · "))
	}
	out := readBar(m.st, full, cells) + " " + m.st.muted.Render(text)
	if work := m.workActivities(); len(work) > 0 {
		out = m.working(m.st.muted, strings.Join(work, " · ")) + m.st.dim.Render(" · ") + out
	}
	return out
}

// activities is the work in progress, writes to GitHub first and the
// reading of the shown tab last.
func (m *Model) activities() []string {
	out := m.workActivities()
	if text, _, _ := m.tabActivity(); text != "" {
		out = append(out, text)
	}
	return out
}

// workActivities is the work in progress but the reading of the shown
// tab.
func (m *Model) workActivities() []string {
	var out []string
	for _, f := range []func() string{m.batchActivity, m.claudeActivity, m.refreshActivity} {
		if s := f(); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// tabActivity is the reading of the shown tab, while it is the first one
// or r's, and empty otherwise: the clock's reading says nothing, nor one
// of a tab shown from the cache. A tab
// that lists more pull requests than one read of heavy fields takes has a
// bar of cells, full of them filled: a pull request counts once when it is
// listed and once when its heavy fields are back, so that the bar moves
// while the pages of the search come too. A tab whose count is not known
// yet, a small one and the inbox have none (cells is zero), and are said
// with the spinner.
func (m *Model) tabActivity() (text string, full, cells int) {
	t := m.tab()
	r := m.refreshing
	byR := r != nil && slices.Contains(r.tabs, m.cur)
	if m.tabDone(t) || !t.told && !byR {
		return "", 0, 0
	}
	text = "reading " + t.name
	if byR {
		text = "refreshing"
	}
	if t.inbox || t.want <= heavyBatch {
		text += "…"
	} else {
		listed := len(t.prs)
		if t.pend != nil {
			listed = len(t.pend.prs)
		} else if !t.loaded {
			listed = 0
		}
		done := min(listed, t.want) + min(t.heavyGot, t.want)
		cells = min(t.want, readBarMax)
		full = done * cells / (2 * t.want)
	}
	if byR && r.cleared > 0 {
		text += fmt.Sprintf(" · %d results cleared", r.cleared)
	}
	return text, full, cells
}

// telling is r's reading of the shown tab, told on the status line: of
// which tab.
type telling struct{ tab int }

// endTabReading says, for a moment, that r's reading of the shown tab is
// over. The first reading ends without a word, as its rows say it; a
// reading that failed has said so, and one of a tab no longer shown says
// nothing.
func (m *Model) endTabReading() {
	if text, _, _ := m.tabActivity(); text != "" {
		m.telling = nil
		if m.refreshing != nil {
			m.telling = &telling{tab: m.cur}
		}
		return
	}
	tl := m.telling
	m.telling = nil
	t := m.tab()
	if tl == nil || tl.tab != m.cur || !m.tabDone(t) || t.err != nil || !t.asOf.IsZero() {
		return
	}
	m.say("refreshed")
}

// readBarMax is the most cells a bar of how far a reading is takes.
const readBarMax = 12

// readBar is a bar of cells, full of them filled. ▰ and ▱ are one cell
// wide on any terminal, as a spinner's frames are (ADR-030).
func readBar(st styles, full, cells int) string {
	return st.mauve.Render(strings.Repeat("▰", full)) + st.border.Render(strings.Repeat("▱", cells-full))
}

// rowResult is the full result of the row under the cursor, where the row
// has room for a few words only, or what excludes the row when it has
// none.
func (m *Model) rowResult() string {
	r, ok := m.current()
	if !ok || r.kind != rowPR {
		return ""
	}
	p := r.pr
	pg, ok := m.progress[p.Key]
	if !ok {
		// A row shown with a, which the exclusion leaves out, says by
		// what.
		if r.ex.excluded() {
			return "excluded by " + strings.Join(r.ex.by, ", ")
		}
		return ""
	}
	if pg.detail != "" {
		return pg.detail
	}
	return p.Key.String() + ": " + pg.text
}

// statusTick asks for a redraw when what the line shows may change by
// time alone.
func (m *Model) statusTick() tea.Cmd {
	now := m.set.Now()
	due := m.line.Due(now)
	if due.IsZero() || (m.statusTickAt.After(now) && !m.statusTickAt.After(due)) {
		return nil
	}
	m.statusTickAt = due
	return m.after(due.Sub(now), statusTickMsg{})
}

type statusTickMsg struct{}

// after is tea.Tick, or what the settings put in its place.
func (m *Model) after(d time.Duration, msg tea.Msg) tea.Cmd {
	if m.set.Tick != nil {
		return m.set.Tick(d, msg)
	}
	return tea.Tick(d, func(time.Time) tea.Msg { return msg })
}

// batchActivity counts each batch still running.
func (m *Model) batchActivity() string {
	var parts []string
	for _, id := range m.batchOrder {
		bs := m.batches[id]
		if bs == nil || bs.noted {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s… %d/%d", m.batchVerb(bs), len(bs.done), len(bs.goes))+batchCounts(bs))
	}
	return strings.Join(parts, " · ")
}

// batchCounts is how a batch's pull requests stand, those that are none
// left out: done, waiting, skipped, stopped.
func batchCounts(bs *batchState) string {
	var done, waiting, skipped, stopped int
	for _, p := range bs.done {
		switch p.result {
		case runner.Skipped:
			skipped++
		case runner.Stopped, runner.Failed:
			stopped++
		default:
			done++
		}
	}
	for k, l := range bs.lanes {
		if _, ended := bs.done[k]; !ended && l.cur != nil && l.cur.step.Waits() {
			waiting++
		}
	}
	var out strings.Builder
	for _, c := range []struct {
		n    int
		word string
	}{{done, "done"}, {waiting, "waiting"}, {skipped, "skipped"}, {stopped, "stopped"}} {
		if c.n > 0 {
			fmt.Fprintf(&out, " · %d %s", c.n, c.word)
		}
	}
	return out.String()
}
