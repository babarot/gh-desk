package tui

import (
	"slices"
	"strings"
	"testing"

	"github.com/babarot/gh-desk/internal/plan"
)

// The names of the sections of the default review tab.
const (
	secYou   = "requested from you"
	secTeam  = "requested from a team"
	secOther = "other"
	secBots  = "bots"
)

// reviewSections are the sections of the default review tab, with those
// named in folded folded at start.
func reviewSections(folded ...string) []plan.Section {
	person, bot := false, true
	ss := []plan.Section{
		{Name: secYou, Rule: plan.Rule{Name: secYou, Requested: []string{"me"}, Bot: &person}},
		{Name: secTeam, Rule: plan.Rule{Name: secTeam, Requested: []string{"@*"}, Bot: &person}},
		{Name: secOther, Rule: plan.Rule{Name: secOther, Bot: &person}},
		{Name: secBots, Rule: plan.Rule{Name: secBots, Bot: &bot}},
	}
	for i := range ss {
		ss[i].Show = plan.ShowOpen
		if slices.Contains(folded, ss[i].Name) {
			ss[i].Show = plan.ShowFolded
		}
	}
	return ss
}

// pickSections are the sections of the default review tab named in names,
// in that order, then the others excluded, as a file of groups that left
// them out is rewritten.
func pickSections(names []string, folded ...string) []plan.Section {
	all := reviewSections(folded...)
	var out []plan.Section
	for _, n := range names {
		out = append(out, all[slices.IndexFunc(all, func(s plan.Section) bool { return s.Name == n })])
	}
	for _, s := range all {
		if !slices.Contains(names, s.Name) {
			s.Show = plan.ShowExcluded
			out = append(out, s)
		}
	}
	return out
}

// mineSections are the sections of the default tab of one's own pull
// requests: the drafts after the rest, with no heads.
func mineSections() []plan.Section {
	ready, draft := false, true
	return []plan.Section{
		{Show: plan.ShowOpen, Rule: plan.Rule{Draft: &ready}},
		{Show: plan.ShowOpen, Rule: plan.Rule{Draft: &draft}},
	}
}

// withSections are the test settings with the review tab split into the
// sections ss.
func withSections(ss ...plan.Section) Settings {
	set := testSettings()
	set.Tabs[0].Sections = ss
	return set
}

// heads are the names of the sections whose heads the tab shown draws, in
// order.
func heads(m *Model) []string {
	t := m.tab()
	var out []string
	for _, r := range m.rows(t) {
		if r.kind == rowGroup {
			out = append(out, t.sections[r.section].Name)
		}
	}
	return out
}

// A tab's sections split its rows by their rules, first come first: a
// section with no condition takes the rest, and one with no rows has no
// head.
func TestSectionsSplitTheRowsByRule(t *testing.T) {
	m := startSource(t, withCarol(), withSections(
		plan.Section{Name: "passing", Show: plan.ShowOpen, Rule: plan.Rule{Checks: []plan.Checks{plan.ChecksPassing}}},
		plan.Section{Name: "nothing", Show: plan.ShowOpen, Rule: plan.Rule{Authors: []string{"nobody"}}},
		plan.Section{Name: "the rest", Show: plan.ShowFolded},
	))
	if got, want := heads(m), []string{"passing", "the rest"}; !slices.Equal(got, want) {
		t.Fatalf("heads = %v, want %v\n%s", got, want, screen(m))
	}
	s := screen(m)
	if !strings.Contains(s, "▾ passing") || !strings.Contains(s, "▸ the rest") {
		t.Errorf("the heads are not drawn as their sections are shown:\n%s", s)
	}
	if !strings.Contains(s, "api#12") || !strings.Contains(s, "api#13") || strings.Contains(s, "api#30") {
		t.Errorf("the rows are not where their sections put them:\n%s", s)
	}
}

// An excluded section leaves its rows out as an exclusion rule does: the
// tab says how many, a shows them under the section's head, and says what
// excludes them.
func TestAnExcludedSectionLeavesItsRowsOut(t *testing.T) {
	m := startSource(t, withCarol(), withSections(
		plan.Section{Name: "can merge", Show: plan.ShowExcluded, Rule: plan.Rule{Checks: []plan.Checks{plan.ChecksPassing}}},
		plan.Section{Name: "the rest", Show: plan.ShowOpen},
	))
	s := screen(m)
	if strings.Contains(s, "api#12") || !strings.Contains(s, "2 pull requests excluded") {
		t.Fatalf("the excluded section's rows are shown, or not counted:\n%s", s)
	}
	press(m, "a")
	if !slices.Contains(heads(m), "can merge") || !strings.Contains(screen(m), "api#12") {
		t.Fatalf("a does not show the excluded section:\n%s", screen(m))
	}
	for _, r := range m.rows(m.tab()) {
		if r.kind == rowPR && r.pr.Key == key("acme", "api", 12) && !slices.Equal(r.ex.by, []string{"can merge"}) {
			t.Errorf("excluded by %v, want the section", r.ex.by)
		}
	}
}

// A section that cannot tell yet is passed over: the row goes in the next
// that takes it, not judged, and is not left out on what is not read.
func TestASectionThatCannotTellIsPassedOver(t *testing.T) {
	f := withCarol()
	f.noHeavy = true
	m := startSource(t, f, withSections(
		plan.Section{Name: "can merge", Show: plan.ShowExcluded, Rule: plan.Rule{Checks: []plan.Checks{plan.ChecksPassing}}},
		plan.Section{Name: "the rest", Show: plan.ShowOpen},
	))
	if s := screen(m); strings.Contains(s, "excluded") || !strings.Contains(s, "api#12") {
		t.Fatalf("a row was left out on checks not read:\n%s", s)
	}
	var r row
	for _, x := range m.rows(m.tab()) {
		if x.kind == rowPR && x.pr.Key == key("acme", "api", 12) {
			r = x
		}
	}
	if r.ex.at != 1 || !r.ex.pending || r.ex.excluded() {
		t.Errorf("row = %+v, want in the rest, pending, not excluded", r.ex)
	}
}

// What no section takes goes in other, made after the rest, or in the
// section the user called other: no row is lost, and none is unselected
// for leaving the tab.
func TestWhatNoSectionTakesGoesInOther(t *testing.T) {
	m := startSource(t, withCarol(), withSections(
		plan.Section{Name: "bots", Show: plan.ShowOpen, Rule: plan.Rule{Bot: new(true)}},
	))
	if got, want := heads(m), []string{"bots", "other"}; !slices.Equal(got, want) {
		t.Fatalf("heads = %v, want %v\n%s", got, want, screen(m))
	}
	if n := len(m.allOf(m.tab())); n != 4 {
		t.Errorf("the tab holds %d pull requests, want all 4", n)
	}
	m = startSource(t, withCarol(), withSections(
		plan.Section{Name: "other", Show: plan.ShowOpen, Rule: plan.Rule{Authors: []string{"nobody"}}},
		plan.Section{Name: "bots", Show: plan.ShowOpen, Rule: plan.Rule{Bot: new(true)}},
	))
	if got, want := heads(m), []string{"other", "bots"}; !slices.Equal(got, want) {
		t.Errorf("heads = %v, want %v: what nothing takes goes in the user's other\n%s", got, want, screen(m))
	}
}

// A row whose only section cannot tell yet is not lost: it waits in other.
func TestAPendingRowWithNoSectionWaitsInOther(t *testing.T) {
	f := withCarol()
	f.noHeavy = true
	m := startSource(t, f, withSections(
		plan.Section{Name: "passing", Show: plan.ShowOpen, Rule: plan.Rule{Checks: []plan.Checks{plan.ChecksPassing}}},
	))
	if got, want := heads(m), []string{"other"}; !slices.Equal(got, want) {
		t.Fatalf("heads = %v, want %v\n%s", got, want, screen(m))
	}
	for _, r := range m.rows(m.tab()) {
		if r.kind == rowPR && !r.ex.pending {
			t.Errorf("%v is not drawn as not judged", r.pr.Key)
		}
	}
}

// A section with no name has no head to unfold it by, so it never folds.
func TestASectionWithNoNameIsNotFolded(t *testing.T) {
	m := startSource(t, withCarol(), withSections(
		plan.Section{Show: plan.ShowFolded, Rule: plan.Rule{Bot: new(true)}},
		plan.Section{Name: "people", Show: plan.ShowOpen},
	))
	if got, want := heads(m), []string{"people"}; !slices.Equal(got, want) || !strings.Contains(screen(m), "api#30") {
		t.Errorf("heads = %v, want %v, with the bot's row shown\n%s", got, want, screen(m))
	}
	tb := m.tab()
	tb.setFolded(0, true)
	if tb.foldedAt(0) || tb.folded[""] {
		t.Errorf("a section with no name was folded: %v", tb.folded)
	}
}

// The inbox is split by its sections too, which may ask for the reason of
// the notification.
func TestInboxSectionsByReason(t *testing.T) {
	set := testSettings()
	set.Tabs[3].Sections = []plan.Section{{Name: "asked", Show: plan.ShowOpen, Rule: plan.Rule{Reasons: []string{"review_requested"}}}}
	m := startSource(t, fixture(), set)
	press(m, "4")
	s := screen(m)
	if got := heads(m); len(got) == 0 || got[0] != "asked" {
		t.Fatalf("heads = %v, want asked first\n%s", got, s)
	}
	if asked, row := strings.Index(s, "▾ asked"), strings.Index(s, "api#12"); asked < 0 || row < asked {
		t.Errorf("the notification asking for a review is not under its section:\n%s", s)
	}
}
