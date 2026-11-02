package tui

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/babarot/gh-triage/internal/model"
	"github.com/babarot/gh-triage/internal/runner"
	"github.com/babarot/gh-triage/internal/statusline"
)

// batchWords names what a batch does while it runs.
var batchWords = map[runner.Kind]string{runner.Approve: "approving", runner.Merge: "approving and merging", runner.Ready: "marking ready"}

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
		return m.st.mauve.Render(m.spinner()) + " " + m.st.muted.Render(text)
	}
	return m.st.muted.Render(text)
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
