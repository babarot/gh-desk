package tui

import (
	"fmt"

	tea "charm.land/bubbletea/v2"

	"github.com/babarot/gh-triage/internal/model"
	"github.com/babarot/gh-triage/internal/plan"
)

// Lock is the lock on the record of pending merges, which one gh-triage
// holds at a time (ADR-009). The app package passes it to a gh-triage
// started while another held it.
type Lock interface {
	// Take tries to take the lock again; ok is false while another
	// gh-triage still holds it. Once taken, merges may wait, and pending
	// are the merges the one that quit was waiting for.
	Take() (pending []model.Pending, ok bool, err error)
	// Holder is who holds the lock, as it wrote itself in: its process
	// ID and the directory it was started in. known is false when it
	// wrote nothing.
	Holder() (pid int, dir string, known bool)
}

type lockMsg struct {
	pending []model.Pending
	ok      bool
	err     error
	pid     int
	dir     string
	known   bool
}

// holder is the gh-triage holding the lock, as last read.
type holder struct {
	pid   int
	dir   string
	known bool
}

// takeLock tries the lock again when this gh-triage runs without it: at
// start, when the merge popup opens, and on refresh, so a gh-triage that
// quit hands its merges that wait over without this one starting again.
func (m *Model) takeLock() tea.Cmd {
	if m.set.CanWait || m.set.Lock == nil || m.taking {
		return nil
	}
	// One try at a time: only the try that takes the lock gets the
	// merges to go on with, and it must not be overtaken by another.
	m.taking = true
	l := m.set.Lock
	return func() tea.Msg {
		ps, ok, err := l.Take()
		msg := lockMsg{pending: ps, ok: ok, err: err}
		if !ok {
			msg.pid, msg.dir, msg.known = l.Holder()
		}
		return msg
	}
}

func (m *Model) tookLock(msg lockMsg) tea.Cmd {
	m.taking = false
	if msg.err != nil {
		m.log.Warn("could not take the lock on the record of pending merges", "err", msg.err.Error())
	}
	if !msg.ok {
		m.holder = holder{pid: msg.pid, dir: msg.dir, known: msg.known}
		return nil
	}
	if m.set.CanWait {
		return nil
	}
	m.set.CanWait = true
	m.log.Info("took the record of pending merges over", "count", len(msg.pending))
	if m.bulk != nil {
		m.bulk.tookOver = true
		if !m.bulk.loading {
			m.decideBulk()
		}
	}
	text := "the other gh-triage quit · this one runs the merges that wait now"
	var cmd tea.Cmd
	if len(msg.pending) > 0 {
		m.set.Resume = msg.pending
		cmd = m.resume()
		text += fmt.Sprintf(" · going on with %d", len(msg.pending))
	}
	m.setStatus(text, false)
	return cmd
}

// lockedBy names the gh-triage holding the lock, for a row.
func (m *Model) lockedBy() string {
	if m.holder.known {
		return fmt.Sprintf("locked by pid %d", m.holder.pid)
	}
	return "locked by another gh-triage"
}

// lockedHolder names it in full, for a sentence.
func (m *Model) lockedHolder() string {
	if m.holder.known {
		return fmt.Sprintf("another gh-triage (pid %d · %s)", m.holder.pid, m.holder.dir)
	}
	return "another gh-triage"
}

// lockedDetail is the full reason of a merge skipped for the lock, shown
// on the status line while the cursor is on its row.
func (m *Model) lockedDetail(k model.PRKey, step plan.Step) string {
	return fmt.Sprintf("%s skipped: it %s, and the merges that wait are locked by %s · quit it and press %s again",
		k, waitWord(step), m.lockedHolder(), m.firstKey("merge"))
}

// waitWord says how a merge would wait.
func waitWord(s plan.Step) string {
	switch s {
	case plan.StepWait:
		return "would wait for checks"
	case plan.StepUpdate:
		return "would update the branch and wait"
	case plan.StepAutoMerge:
		return "would turn on auto-merge and watch it"
	}
	return "would wait"
}

// rowStatus shows the full result of the row under the cursor on the
// status line, where the row has room for a few words only. It reports
// whether the row has one.
func (m *Model) rowStatus() bool {
	p, _, ok := m.currentPR()
	if !ok {
		return false
	}
	pg, ok := m.progress[p.Key]
	if !ok {
		return false
	}
	text := pg.detail
	if text == "" {
		text = p.Key.String() + ": " + pg.text
	}
	m.setStatus(text, false)
	m.statusOfRow = true
	return true
}

// followCursor changes the status line when the cursor moved to another
// pull request: its result if it has one, or nothing if the line showed
// the result of the row it left.
func (m *Model) followCursor(before model.PRKey, had bool) {
	p, _, ok := m.currentPR()
	if ok == had && (!ok || p.Key == before) {
		return
	}
	if m.rowStatus() {
		return
	}
	if m.statusOfRow {
		m.setStatus("", false)
	}
}
