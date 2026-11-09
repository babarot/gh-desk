package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/babarot/gh-desk/internal/model"
)

// involvedPR is a pull request of the involved tab, updated ago.
func involvedPR(n int, title string, ago time.Duration) model.PR {
	return model.PR{ID: title, Key: key("acme", "lib", n), Title: title, Author: "alice", State: model.StateOpen,
		HeadOID: title, BaseRef: "main", Updated: now.Add(-ago)}
}

// involvedOf is the fixture with the involved tab listing prs, shown, with
// the cursor put on b by the user.
func involvedOf(t *testing.T, prs []model.PR, b model.PRKey) (*fakeSource, *Model) {
	t.Helper()
	f := fixture()
	f.searches["involved"] = prs
	m := startSource(t, f, testSettings())
	press(m, "3")
	cursorTo(t, m, b)
	return f, m
}

func onPR(t *testing.T, m *Model, k model.PRKey) {
	t.Helper()
	if r, ok := m.current(); !ok || r.kind != rowPR || r.pr.Key != k {
		t.Fatalf("the cursor is on %v, want %s:\n%s", r.pr.Key, k, screen(m))
	}
}

// r puts away the merged rows above the cursor; it stays on its pull
// request, not on the one that moved up into its line.
func TestRefreshPuttingMergedAwayKeepsTheCursor(t *testing.T) {
	a, b, c := involvedPR(1, "First", time.Hour), involvedPR(2, "Second", 2*time.Hour), involvedPR(3, "Third", 3*time.Hour)
	f, m := involvedOf(t, []model.PR{a, b, c}, b.Key)
	m.merged[a.Key] = true
	f.searches["involved"] = []model.PR{b, c}
	press(m, "r")
	onPR(t, m, b.Key)
}

// The pull request under the cursor leaving the list leaves its row on its
// line, marked, until the cursor leaves it; r puts it away.
func TestARowLeavingStaysUnderTheCursor(t *testing.T) {
	a, b, c := involvedPR(1, "First", time.Hour), involvedPR(2, "Second", 2*time.Hour), involvedPR(3, "Third", 3*time.Hour)
	f, m := involvedOf(t, []model.PR{a, b, c}, b.Key)
	f.searches["involved"] = []model.PR{a, c}
	run(m, m.reload())
	onPR(t, m, b.Key)
	if r, _ := m.current(); !r.gone {
		t.Error("the row kept is not marked as gone")
	}
	if s := status(m); !strings.Contains(s, "no longer in this list") {
		t.Errorf("the status line does not say the row left: %q", s)
	}
	press(m, "j")
	onPR(t, m, c.Key)
	if s := screen(m); strings.Contains(s, "Second") {
		t.Errorf("the row stayed after the cursor left it:\n%s", s)
	}

	f, m = involvedOf(t, []model.PR{a, b, c}, b.Key)
	f.searches["involved"] = []model.PR{a, c}
	run(m, m.reload())
	onPR(t, m, b.Key)
	press(m, "r")
	if s := screen(m); strings.Contains(s, "Second") {
		t.Errorf("r did not put away the row kept:\n%s", s)
	}
	onPR(t, m, c.Key)
}

// Before the user has moved or pressed anything, the cursor is not held
// on a pull request: as the first reading fills the list, it stays on top.
func TestTheCursorFollowsOnlyWhatTheUserSaw(t *testing.T) {
	a, b := involvedPR(1, "First", time.Hour), involvedPR(2, "Second", 2*time.Hour)
	f := fixture()
	f.searches["involved"] = []model.PR{a, b}
	m := startSource(t, f, testSettings())
	inv := m.tabs[2]
	if inv.seen {
		t.Fatal("a tab never shown has its cursor seen")
	}
	f.searches["involved"] = []model.PR{involvedPR(3, "Newer", time.Minute), a, b}
	run(m, m.loadTab(2))
	press(m, "3")
	onPR(t, m, key("acme", "lib", 3))
}

// Back on a tab whose rows above the cursor left while it was not shown,
// the cursor is on its pull request, not on the one now at its line.
func TestTheCursorStaysOnItsPRInATabNotShown(t *testing.T) {
	a, b, c, d := involvedPR(1, "First", time.Hour), involvedPR(2, "Second", 2*time.Hour), involvedPR(3, "Third", 3*time.Hour), involvedPR(4, "Fourth", 4*time.Hour)
	f, m := involvedOf(t, []model.PR{a, b, c, d}, c.Key)
	press(m, "1")
	f.searches["involved"] = []model.PR{c, d}
	run(m, m.loadTab(2))
	press(m, "3")
	onPR(t, m, c.Key)
}
