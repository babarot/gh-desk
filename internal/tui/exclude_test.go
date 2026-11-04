package tui

import (
	"regexp"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/babarot/gh-desk/internal/model"
	"github.com/babarot/gh-desk/internal/plan"
	"github.com/babarot/gh-desk/internal/runner"
	"github.com/babarot/gh-desk/internal/tui/keymap"
)

// startSource makes the TUI over f with the settings set, sized, with every
// tab read.
func startSource(t *testing.T, f *fakeSource, set Settings) *Model {
	t.Helper()
	m := New(f, &fakeRunner{results: map[model.PRKey]runner.Event{}}, &fakeClaude{}, keymap.Default(), nil, set)
	m.Update(tea.WindowSizeMsg{Width: 130, Height: 34})
	run(m, m.Init())
	return m
}

// withCarol is the fixture with one more pull request requested from you,
// by carol, so that the group has two.
func withCarol() *fakeSource {
	f := fixture()
	f.searches["review"] = append(f.searches["review"], model.PR{ID: "5", Key: key("acme", "api", 13), Title: "Trace the slow queries",
		Author: "carol", State: model.StateOpen, HeadRef: "trace", HeadOID: "eeeeeee5", BaseRef: "main", Additions: 5, Deletions: 1, ChangedFiles: 1,
		Decision: model.DecisionReviewRequired, Checks: model.CheckSuccess, Requested: []model.Reviewer{{Login: "me"}},
		Updated: now.Add(-time.Hour)})
	return f
}

// cursorTo moves the cursor down to the row of k, from the top.
func cursorTo(t *testing.T, m *Model, k model.PRKey) {
	t.Helper()
	press(m, "g")
	for range 20 {
		if r, ok := m.current(); ok && r.kind == rowPR && r.pr.Key == k {
			return
		}
		press(m, "j")
	}
	t.Fatalf("no row of %s:\n%s", k, screen(m))
}

func excluding(rs ...plan.Rule) Settings {
	set := testSettings()
	set.Exclude = rs
	return set
}

// A pull request an exclusion rule matches leaves the tab, which says how
// many it leaves out; a shows it dimmed, and says by what when the cursor
// is on it.
func TestExcludeLeavesRowsOutUntilShown(t *testing.T) {
	m := startSource(t, withCarol(), excluding(plan.Rule{Name: "carol's", Authors: []string{"carol"}}))
	if strings.Contains(screen(m), "api#13") {
		t.Fatalf("the excluded pull request is shown:\n%s", screen(m))
	}
	if !strings.Contains(screen(m), "1 pull request excluded · a to show") {
		t.Errorf("the tab does not say what it leaves out:\n%s", screen(m))
	}
	if !strings.Contains(screen(m), "a excluded") {
		t.Errorf("the key line does not tell a:\n%s", screen(m))
	}
	press(m, "a")
	if !strings.Contains(screen(m), "⊘ api#13") {
		t.Fatalf("a does not show the excluded row with its mark:\n%s", screen(m))
	}
	if strings.Contains(screen(m), "excluded · a to show") {
		t.Errorf("the count stays while the excluded are shown:\n%s", screen(m))
	}
	cursorTo(t, m, key("acme", "api", 13))
	if got := status(m); got != "excluded by carol's" {
		t.Errorf("status = %q, want what excludes the row", got)
	}
	if !strings.Contains(screen(m), "excluded  carol's") {
		t.Errorf("the pane does not say what excludes the row:\n%s", screen(m))
	}
	press(m, "a")
	if strings.Contains(screen(m), "api#13") {
		t.Errorf("a again does not hide it:\n%s", screen(m))
	}
}

// a keeps the cursor on its pull request while rows come and go above it.
func TestShowExcludedKeepsTheCursor(t *testing.T) {
	m := startSource(t, withCarol(), excluding(plan.Rule{Name: "carol's", Authors: []string{"carol"}}))
	press(m, "a")
	cursorTo(t, m, key("acme", "api", 12))
	press(m, "a")
	if r, ok := m.current(); !ok || r.pr.Key != key("acme", "api", 12) {
		t.Errorf("the cursor is on %v, want acme/api#12", r.pr.Key)
	}
	press(m, "a")
	if r, ok := m.current(); !ok || r.pr.Key != key("acme", "api", 12) {
		t.Errorf("the cursor is on %v after showing again, want acme/api#12", r.pr.Key)
	}
}

// Nothing out of sight is selected: not by a group's header, not by S.
func TestExcludedAreNotSelected(t *testing.T) {
	set := excluding(plan.Rule{Name: "carol's", Authors: []string{"carol"}})
	set.Rules = []plan.Rule{{Name: "any", Title: regexp.MustCompile(`.`)}}
	m := startSource(t, withCarol(), set)
	press(m, "g", "space")
	if m.isSelected(key("acme", "api", 13)) || !m.isSelected(key("acme", "api", 12)) {
		t.Fatalf("the header selected %v, want alice's alone", m.selected)
	}
	press(m, "u", "S", "j", "enter")
	if m.isSelected(key("acme", "api", 13)) {
		t.Errorf("S selected the excluded pull request: %v", m.selected)
	}
	if !m.isSelected(key("acme", "api", 12)) {
		t.Errorf("S did not select what is shown: %v", m.selected)
	}
	// Shown with a, they are rows like any other.
	press(m, "u", "a", "S", "j", "enter")
	if !m.isSelected(key("acme", "api", 13)) {
		t.Errorf("S did not select the excluded row shown with a: %v", m.selected)
	}
}

// A selection made before a row is excluded stays, as one a filter hides
// does; d acts only on what is shown.
func TestHideSkipsTheExcludedSelection(t *testing.T) {
	m := startSource(t, withCarol(), excluding(plan.Rule{Name: "carol's", Authors: []string{"carol"}}))
	press(m, "a")
	cursorTo(t, m, key("acme", "api", 13))
	press(m, "space", "a", "d")
	if !m.isSelected(key("acme", "api", 13)) {
		t.Errorf("the excluded selection was put away out of sight: %v", m.selected)
	}
	if got := status(m); !strings.Contains(got, "none of the selection is shown here") {
		t.Errorf("status = %q", got)
	}
}

// The inbox counts its merged or closed notifications among the excluded,
// and its unread count leaves out what it excludes; show_settled keeps
// them as rows.
func TestInboxExcludes(t *testing.T) {
	m := startSource(t, fixture(), excluding(plan.Rule{Name: "alice's", Authors: []string{"alice"}}))
	press(m, "4")
	if !strings.Contains(screen(m), "2 pull requests excluded (1 merged or closed, 2 unread) · a to show") {
		t.Errorf("the inbox does not count what it excludes:\n%s", screen(m))
	}
	if !strings.Contains(headerLine(m), "4 inbox (0)") {
		t.Errorf("the unread count counts the excluded: %q", headerLine(m))
	}
	press(m, "a")
	if !strings.Contains(headerLine(m), "4 inbox (2)") {
		t.Errorf("the unread count leaves out what a shows: %q", headerLine(m))
	}

	set := testSettings()
	for i := range set.Tabs {
		set.Tabs[i].ShowSettled = set.Tabs[i].Inbox
	}
	m = startSource(t, fixture(), set)
	press(m, "4")
	if strings.Contains(screen(m), "excluded") || !strings.Contains(screen(m), "api#9") {
		t.Errorf("show_settled still excludes the merged one:\n%s", screen(m))
	}
}

// A reason is matched on the inbox's rows, which have a notification, and
// on no other tab's.
func TestExcludeByReason(t *testing.T) {
	m := startSource(t, fixture(), excluding(plan.Rule{Name: "asked", Reasons: []string{"review_requested"}}))
	if strings.Contains(screen(m), "excluded") || !strings.Contains(screen(m), "api#12") {
		t.Errorf("a reason excludes a row with no notification:\n%s", screen(m))
	}
	press(m, "4", "a")
	if !strings.Contains(screen(m), "⊘ api#12") {
		t.Errorf("the notification asking for a review is not excluded:\n%s", screen(m))
	}
}

// A group the tab does not show has no rows to show with a, so what it
// would exclude is not counted.
func TestExcludedOfAGroupNotShownAreNotCounted(t *testing.T) {
	set := excluding(plan.Rule{Name: "bots", Bot: true})
	set.Tabs[0].Groups = []plan.Group{plan.GroupYou, plan.GroupTeam}
	m := startSource(t, fixture(), set)
	if strings.Contains(screen(m), "excluded") {
		t.Errorf("the tab counts a pull request of a group it does not show:\n%s", screen(m))
	}
}

// A triage is given the rows the tab shows: the excluded only while a
// shows them, and then marked.
func TestTriageIsGivenTheExcludedOnlyWhenShown(t *testing.T) {
	m := startSource(t, withCarol(), excluding(plan.Rule{Name: "carol's", Authors: []string{"carol"}}))
	given := func() map[model.PRKey]bool {
		s, _ := m.snapshot()
		out := map[model.PRKey]bool{}
		for _, r := range s.Rows {
			out[r.PR.Key] = r.Excluded
		}
		return out
	}
	if ex, ok := given()[key("acme", "api", 13)]; ok {
		t.Errorf("the triage is given an excluded row out of sight (excluded=%v)", ex)
	}
	press(m, "a")
	if ex, ok := given()[key("acme", "api", 13)]; !ok || !ex {
		t.Errorf("the excluded row shown with a is given %v, marked %v; want given, marked", ok, ex)
	}
}
