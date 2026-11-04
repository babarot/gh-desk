package tui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/babarot/gh-desk/internal/model"
	"github.com/babarot/gh-desk/internal/runner"
)

// The progress pane is the top of the log viewer while a batch it shows
// runs: one line for each pull request, with the steps it has been
// through, the one it is on, and those the runner expects ahead (ADR-061).

// stepWords name the steps on a lane.
var stepWords = map[runner.Step]string{
	runner.StepReady: "ready", runner.StepApprove: "approve", runner.StepReview: "review", runner.StepUpdate: "update",
	runner.StepWait: "wait", runner.StepWaitChecks: "checks", runner.StepWaitApproval: "approval checks",
	runner.StepWaitBlocked: "blocked", runner.StepMerge: "merge", runner.StepAutoMerge: "auto-merge",
}

// progressShown are the batches the open log viewer shows: those running
// when it opened and those started since, finished or not.
func (m *Model) progressShown() []*batchState {
	if m.logv == nil {
		return nil
	}
	var out []*batchState
	for _, id := range m.logv.batches {
		if bs := m.batches[id]; bs != nil {
			out = append(out, bs)
		}
	}
	return out
}

// renderProgress is the progress pane, w wide, at most max lines: a box
// for each batch shown. In wide, a lane lays its steps side by side when
// every lane of its batch fits; otherwise each is a row of dots.
func (m *Model) renderProgress(w, limit int, wide bool) []string {
	var out []string
	for _, bs := range m.progressShown() {
		room := limit - len(out) - 2
		if room < 1 {
			break
		}
		out = append(out, m.progressBox(bs, w, room, wide)...)
	}
	return out
}

func (m *Model) progressBox(bs *batchState, w, room int, wide bool) []string {
	st := m.st
	inner := w - 4
	var lines []string
	if wide {
		for _, k := range bs.order {
			lines = append(lines, m.wideLane(bs, k))
		}
		for _, l := range lines {
			if lipgloss.Width(l) > inner {
				lines = nil
				break
			}
		}
	}
	if lines == nil {
		dots, name := 0, 0
		for _, k := range bs.order {
			dots = max(dots, len(stepsOf(bs.lanes[k])))
			name = max(name, len(shortKey(k)))
		}
		for _, k := range bs.order {
			lines = append(lines, m.dotLane(bs, k, min(name, 20), dots))
		}
	}
	if len(lines) > room {
		more := len(lines) - room + 1
		lines = append(lines[:room-1], st.dim.Render(fmt.Sprintf("  … %d more", more)))
	}
	now := m.set.Now()
	title := st.mauve.Bold(true).Render("progress " + m.spinner())
	right := fmt.Sprintf("%s · %d/%d · %s", m.batchVerb(bs), len(bs.done), len(bs.goes), clock(now.Sub(bs.started)))
	if bs.noted {
		title = st.green.Bold(true).Render("progress ✓")
		right = fmt.Sprintf("%s · %d/%d · done in %s", m.batchVerb(bs), len(bs.done), len(bs.goes), clock(bs.ended.Sub(bs.started)))
	}
	return m.viewerBox(title, right, lines, w, len(lines))
}

// drawnStep is a step on a lane as drawn: done, the one it is on, or
// ahead.
type drawnStep struct {
	step  runner.Step
	state int // 0 done, 1 on it, 2 ahead
	took  time.Duration
	at    time.Time
}

// stepsOf are a lane's steps in order: those done, the one it is on, then
// those ahead.
func stepsOf(l *lane) []drawnStep {
	if l == nil {
		return nil
	}
	var out []drawnStep
	for _, s := range l.done {
		out = append(out, drawnStep{step: s.step, took: s.took})
	}
	if l.cur != nil {
		out = append(out, drawnStep{step: l.cur.step, state: 1, at: l.cur.at})
	}
	for _, s := range l.next {
		out = append(out, drawnStep{step: s, state: 2})
	}
	return out
}

// shortKey names a pull request on a lane: its repository and number.
func shortKey(k model.PRKey) string { return fmt.Sprintf("%s#%d", k.Repo, k.Number) }

// laneHead is the start of a lane: a mark on the pull request the cursor
// was on when the viewer opened, and its name.
func (m *Model) laneHead(k model.PRKey, name int) string {
	mark := "  "
	if m.logv != nil && m.logv.pr == k {
		mark = m.st.blue.Render("▸ ")
	}
	return mark + m.st.text.Render(fit(shortKey(k), name))
}

// dotLane is a lane in one row of dots, then what it does now or how it
// ended; what a wait is for is cut to the width rather than wrapped, so
// the pane keeps its height.
func (m *Model) dotLane(bs *batchState, k model.PRKey, name, dots int) string {
	st := m.st
	l := bs.lanes[k]
	head := m.laneHead(k, name) + " "
	steps := stepsOf(l)
	if end, ok := bs.done[k]; ok && len(steps) == 0 {
		return head + m.laneEnd(end)
	}
	var b strings.Builder
	for _, s := range steps {
		switch s.state {
		case 0:
			b.WriteString(st.green.Render("●"))
		case 1:
			b.WriteString(st.mauve.Render("◐"))
		default:
			b.WriteString(st.dim.Render("○"))
		}
	}
	line := head + b.String() + spaces(dots-len(steps)+2)
	if end, ok := bs.done[k]; ok {
		return line + m.laneEnd(end)
	}
	if l != nil && l.cur != nil {
		return line + m.laneNow(l)
	}
	return line + st.dim.Render(m.progress[k].text)
}

// wideLane is a lane with its steps side by side, each with how long it
// took or has taken so far.
func (m *Model) wideLane(bs *batchState, k model.PRKey) string {
	st := m.st
	l := bs.lanes[k]
	head := m.laneHead(k, 10) + " "
	end, ended := bs.done[k]
	steps := stepsOf(l)
	if ended && len(steps) == 0 {
		return head + m.laneEnd(end)
	}
	if len(steps) == 0 {
		return head + st.dim.Render(m.progress[k].text)
	}
	var parts []string
	for _, s := range steps {
		word := stepWords[s.step]
		switch s.state {
		case 0:
			parts = append(parts, st.green.Render("✓ "+word)+st.dim.Render(" "+took(s.took)))
		case 1:
			parts = append(parts, m.laneNow(l))
		default:
			parts = append(parts, st.dim.Render("· "+word))
		}
	}
	line := head + strings.Join(parts, st.border.Render(" ─ "))
	if ended {
		line += "   " + m.laneEnd(end)
	}
	return line
}

// laneNow is the step a lane is on: its word, how long so far, and for a
// wait, what it is for.
func (m *Model) laneNow(l *lane) string {
	st := m.st
	s := st.mauve.Render(m.spinner()) + " " + st.text.Render(stepWords[l.cur.step]) + st.muted.Render(" "+clock(m.set.Now().Sub(l.cur.at)))
	if w := checksWord(l.checks); w != "" {
		s += st.dim.Render(" · " + w)
	}
	return s
}

// laneEnd is how a pull request ended in its batch.
func (m *Model) laneEnd(p progress) string {
	switch p.result {
	case runner.Skipped:
		return m.st.yellow.Render("↷ " + p.text)
	case runner.Stopped, runner.Failed:
		return m.st.red.Render("✗ " + p.text)
	}
	return m.st.green.Render("✓ " + p.text)
}

// checksWord says what a wait is for: the checks by name when they are
// few, how many otherwise.
func checksWord(names []string) string {
	switch {
	case len(names) == 0:
		return ""
	case len(names) <= 2:
		return strings.Join(names, ", ") + " running"
	}
	return fmt.Sprintf("%d running", len(names))
}

// batchVerb names what a batch does.
func (m *Model) batchVerb(bs *batchState) string {
	switch {
	case bs.resumed:
		return "going on with merges"
	case bs.kind == runner.Review:
		return reviewingWords[bs.review.Event]
	}
	return batchWords[bs.kind]
}

// clock is a time taken, to the second: 7s, 1:05.
func clock(d time.Duration) string {
	s := int(d.Round(time.Second) / time.Second)
	if s < 60 {
		return fmt.Sprintf("%ds", s)
	}
	return fmt.Sprintf("%d:%02d", s/60, s%60)
}

// took is a step's time: tenths under a second.
func took(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	return clock(d)
}
