package tui

import (
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/babarot/gh-desk/internal/model"
	"github.com/babarot/gh-desk/internal/runner"
	"github.com/babarot/gh-desk/internal/tui/keymap"
)

// scopedBy are the test settings with every tab narrowed by scope, value
// chosen at start, and the layout that keeps it.
func scopedBy(scope, value string) (Settings, *fakeLayout) {
	set := testSettings()
	lay := &fakeLayout{}
	set.Scope, set.ScopeValue, set.Layout = scope, value, lay
	return set, lay
}

// prKeys are the pull requests the rows of tab i hold, with no section
// folded.
func prKeys(m *Model, i int) []model.PRKey {
	var out []model.PRKey
	for _, r := range m.listed(m.tabs[i]) {
		out = append(out, r.pr.Key)
	}
	return out
}

// With a scope set, the header shows the scope switch where the viewer was,
// and the help says who is signed in.
func TestScopeSwitchTakesTheViewersPlace(t *testing.T) {
	set, _ := scopedBy(ScopeOwner, "")
	m := startSet(t, set, 130)
	h := headerLine(m)
	if !strings.HasPrefix(h, "desk // ‹ all  › ") || strings.Contains(h, "@me") {
		t.Errorf("header = %q", h)
	}
	press(m, "?")
	if !strings.Contains(screen(m), "signed in as @me") {
		t.Errorf("the help does not say who is signed in:\n%s", screen(m))
	}
	// With none set, the header is as it was.
	if h := headerLine(startSet(t, testSettings(), 130)); !strings.HasPrefix(h, "desk // @me") {
		t.Errorf("header with no scope = %q", h)
	}
}

// > and < go through the values by name, all first, round; each narrows
// every tab, counts it, and leaves the owner out of the PR column. The one
// chosen is kept.
func TestScopeNarrowsEveryTab(t *testing.T) {
	set, lay := scopedBy(ScopeOwner, "")
	m := startSet(t, set, 130)
	press(m, ">")
	if m.scope != "acme" {
		t.Fatalf("scope = %q, want acme", m.scope)
	}
	if h := headerLine(m); !strings.Contains(h, "‹ acme › ") || !strings.Contains(h, "2 mine (0)") {
		t.Errorf("header = %q", h)
	}
	if got := prKeys(m, 1); len(got) != 0 {
		t.Errorf("mine holds %v under acme", got)
	}
	if s := screen(m); !strings.Contains(s, " api#12 ") {
		t.Errorf("the PR column keeps the owner chosen:\n%s", s)
	}
	press(m, ">")
	if m.scope != "me" || len(prKeys(m, 0)) != 0 || len(prKeys(m, 1)) != 1 {
		t.Errorf("scope = %q, review %v, mine %v", m.scope, prKeys(m, 0), prKeys(m, 1))
	}
	press(m, ">")
	if m.scope != "" {
		t.Errorf("scope = %q, want all after the last", m.scope)
	}
	press(m, "<")
	if m.scope != "me" {
		t.Errorf("< from all = %q, want the last", m.scope)
	}
	press(m, "<")
	if !slices.Equal(lay.scopes, []string{"owner=acme", "owner=me", "owner=", "owner=me", "owner=acme"}) {
		t.Errorf("kept %v", lay.scopes)
	}
}

// A value of the repository scope leaves the repository out of the PR
// column, and narrows the inbox too.
func TestScopeOfARepository(t *testing.T) {
	set, _ := scopedBy(ScopeRepo, "acme/web")
	m := startSet(t, set, 130)
	if got := prKeys(m, 0); !slices.Equal(got, []model.PRKey{key("acme", "web", 7)}) {
		t.Fatalf("review = %v", got)
	}
	if got := prKeys(m, 3); len(got) != 0 {
		t.Errorf("inbox = %v under acme/web", got)
	}
	set, _ = scopedBy(ScopeRepo, "acme/api")
	m = startSet(t, set, 130)
	// The pane's edge names the pull request in full; the row does not.
	row := ""
	for l := range strings.SplitSeq(screen(m), "\n") {
		if strings.Contains(l, "Add retries") && !strings.Contains(l, "╭") {
			row = l
			break
		}
	}
	if !strings.Contains(row, " #12 ") || strings.Contains(row, "api#12") {
		t.Errorf("the PR column names the repository chosen: %q", row)
	}
}

// The value kept from the last launch is taken at start, and let go once
// every tab is read without it, for this launch only.
func TestScopeKeptIsLetGoWhenNoTabHoldsIt(t *testing.T) {
	set, lay := scopedBy(ScopeOwner, "gone")
	m := startSet(t, set, 130)
	if m.scope != "" || len(lay.scopes) != 0 {
		t.Errorf("scope = %q, kept %v; want all, and nothing written", m.scope, lay.scopes)
	}
	set, _ = scopedBy(ScopeOwner, "me")
	if m := startSet(t, set, 130); m.scope != "me" {
		t.Errorf("scope = %q, want me, which mine holds", m.scope)
	}
}

// The values are those of every tab, but one only every tab leaves out: the
// inbox's merged acme/api#9 adds no repository of its own.
func TestScopeValues(t *testing.T) {
	set, _ := scopedBy(ScopeRepo, "")
	m := startSet(t, set, 130)
	if got, want := m.scopeChoices(), []string{"", "acme/api", "acme/web", "me/tool"}; !slices.Equal(got, want) {
		t.Errorf("choices = %q, want %q", got, want)
	}
}

// O opens the scope chooser: what is typed narrows the values, enter
// chooses one, and esc closes it choosing none. Its text box has the
// terminal's cursor.
func TestScopeChooser(t *testing.T) {
	set, _ := scopedBy(ScopeOwner, "")
	m := startSet(t, set, 130)
	press(m, "O")
	s := screen(m)
	if !strings.Contains(s, "choose an owner for every tab") || !strings.Contains(s, "acme") || !strings.Contains(s, "• all") {
		t.Fatalf("chooser:\n%s", s)
	}
	press(m, "m", "e")
	if got := m.chooserValues(); !slices.Equal(got, []string{"acme", "me"}) {
		t.Errorf("values for me = %q", got)
	}
	out, cur := m.render()
	lines := strings.Split(ansi.Strip(out), "\n")
	if cur == nil || cur.Y >= len(lines) || !strings.HasSuffix(ansi.Truncate(lines[cur.Y], cur.X, ""), "me") {
		t.Errorf("cursor = %+v", cur)
	}
	press(m, "down", "enter")
	if m.chooser != nil || m.scope != "me" {
		t.Errorf("chooser open = %v, scope = %q", m.chooser != nil, m.scope)
	}
	press(m, "O", "esc")
	if m.chooser != nil || m.scope != "me" {
		t.Errorf("esc: chooser open = %v, scope = %q", m.chooser != nil, m.scope)
	}
}

// The marks of the scope switch step through the values, and its name
// opens the chooser.
func TestScopeSwitchClicks(t *testing.T) {
	set, _ := scopedBy(ScopeOwner, "")
	m := startSet(t, set, 130)
	at := func(s string) int {
		h := ansi.Strip(headerLine(m))
		return ansi.StringWidth(h[:strings.Index(h, s)])
	}
	click := func(x int) { mouseAt(m, tea.MouseClickMsg{X: x, Y: 0, Button: tea.MouseLeft}) }
	click(at("›"))
	if m.scope != "acme" {
		t.Fatalf("› gave %q", m.scope)
	}
	click(at("‹"))
	if m.scope != "" {
		t.Errorf("‹ gave %q", m.scope)
	}
	click(at("all"))
	if m.chooser == nil {
		t.Error("a click on the name opens no chooser")
	}
}

// With no scope set, its keys say so and do nothing.
func TestScopeKeysWithNoneSet(t *testing.T) {
	m := startSet(t, testSettings(), 130)
	press(m, ">")
	if m.scope != "" || !strings.Contains(status(m), "no scope is set") {
		t.Errorf("scope = %q, status = %q", m.scope, status(m))
	}
	press(m, "O")
	if m.chooser != nil {
		t.Error("O opened a chooser with no scope set")
	}
}

// What a scope leaves out is not acted on from the rows: V selects only what
// is in it.
func TestScopeLeavesItsRowsOutOfActions(t *testing.T) {
	set, _ := scopedBy(ScopeAuthor, "alice")
	m := New(fixture(), &fakeRunner{results: map[model.PRKey]runner.Event{}}, &fakeClaude{}, keymap.Default(), nil, set)
	m.Update(tea.WindowSizeMsg{Width: 130, Height: 34})
	run(m, m.Init())
	press(m, "V")
	if len(m.selected) != 1 || !m.isSelected(key("acme", "api", 12)) {
		t.Errorf("selected %v", m.selected)
	}
}

// How the scope switch and the chooser look.
func TestScopeGolden(t *testing.T) {
	set, _ := scopedBy(ScopeOwner, "acme")
	m := startSet(t, set, 130)
	golden(t, "scope-switch", m)
	press(m, "O")
	golden(t, "scope-chooser", m)
}
