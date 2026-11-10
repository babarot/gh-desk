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

// The filter narrows the excluded as it narrows the rows, out of sight: the
// line under the rows counts only those a would show, and leaves when none
// of them match.
func TestFilterNarrowsTheExcludedCount(t *testing.T) {
	m := startSource(t, withCarol(), excluding(plan.Rule{Name: "carol's", Authors: []string{"carol"}}))
	m.filter = "slow"
	if strings.Contains(screen(m), "api#13") {
		t.Fatalf("the filter shows the excluded pull request:\n%s", screen(m))
	}
	if !strings.Contains(screen(m), "1 pull request excluded · a to show") {
		t.Errorf("the filter matching the excluded hides the count:\n%s", screen(m))
	}
	m.filter = "retries"
	if strings.Contains(screen(m), "excluded · a to show") {
		t.Errorf("the count has an excluded row the filter hides:\n%s", screen(m))
	}
	m.filter = ""
	if !strings.Contains(screen(m), "1 pull request excluded · a to show") {
		t.Errorf("clearing the filter does not bring the count back:\n%s", screen(m))
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

// A row that both an exclusion rule and an excluded section leave out is
// counted once.
func TestExcludedTwiceIsCountedOnce(t *testing.T) {
	set := excluding(plan.Rule{Name: "bots", Bot: new(true)})
	set.Tabs[0].Sections = pickSections([]string{secYou, secTeam}, secTeam)
	m := startSource(t, fixture(), set)
	if !strings.Contains(screen(m), "1 pull request excluded") {
		t.Errorf("the bot's pull request is not counted once:\n%s", screen(m))
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

// A row an exclusion rule cannot tell yet is shown, marked ? when the
// shimmer is off and shimmering when it is on; it leaves once the rule
// can tell, and the cursor stays on its pull request meanwhile.
func TestPendingRowsAreShownUntilTold(t *testing.T) {
	f := withCarol()
	carol := &f.searches["review"][len(f.searches["review"])-1]
	carol.Labels = []string{"wip"}
	f.noHeavy = true
	m := startSource(t, f, excluding(plan.Rule{Name: "wip", Labels: []string{"wip"}}))
	if !strings.Contains(screen(m), "? api#13") || !strings.Contains(screen(m), "? api#12") {
		t.Fatalf("the rows the rule cannot tell are not marked:\n%s", screen(m))
	}
	cursorTo(t, m, key("acme", "api", 12))

	// The heavy fields arrive: carol's has the label, alice's does not.
	f.noHeavy = false
	ids := []string{"1", "5"}
	got, _ := f.Heavy(t.Context(), ids)
	_, cmd := m.Update(heavyMsg{job: heavyJob{tab: 0, gen: m.tabs[0].gen, ids: ids}, got: got})
	run(m, cmd)
	if strings.Contains(screen(m), "api#13") {
		t.Errorf("the row the rule now matches is still shown:\n%s", screen(m))
	}
	if strings.Contains(screen(m), "? api#12") {
		t.Errorf("the row the rule now tells is still marked:\n%s", screen(m))
	}
	if r, ok := m.current(); !ok || r.kind != rowPR || r.pr.Key != key("acme", "api", 12) {
		t.Errorf("the cursor left acme/api#12 when the row above it went: on %+v", r)
	}

	set := excluding(plan.Rule{Name: "wip", Labels: []string{"wip"}})
	set.Shimmer = true
	f.noHeavy = true
	m = startSource(t, f, set)
	if strings.Contains(screen(m), "? api#12") || !strings.Contains(screen(m), "api#12") {
		t.Errorf("with the shimmer on, the row is marked ? or not shown:\n%s", screen(m))
	}
	if !m.pendingShown() {
		t.Error("pendingShown is false with rows waiting")
	}
}

// The inbox read again keeps the cursor on its pull request when a new
// notification comes in above it.
func TestInboxReadAgainKeepsTheCursor(t *testing.T) {
	f := fixture()
	m := startSource(t, f, testSettings())
	press(m, "4")
	cursorTo(t, m, key("acme", "api", 12))
	f.notifs = append([]model.Notification{{ThreadID: "n3", Key: key("acme", "api", 30), Title: "Update dependency octo/app to v0.13.2",
		Reason: "review_requested", Unread: true, Updated: now.Add(-time.Minute)}}, f.notifs...)
	press(m, "r")
	if !strings.Contains(screen(m), "api#30") {
		t.Fatalf("the new notification is not read:\n%s", screen(m))
	}
	if r, ok := m.current(); !ok || r.kind != rowPR || r.pr.Key != key("acme", "api", 12) {
		t.Errorf("the cursor left acme/api#12 when a notification came in above it: on %v", r.pr.Key)
	}
}

// The line that says how many the tab leaves out stays when the rows fill
// the list, and the rows scroll above it.
func TestExcludedLineStaysUnderAFullList(t *testing.T) {
	set := excluding(plan.Rule{Name: "carol's", Authors: []string{"carol"}})
	set.Tabs[0].Sections = reviewSections()
	m := startSource(t, withCarol(), set)
	m.Update(tea.WindowSizeMsg{Width: 130, Height: 18})
	if got := len(m.rows(m.tab())); got <= m.layout().list-1 {
		t.Fatalf("the rows (%d) do not fill the list (%d lines)", got, m.layout().list)
	}
	if !strings.Contains(screen(m), "1 pull request excluded · a to show") {
		t.Errorf("the count goes when the rows fill the list:\n%s", screen(m))
	}
	for range 10 {
		press(m, "j")
	}
	// The screen is drawn after each key, which scrolls the rows.
	shown := screen(m)
	r, _ := m.current()
	if !m.onScreen(r.pr.Key) || !strings.Contains(shown, "excluded · a to show") {
		t.Errorf("the cursor's row or the count is off the screen at the bottom:\n%s", screen(m))
	}
}

// A search tab's count leaves out what it excludes, while it does not show
// them.
func TestSearchCountLeavesOutTheExcluded(t *testing.T) {
	m := startSource(t, withCarol(), excluding(plan.Rule{Name: "carol's", Authors: []string{"carol"}}))
	if !strings.Contains(headerLine(m), "1 review (3)") {
		t.Errorf("the count has the excluded: %q", headerLine(m))
	}
	press(m, "a")
	if !strings.Contains(headerLine(m), "1 review (4)") {
		t.Errorf("the count leaves out what a shows: %q", headerLine(m))
	}
}

// A rule with no name is called by what it asks for.
func TestUnnamedRuleIsCalledByItsConditions(t *testing.T) {
	m := startSource(t, withCarol(), excluding(plan.Rule{Authors: []string{"carol"}}))
	press(m, "a")
	cursorTo(t, m, key("acme", "api", 13))
	if got := status(m); got != "excluded by carol" {
		t.Errorf("status = %q, want the rule's condition", got)
	}
}

// A rule on the state, unnamed, is called by it, and what it leaves out of
// a search tab is not counted as the inbox's merged or closed.
func TestStateRuleIsNotTheInboxSettled(t *testing.T) {
	f := withCarol()
	f.searches["review"][len(f.searches["review"])-1].State = model.StateMerged
	m := startSource(t, f, excluding(plan.Rule{States: []model.PRState{model.StateMerged, model.StateClosed}}))
	if !strings.Contains(screen(m), "1 pull request excluded · a to show") {
		t.Errorf("the count of the state rule:\n%s", screen(m))
	}
	press(m, "a")
	cursorTo(t, m, key("acme", "api", 13))
	if got := status(m); got != "excluded by state merged or closed" {
		t.Errorf("status = %q", got)
	}
}

// The pane's read of what a merge would do can tell that a pull request
// waits only for an approval, which an exclusion rule on its checks reads:
// a row leaving above the cursor does not move the cursor onto another.
func TestPaneReadKeepsTheCursor(t *testing.T) {
	f := fixture()
	waiting := model.PR{ID: "9", Key: key("acme", "api", 20), Title: "Deploy the client", Author: "dave", State: model.StateOpen,
		HeadRef: "deploy", HeadOID: "fffffff9", BaseRef: "main", Checks: model.CheckPending, Requested: []model.Reviewer{{Login: "me"}},
		Updated: now.Add(-10 * time.Minute)}
	f.searches["review"] = append(f.searches["review"], waiting)
	f.info = map[model.PRKey]func(*model.MergeInfo){waiting.Key: func(mi *model.MergeInfo) {
		mi.Checks = []model.Check{{Name: "deploy/prod", State: model.CheckPending}}
	}}
	set := excluding(plan.Rule{Checks: []plan.Checks{plan.ChecksPassing}, Authors: []string{"dave"}})
	set.Plan.ApprovalChecks = []string{"deploy/*"}
	m := startSource(t, f, set)
	// As if its read were still on the way: it is not known to wait only
	// for an approval, so it is shown, above the cursor. Moving the cursor
	// past it would read it, so the cursor is put on b, as the user would.
	delete(m.cache.info, waiting.Key)
	b := f.searches["review"][0].Key
	for i, r := range m.rows(m.tab()) {
		if r.kind == rowPR && r.pr.Key == b {
			m.tab().sel = i
		}
	}
	m.moved(m.tab())
	m.tab().seen = true
	if !strings.Contains(screen(m), "Deploy the client") {
		t.Fatalf("not shown before the read:\n%s", screen(m))
	}
	got, _ := f.MergeInfo(t.Context(), []string{waiting.ID}, true)
	_, cmd := m.Update(infoMsg{key: waiting.Key, mi: got[waiting.ID], ok: true})
	run(m, cmd)
	if r, ok := m.current(); !ok || r.pr.Key != b {
		t.Errorf("the cursor moved off %s:\n%s", b, screen(m))
	}
}
