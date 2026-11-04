package tui

import (
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/babarot/gh-desk/internal/logging"
	"github.com/babarot/gh-desk/internal/model"
	"github.com/babarot/gh-desk/internal/runner"
)

// startLogged starts the TUI width wide and height high with a log of n
// records, every other one about the pull request under the cursor.
func startLogged(t *testing.T, width, height, n int) (*Model, *slog.Logger) {
	t.Helper()
	set := testSettings()
	recs := logging.NewRecords(0, set.Now)
	set.LogRecords = recs
	m := startSet(t, set, width)
	m.Update(tea.WindowSizeMsg{Width: width, Height: height})
	log := logging.Memory(recs, slog.LevelInfo).With(logging.KeyRun, "r1")
	p, _, ok := m.currentPR()
	if !ok {
		t.Fatal("no pull request under the cursor")
	}
	for i := range n {
		if i%2 == 0 {
			log.Info(fmt.Sprintf("about it %d", i), logging.KeyPR, p.Key.String())
		} else {
			log.Info(fmt.Sprintf("other %d", i), logging.KeyPR, "acme/other#1")
		}
	}
	return m, log
}

// On a wide screen the log viewer is drawn over the list's right side,
// and while it is open it takes the keys: the list's do nothing.
func TestLogOverTheList(t *testing.T) {
	m, log := startLogged(t, 130, 34, 4)
	press(m, "L")
	s := screen(m)
	if m.logInPane() || !strings.Contains(s, "╭ log ") || !strings.Contains(s, "about it 2") {
		t.Fatalf("the log is not over the list:\n%s", s)
	}
	if strings.Contains(s, "run=") {
		t.Errorf("the run's attribute is shown:\n%s", s)
	}
	if !strings.Contains(s, "╭  overview") {
		t.Errorf("the preview is not under it:\n%s", s)
	}
	sel := m.tab().sel
	press(m, "i", "j", "M")
	if m.claudeOpen || m.tab().sel != sel || m.bulk != nil {
		t.Errorf("a key of the list acted while the log was open")
	}
	log.Info("written after")
	if !strings.Contains(screen(m), "written after") {
		t.Errorf("a record written while open is not shown:\n%s", screen(m))
	}
	if !strings.Contains(screen(m), "this PR only") {
		t.Errorf("the key line does not tell the log's keys:\n%s", screen(m))
	}
	press(m, "esc")
	if m.logv != nil || strings.Contains(screen(m), "╭ log ") {
		t.Errorf("esc did not close the log:\n%s", screen(m))
	}
}

// On a narrow screen the log viewer takes the bottom pane's place, with
// the preview off as well; where no pane fits, L says so.
func TestLogInThePanesPlace(t *testing.T) {
	m, _ := startLogged(t, 100, 34, 4)
	press(m, "p", "L")
	s := screen(m)
	if !m.logInPane() || m.layout().pane == 0 {
		t.Fatalf("the log is not in the pane's place:\n%s", s)
	}
	if _, y := lineOf(t, m, "╭ log "); y != m.paneTop(m.layout()) {
		t.Errorf("the log starts at line %d, not at the pane's top:\n%s", y, s)
	}
	press(m, "L")
	if m.layout().pane != 0 {
		t.Errorf("closing the log left a pane with the preview off")
	}

	// Made too short for it while open, it closes rather than take the
	// keys unseen.
	press(m, "L")
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 12})
	if m.logv != nil {
		t.Errorf("the log stayed open with no room for it")
	}

	short, _ := startLogged(t, 100, 12, 4)
	press(short, "L")
	if short.logv != nil || !strings.Contains(screen(short), "no room for the log") {
		t.Errorf("L opened the log on a screen too short for it:\n%s", screen(short))
	}
}

// Moving back stops following and says how to follow again; the bottom
// follows again. f narrows the log to the pull request under the cursor
// when it opened.
func TestLogScrollsAndNarrows(t *testing.T) {
	m, _ := startLogged(t, 130, 34, 60)
	press(m, "L")
	if !strings.Contains(screen(m), "about it 58") {
		t.Fatalf("the newest record is not shown:\n%s", screen(m))
	}
	press(m, "ctrl+u", "ctrl+u")
	s := screen(m)
	if !strings.Contains(s, "paused · "+m.firstKey("bottom")+" to follow") || strings.Contains(s, "about it 58") {
		t.Errorf("moving back did not stop following:\n%s", s)
	}
	press(m, "G")
	if !strings.Contains(screen(m), "following") || !strings.Contains(screen(m), "about it 58") {
		t.Errorf("G did not follow again:\n%s", screen(m))
	}
	press(m, "f")
	s = screen(m)
	if !strings.Contains(s, " only") || strings.Contains(s, "other 57") || !strings.Contains(s, "about it 58") {
		t.Errorf("f did not narrow the log to the pull request:\n%s", s)
	}
}

// The mouse on the log viewer scrolls it; pointing outside closes it and
// goes on to what is there.
func TestLogMouse(t *testing.T) {
	m, _ := startLogged(t, 130, 34, 60)
	press(m, "L")
	w, _ := m.logWidth()
	mouseAt(m, tea.MouseWheelMsg{X: 130 - w + 2, Y: 5, Button: tea.MouseWheelUp})
	if m.logv == nil || m.logv.follow {
		t.Fatalf("the wheel on the log did not scroll it")
	}
	mouseAt(m, tea.MouseClickMsg{X: 2, Y: 5, Button: tea.MouseLeft})
	if m.logv != nil {
		t.Errorf("a click on the list did not close the log")
	}
}

// ? on the log viewer opens the keys with the log's first.
func TestLogHelp(t *testing.T) {
	m, _ := startLogged(t, 130, 34, 2)
	press(m, "L", "?")
	if s := helpText(m); !strings.HasPrefix(s, "▾ the log · works here") {
		t.Errorf("the log's section is not first:\n%s", s)
	}
}

// A line too narrow for even one wide character ends: the rest is
// dropped, not wrapped onto empty lines forever.
func TestLogWrapEndsOnANarrowScreen(t *testing.T) {
	m, _ := startLogged(t, 130, 34, 0)
	for width := 15; width <= 24; width++ {
		lines := wrapWords("12:00:00 INFO  ", 15, []logWord{{m.st.text, "msg"}, {m.st.dim, " k="}, {m.st.muted, "日本語の値"}}, width)
		if len(lines) > 20 {
			t.Errorf("width %d wrapped into %d lines", width, len(lines))
		}
	}
}

// startBatch starts a batch of two pull requests in m, as the runner's
// events would leave it after a while: one waiting for its checks after
// approving, one merged. It returns a function that feeds more events.
func startBatch(t *testing.T, m *Model, id string) (func(time.Duration, runner.Event), model.PRKey, model.PRKey) {
	t.Helper()
	a, b := key("acme", "api", 12), key("acme", "web", 7)
	clock := now
	m.set.Now = func() time.Time { return clock }
	bs := &batchState{kind: runner.Merge, goes: map[model.PRKey]bool{a: true, b: true}, order: []model.PRKey{a, b}, done: map[model.PRKey]progress{}}
	m.addBatch(id, bs)
	ch := make(chan runner.Event)
	feed := func(after time.Duration, ev runner.Event) {
		clock = clock.Add(after)
		m.event(eventMsg{batch: id, ev: ev, ok: true, ch: ch})
	}
	feed(0, runner.Event{Key: a, Forecast: true, Plan: []runner.Step{runner.StepApprove, runner.StepWait, runner.StepMerge}})
	feed(0, runner.Event{Key: b, Forecast: true, Plan: []runner.Step{runner.StepApprove, runner.StepMerge}})
	feed(0, runner.Event{Key: a, Step: runner.StepApprove, Text: "approving"})
	feed(0, runner.Event{Key: b, Step: runner.StepApprove, Text: "approving"})
	feed(time.Second, runner.Event{Key: b, Step: runner.StepMerge, Text: "merging"})
	feed(time.Second, runner.Event{Key: b, Result: runner.Merged, Text: "merged", Method: model.MethodSquash})
	feed(0, runner.Event{Key: a, Forecast: true, Plan: []runner.Step{runner.StepWait, runner.StepMerge}})
	feed(0, runner.Event{Key: a, Step: runner.StepWaitChecks, Text: "waiting for checks", Checks: []string{"build", "lint"}})
	feed(0, runner.Event{Key: a, Forecast: true, Plan: []runner.Step{runner.StepMerge}})
	feed(5*time.Second, runner.Event{Key: a, Step: runner.StepWaitChecks, Text: "waiting for checks", Checks: []string{"build"}})
	return feed, a, b
}

// While a batch runs, the log viewer shows a line for each pull request
// above the log: its steps as dots, then what it does now, a wait with
// what it is for; and the status line counts how they stand.
func TestProgressOverTheList(t *testing.T) {
	m, _ := startLogged(t, 130, 34, 2)
	startBatch(t, m, "b1")
	press(m, "L")
	s := screen(m)
	for _, want := range []string{"╭ progress ", "approving and merging · 1/2 · 7s", "api#12", "●◐○", "checks 5s · build running", "web#7", "●●", "✓ merged (squash)", "╭ log "} {
		if !strings.Contains(s, want) {
			t.Errorf("no %q in the progress pane:\n%s", want, s)
		}
	}
	if !strings.Contains(s, "approving and merging… 1/2 · 1 done · 1 waiting") {
		t.Errorf("the status line does not count how they stand:\n%s", s)
	}
}

// In the bottom pane's place the steps are side by side, with their
// times.
func TestProgressInThePanesPlace(t *testing.T) {
	m, _ := startLogged(t, 100, 34, 2)
	startBatch(t, m, "b1")
	press(m, "L")
	if s := screen(m); !strings.Contains(s, "✓ approve 2s ─ ") || !strings.Contains(s, "· merge") {
		t.Errorf("the steps are not side by side:\n%s", s)
	}
}

// A batch that ends while the viewer is open stays, done; one started
// while it is open joins; reopening shows only those running.
func TestProgressStaysUntilClosed(t *testing.T) {
	m, _ := startLogged(t, 130, 34, 2)
	press(m, "L")
	if strings.Contains(screen(m), "╭ progress") {
		t.Fatalf("a progress pane with no batch running:\n%s", screen(m))
	}
	feed, a, _ := startBatch(t, m, "b1")
	if !strings.Contains(screen(m), "╭ progress ") {
		t.Fatalf("a batch started while open did not join:\n%s", screen(m))
	}
	feed(time.Second, runner.Event{Key: a, Step: runner.StepMerge, Text: "merging"})
	feed(time.Second, runner.Event{Key: a, Result: runner.Merged, Text: "merged", Method: model.MethodMerge})
	if s := screen(m); !strings.Contains(s, "progress ✓") || !strings.Contains(s, "done in 9s") {
		t.Errorf("the batch that ended did not stay, done:\n%s", s)
	}
	press(m, "L", "L")
	if strings.Contains(screen(m), "╭ progress") {
		t.Errorf("reopening showed a batch that had ended:\n%s", screen(m))
	}
}

// What GitHub said reaches the log viewer cleaned: a check's name or an
// error with an escape sequence or a line break draws neither.
func TestLogViewerCleansWhatGitHubSaid(t *testing.T) {
	m, log := startLogged(t, 130, 34, 0)
	feed, a, _ := startBatch(t, m, "b1")
	feed(0, runner.Event{Key: a, Step: runner.StepWaitChecks, Text: "waiting for checks", Checks: []string{"evil\x1b[2Jname\nnext"}})
	log.Warn("search read in part", "err", "bad\x1b]0;title\x07\nline")
	press(m, "L")
	s, _ := m.render()
	if strings.Contains(s, "\x1b[2J") || strings.Contains(s, "\x1b]0;") || strings.Contains(s, "\x07") {
		t.Errorf("an escape sequence from GitHub reached the screen")
	}
	if strings.Count(screen(m), "\n") != 33 {
		t.Errorf("a line break from GitHub added a line:\n%s", screen(m))
	}
}
