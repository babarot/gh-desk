package tui

import (
	"fmt"
	"log/slog"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/babarot/gh-desk/internal/logging"
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
