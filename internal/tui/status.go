package tui

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/babarot/gh-triage/internal/model"
	"github.com/babarot/gh-triage/internal/runner"
)

// batchWords names what a batch does while it runs.
var batchWords = map[runner.Kind]string{runner.Approve: "approving", runner.Merge: "approving and merging", runner.Ready: "marking ready"}

// The status line (ADR-025) shows, in order: the newest event still in
// its time, information giving way to a failure still in its time, the result of the row the
// cursor has just moved onto, the work in progress, and the result of the
// row under the cursor. Events are said with say, refuse and fail, and
// only for what already happened; work in progress is listed in
// activities and built from the state, never written into the line.

type eventKind int

const (
	eventInfo eventKind = iota
	eventRefusal
	eventFailure
)

// How long each kind of event stays on the line. Information lasts as long
// as a toast, which says the same kind of thing; a failure longer, as it
// may need reading twice.
const (
	infoFor    = toastFor
	refusalFor = 4 * time.Second
	failureFor = 10 * time.Second
	// rowFor is how long the result of the row the cursor moved onto goes
	// before the work in progress.
	rowFor = toastFor
)

type statusEvent struct {
	kind eventKind
	text string
	at   time.Time
}

func (e statusEvent) life() time.Duration {
	switch e.kind {
	case eventRefusal:
		return refusalFor
	case eventFailure:
		return failureFor
	}
	return infoFor
}

func (e statusEvent) live(now time.Time) bool {
	return e.text != "" && now.Sub(e.at) < e.life()
}

// statusLine is what the status line holds: the newest event, the newest
// failure, and when the cursor moved onto the row it is on.
type statusLine struct {
	last   statusEvent
	failed statusEvent
	rowAt  time.Time
	// tickAt is when the redraw asked for is due, zero when none is.
	tickAt time.Time
}

// say tells of something that happened, such as a write that went through.
func (m *Model) say(text string) { m.tell(eventInfo, text) }

// refuse tells why the key pressed cannot act here. It goes at the next
// key.
func (m *Model) refuse(text string) { m.tell(eventRefusal, text) }

// fail tells of something that went wrong. A failure that may arrive
// while the user is not looking goes into the notifications as well; see
// failNoted.
func (m *Model) fail(text string) { m.tell(eventFailure, text) }

func (m *Model) tell(kind eventKind, text string) {
	e := statusEvent{kind: kind, text: text, at: m.set.Now()}
	m.line.last = e
	if kind == eventFailure {
		m.line.failed = e
	}
}

// failNoted says a failure and keeps it in the notifications, with the
// pull request it is about when there is one.
func (m *Model) failNoted(text string, k model.PRKey) tea.Cmd {
	m.fail(text)
	return m.pushNote("✗", text, []noteItem{{key: k, text: text}})
}

// keyPressed puts away a refusal, which answers the key before.
func (m *Model) keyPressed() {
	if m.line.last.kind == eventRefusal {
		m.line.last = statusEvent{}
	}
}

// movedOnto marks the row the cursor has just moved onto, so its result
// goes before the work in progress for a moment.
func (m *Model) movedOnto() {
	m.line.rowAt = time.Time{}
	if m.rowResult() != "" {
		m.line.rowAt = m.set.Now()
	}
}

// shownEvent is the event the line shows now, if any: the newest, except
// that information does not hide a failure still in its time. A refusal
// does, as it answers the key just pressed and goes at the next.
func (m *Model) shownEvent() (statusEvent, bool) {
	now := m.set.Now()
	if m.line.last.live(now) && m.line.last.kind != eventInfo {
		return m.line.last, true
	}
	if m.line.failed.live(now) {
		return m.line.failed, true
	}
	if m.line.last.live(now) {
		return m.line.last, true
	}
	return statusEvent{}, false
}

// statusText is the status line as drawn.
func (m *Model) statusText() string {
	if e, ok := m.shownEvent(); ok {
		switch e.kind {
		case eventFailure:
			return m.st.red.Render(e.text)
		case eventRefusal:
			return m.st.peach.Render(e.text)
		}
		return m.st.muted.Render(e.text)
	}
	row := m.rowResult()
	fresh := row != "" && m.set.Now().Sub(m.line.rowAt) < rowFor
	if fresh {
		return m.st.muted.Render(row)
	}
	if acts := m.activities(); len(acts) > 0 {
		return m.st.mauve.Render(m.spinner()) + " " + m.st.muted.Render(strings.Join(acts, " · "))
	}
	return m.st.muted.Render(row)
}

// activities is the work in progress, writes to GitHub first.
func (m *Model) activities() []string {
	var out []string
	for _, f := range []func() string{m.postingActivity, m.reviewActivity, m.batchActivity, m.claudeActivity, m.refreshActivity} {
		if s := f(); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// rowResult is the full result of the row under the cursor, where the row
// has room for a few words only.
func (m *Model) rowResult() string {
	p, _, ok := m.currentPR()
	if !ok {
		return ""
	}
	pg, ok := m.progress[p.Key]
	if !ok {
		return ""
	}
	if pg.detail != "" {
		return pg.detail
	}
	return p.Key.String() + ": " + pg.text
}

// statusTick asks for a redraw when what the line shows next is due: an
// event or the moment of a row's result ends.
func (m *Model) statusTick() tea.Cmd {
	now := m.set.Now()
	var due time.Time
	for _, t := range []time.Time{
		m.line.last.at.Add(m.line.last.life()), m.line.failed.at.Add(m.line.failed.life()), m.line.rowAt.Add(rowFor),
	} {
		if t.After(now) && (due.IsZero() || t.Before(due)) {
			due = t
		}
	}
	if due.IsZero() || (!m.line.tickAt.IsZero() && !m.line.tickAt.After(due) && m.line.tickAt.After(now)) {
		return nil
	}
	m.line.tickAt = due
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
		verb := batchWords[bs.kind]
		if bs.resumed {
			verb = "going on with merges"
		}
		parts = append(parts, fmt.Sprintf("%s… %d/%d", verb, len(bs.done), len(bs.goes)))
	}
	return strings.Join(parts, " · ")
}

// reviewActivity is a single review on its way.
func (m *Model) reviewActivity() string {
	if m.sending == "" || m.posting != nil || m.sendingWhat == "" {
		return ""
	}
	return fmt.Sprintf("sending %s to %s…", m.sendingWhat, m.sending)
}
