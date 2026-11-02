package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/babarot/gh-triage/internal/model"
	"github.com/babarot/gh-triage/internal/plan"
	"github.com/babarot/gh-triage/internal/runner"
	"github.com/babarot/gh-triage/internal/tui/keymap"
)

// startSet starts the TUI with set, width columns wide.
func startSet(t *testing.T, set Settings, width int) *Model {
	t.Helper()
	m := New(fixture(), &fakeRunner{results: map[model.PRKey]runner.Event{}}, &fakeClaude{}, keymap.Default(), nil, set)
	m.Update(tea.WindowSizeMsg{Width: width, Height: 34})
	run(m, m.Init())
	return m
}

func headerLine(m *Model) string { return strings.SplitN(screen(m), "\n", 2)[0] }

// moreTabs are the test tabs with four more searches before the inbox:
// eight in all.
func moreTabs() Settings {
	set := testSettings()
	inbox := set.Tabs[len(set.Tabs)-1]
	set.Tabs = set.Tabs[:len(set.Tabs)-1]
	for _, n := range []string{"team-a", "team-b", "docs", "deps"} {
		set.Tabs = append(set.Tabs, Tab{Name: n, Query: n, Limit: 100})
	}
	set.Tabs = append(set.Tabs, inbox)
	return set
}

func TestInboxAnywhere(t *testing.T) {
	set := testSettings()
	set.Tabs = []Tab{{Name: "通知", Inbox: true, Limit: 100}, set.Tabs[0]}
	m := startSet(t, set, 130)
	if h := headerLine(m); !strings.Contains(h, "1 通知") || !strings.Contains(h, "2 review") {
		t.Fatalf("header = %q; want the inbox first, under its own name", h)
	}
	if m.inboxTab() != m.tabs[0] || !m.inboxTab().loaded {
		t.Fatal("the inbox is not the first tab, or was not read")
	}
	// The unread marks come from the inbox wherever it is.
	press(m, "2")
	if !strings.Contains(screen(m), "● api#12") {
		t.Errorf("no unread mark on the review tab:\n%s", screen(m))
	}
}

func TestGroupsInTheOrderGiven(t *testing.T) {
	set := testSettings()
	set.Tabs[0].Groups = []plan.Group{plan.GroupBots, plan.GroupYou}
	set.Tabs[0].Folded = nil
	m := startSet(t, set, 130)
	s := screen(m)
	bots, you := strings.Index(s, "▾ bots"), strings.Index(s, "▾ requested from you")
	if bots < 0 || you < 0 || bots > you {
		t.Fatalf("want bots, then you:\n%s", s)
	}
	if strings.Contains(s, "requested from a team") || strings.Contains(s, "Rework the settings page") {
		t.Errorf("a group left out of groups is shown:\n%s", s)
	}
	if h := headerLine(m); !strings.Contains(h, "1 bots · 1 to you") || strings.Contains(h, "to a team") {
		t.Errorf("header = %q", h)
	}
}

func TestTabKeysToNine(t *testing.T) {
	m := startSet(t, moreTabs(), 200)
	press(m, "5")
	if m.cur != 4 {
		t.Fatalf("5 went to tab %d, want 4", m.cur)
	}
	press(m, "9")
	if m.cur != 4 {
		t.Errorf("9 moved to tab %d with eight tabs", m.cur)
	}
	press(m, "8")
	if !m.tab().inbox {
		t.Error("8 did not go to the inbox")
	}
	press(m, "?")
	if s := helpText(m); !strings.Contains(s, "tab 1 to 8") || strings.Contains(s, "9") {
		t.Errorf("the ? list should name tabs 1 to 8 only:\n%s", s)
	}
}

func TestTabsThatDoNotFit(t *testing.T) {
	m := startSet(t, moreTabs(), 105)
	h := headerLine(m)
	if !strings.Contains(h, "1 review") || strings.Contains(h, "8 inbox") || !strings.Contains(h, " …") {
		t.Fatalf("header = %q; want the first tabs, then …", h)
	}
	press(m, "8")
	h = headerLine(m)
	if !strings.Contains(h, "… ") || strings.Contains(h, "1 review") || !strings.Contains(h, "8 inbox") {
		t.Fatalf("header = %q; want …, then the last tabs", h)
	}
	// Going back to a tab that is shown leaves the tabs where they are.
	press(m, "5")
	if got := headerLine(m); !strings.Contains(got, "8 inbox") {
		t.Errorf("header = %q; the tabs moved though 5 was shown", got)
	}
	// A click goes to the tab under it, past the … mark.
	x, y := lineOf(t, m, "6 docs")
	mouseAt(m, tea.MouseClickMsg{X: x + 2, Y: y, Button: tea.MouseLeft})
	if m.cur != 5 {
		t.Errorf("click went to tab %d, want 5", m.cur)
	}
}

func TestGroupLeftOutIsNotSelected(t *testing.T) {
	// A rule that picks bots' pull requests: with bots left out of the
	// tab's groups, renovate's is neither shown nor picked.
	set := testSettings()
	set.Rules = []plan.Rule{{Name: "bots", Bot: true}}
	set.Tabs[0].Groups = []plan.Group{plan.GroupYou, plan.GroupTeam}
	m := startSet(t, set, 130)
	for _, r := range m.rules() {
		if hits := m.ruleMatches(r); len(hits) > 0 {
			t.Errorf("rule %q picked %v from groups not shown", r.Name, hits[0].Key)
		}
	}
	set.Tabs[0].Groups = plan.Groups
	m = startSet(t, set, 130)
	picked := false
	for _, r := range m.rules() {
		picked = picked || len(m.ruleMatches(r)) > 0
	}
	if !picked {
		t.Fatal("no rule picks the bot's pull request with bots shown; the test proves nothing")
	}
}

func TestConfigWarningsAtStart(t *testing.T) {
	set := testSettings()
	set.ConfigWarnings = []string{"~/.config/gh-triage/config.toml:4:9: tabs[0].query has no is:pr"}
	m := startSet(t, set, 130)
	if s := screen(m); !strings.Contains(s, "the config file has 1 warning") {
		t.Fatalf("no notification of the warning at start:\n%s", s)
	}
	press(m, "n")
	s := screen(m)
	if !strings.Contains(s, "config.toml:4:9: tabs[0].query has no is:pr") || strings.Contains(s, "#0") {
		t.Errorf("the warning is not listed as itself:\n%s", s)
	}
	// It names no pull request, so choosing it only closes the list.
	press(m, "down", "enter")
	if m.notesAt != nil || m.view != viewList {
		t.Errorf("notes still open, or the view moved: %v", m.view)
	}
}
