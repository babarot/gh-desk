package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/babarot/gh-desk/internal/model"
	"github.com/babarot/gh-desk/internal/runner"
)

// stopKinds sorts the pull requests a stop is asked for: those a job of
// this gh-desk waits on, which it stops; those handed to auto-merge or a
// merge queue, which are GitHub's and left alone (ADR-122); and the rest,
// at work nowhere here.
func (m *Model) stopKinds(prs []model.PR) (atWork, handed, idle []model.PRKey) {
	for _, p := range prs {
		pg, ok := m.progress[p.Key]
		switch {
		case ok && pg.result == runner.Running:
			atWork = append(atWork, p.Key)
		case ok && (pg.result == runner.AutoMergeOn || pg.result == runner.Queued):
			handed = append(handed, p.Key)
		default:
			idle = append(idle, p.Key)
		}
	}
	return atWork, handed, idle
}

// aboutStop says what stop would do to prs, or why it cannot.
func (m *Model) aboutStop(prs []model.PR) (string, bool) {
	atWork, handed, _ := m.stopKinds(prs)
	switch {
	case len(atWork) > 0 && len(prs) == 1:
		return "stop waiting; nothing more is written", false
	case len(atWork) > 0:
		return fmt.Sprintf("stop %d of %d at work; nothing more is written", len(atWork), len(prs)), false
	case len(handed) > 0:
		return "auto-merge is GitHub's now; turn it off there", true
	case len(prs) == 1:
		return "not at work", true
	}
	return "none of them is at work", true
}

// stopWork stops the jobs on the selection, or on the pull request under
// the cursor, at their next wait. Nothing is written to GitHub, so there
// is no confirm popup; the selection is spent as a confirmed action's is
// (ADR-094).
func (m *Model) stopWork() tea.Cmd {
	prs := m.menuTargets()
	if len(prs) == 0 {
		return nil
	}
	if why, off := m.aboutStop(prs); off {
		m.refuse("stop: " + why)
		return nil
	}
	atWork, handed, idle := m.stopKinds(prs)
	keys := make([]model.PRKey, 0, len(prs))
	for _, p := range prs {
		keys = append(keys, p.Key)
	}
	m.spend(keys)
	asked := m.run.Stop(atWork)
	// One that ended, or was handed to GitHub, since its row was drawn
	// is not asked: it is told by its own result.
	parts := []string{fmt.Sprintf("asked to stop %d", len(asked))}
	if n := len(handed); n > 0 {
		parts = append(parts, fmt.Sprintf("auto-merge stays on for %d", n))
	}
	if n := len(idle) + len(atWork) - len(asked); n > 0 {
		parts = append(parts, fmt.Sprintf("%d not at work", n))
	}
	items := make([]noteItem, 0, len(asked)+len(handed))
	for _, k := range asked {
		items = append(items, noteItem{key: k, text: "asked to stop", result: runner.Running})
	}
	for _, k := range handed {
		items = append(items, noteItem{key: k, text: "auto-merge stays on GitHub", result: m.progress[k].result})
	}
	return m.pushNote("!", "stop: "+strings.Join(parts, " · "), items)
}

// stoppable reports whether a job of this gh-desk waits on a pull request,
// so that the status line tells the key that stops it.
func (m *Model) stoppable() bool {
	for _, pg := range m.progress {
		if pg.result == runner.Running {
			return true
		}
	}
	return false
}

// stopHint is the key that stops, told after what is at work.
func (m *Model) stopHint() string {
	ks := m.keys.Keys("stop")
	if len(ks) == 0 {
		return ""
	}
	k := ks[0]
	return m.st.dim.Render(" · ") + m.st.key.Render(k) + " " + m.st.muted.Render(m.keys.Help("stop"))
}
