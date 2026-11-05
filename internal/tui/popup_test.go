package tui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/babarot/gh-desk/internal/model"
	"github.com/babarot/gh-desk/internal/plan"
	"github.com/babarot/gh-desk/internal/runner"
	"github.com/babarot/gh-desk/internal/tui/keymap"
)

// longFixture adds pull requests with long names and titles, all made up.
func longFixture() (*fakeSource, []model.PRKey) {
	f := fixture()
	var keys []model.PRKey
	for i := range 20 {
		k := key("acme", fmt.Sprintf("a-repository-with-a-rather-long-name-%02d", i), 1000+i)
		f.searches["review"] = append(f.searches["review"], model.PR{
			ID: fmt.Sprintf("long%d", i), Key: k, Title: "Update the dependency that has a very long name to its latest version", Author: "renovate",
			AuthorIsBot: true, State: model.StateOpen, HeadRef: "renovate/long", HeadOID: fmt.Sprintf("eeeeee%02d", i), BaseRef: "main",
			Additions: 1, Deletions: 1, ChangedFiles: 1, Decision: model.DecisionReviewRequired, Checks: model.CheckSuccess,
			Requested: []model.Reviewer{{Login: "me"}}, Updated: now.Add(-time.Duration(i) * time.Minute),
		})
		keys = append(keys, k)
	}
	return f, keys
}

// startSmall starts the TUI over f on a small terminal.
func startSmall(t *testing.T, f *fakeSource) *Model {
	t.Helper()
	m := start(t, f, keymap.Default())
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	return m
}

// readPopup reads all of a popup: what it shows, then again after down
// while its bottom edge tells of lines below. Every time, the popup must
// fit the screen and show its foot. It returns the text, spaces collapsed.
// linesOnly drops where a popup's text box starts.
func linesOnly(render func() ([]string, int)) func() []string {
	return func() []string { l, _ := render(); return l }
}

func readPopup(t *testing.T, m *Model, render func() []string, foot string, down string) string {
	t.Helper()
	var text []string
	for range 80 {
		ls := render()
		if len(ls) > m.popupHeight() {
			t.Fatalf("%d lines, over the %d a popup may take:\n%s", len(ls), m.popupHeight(), ansi.Strip(strings.Join(ls, "\n")))
		}
		for _, l := range ls {
			if w := ansi.StringWidth(l); w > m.width {
				t.Fatalf("%d cells, wider than the screen: %q", w, ansi.Strip(l))
			}
		}
		if f := ansi.Strip(ls[len(ls)-2]); !strings.Contains(f, foot) {
			t.Fatalf("the foot is not in sight: %q", f)
		}
		for _, l := range ls[1 : len(ls)-1] {
			l = ansi.Strip(l)
			l = strings.TrimSuffix(strings.TrimPrefix(l, "│ "), " │")
			text = append(text, l)
		}
		if !strings.Contains(ansi.Strip(ls[len(ls)-1]), "↓") {
			return strings.Join(strings.Fields(strings.Join(text, " ")), " ")
		}
		press(m, down)
	}
	t.Fatal("the popup did not reach its end")
	return ""
}

// has fails when text does not hold want whole, spaces collapsed.
func has(t *testing.T, text, want string) {
	t.Helper()
	if want = strings.Join(strings.Fields(want), " "); !strings.Contains(text, want) {
		t.Errorf("%q is not read whole in:\n%s", want, text)
	}
}

func selectAll(m *Model, keys []model.PRKey) {
	for _, k := range keys {
		m.selected[k] = byHand
	}
}

// On a small terminal, what must be read in a popup is wrapped, not cut,
// and a popup taller than the screen scrolls, keeping its question and
// keys in sight.
func TestPopupsOnASmallScreen(t *testing.T) {
	t.Run("bulk", func(t *testing.T) {
		f, keys := longFixture()
		m := startSmall(t, f)
		selectAll(m, keys)
		press(m, "A")
		text := readPopup(t, m, m.renderBulk, "yes", "j")
		has(t, text, "GitHub cannot take an approval or a merge back. Each acts only on the head shown when it was read.")
		// A name may be cut, but never its number.
		has(t, text, "#1019")
	})
	t.Run("comment", func(t *testing.T) {
		f, keys := longFixture()
		m := startSmall(t, f)
		selectAll(m, keys)
		press(m, "c")
		readPopup(t, m, linesOnly(m.renderCompose), "discard", "j")
		body := "First line of a comment that is long enough to be wrapped in the popup, then more.\nsecond line"
		run(m, m.paste(tea.PasteMsg{Content: body}))
		press(m, "ctrl+s")
		text := readPopup(t, m, m.renderConfirm, "yes", "j")
		has(t, text, "First line of a comment that is long enough to be wrapped in the popup, then more.")
		has(t, text, "Adds the comment to each conversation; a review asked of you stays asked.")
		has(t, text, "more")
	})
	t.Run("menu", func(t *testing.T) {
		f, keys := longFixture()
		m := startSmall(t, f)
		selectAll(m, keys)
		press(m, "enter")
		text := readPopup(t, m, m.renderMenu, "close", "j")
		for _, it := range m.menuItems() {
			has(t, text, it.about)
		}
	})
	t.Run("notifications", func(t *testing.T) {
		m := startSmall(t, fixture())
		long := "GraphQL: Pull request is not mergeable: the base branch policy prohibits the merge, and a required status check is expected."
		m.pushNote("✗", "merge failed", []noteItem{{key: key("acme", "api", 12), text: long, result: runner.Failed}})
		m.openNotes()
		has(t, readPopup(t, m, m.renderNotes, "close", "j"), long)
	})
	t.Run("select by rule", func(t *testing.T) {
		m := startSmall(t, fixture())
		m.set.Rules = append(m.set.Rules, plan.Rule{Name: "many bumps", Authors: []string{"renovate"},
			Packages: []string{"octo/app", "octo/cli", "octo/server", "octo/client", "octo/release-tools", "octo/schema"}})
		press(m, "S")
		text := readPopup(t, m, m.renderPick, "cancel", "j")
		has(t, text, ruleWords(m.set.Rules[len(m.set.Rules)-1]))
	})
	t.Run("claude", func(t *testing.T) {
		m, _ := startTriage(t, `{"reply":"Nothing to do.","actions":[]}`)
		m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
		long := "show the bot pull requests whose checks pass and which change nothing but the lock files of the repository"
		m.triage.turns = append([]triageTurn{{request: long, reply: strings.Repeat("A reply of many lines. ", 30)}}, m.triage.turns...)
		m.triage.top, m.triage.follow = 0, false
		has(t, readPopup(t, m, linesOnly(m.renderTriage), "close", "j"), long)
	})
	t.Run("help", func(t *testing.T) {
		m := startSmall(t, fixture())
		m.Update(tea.WindowSizeMsg{Width: 50, Height: 24})
		press(m, "?")
		clear(m.helpFolded)
		all, _, _, _ := m.helpShown()
		text := strings.Join(strings.Fields(ansi.Strip(strings.Join(all, " "))), " ")
		for _, s := range m.helpSections() {
			for _, r := range s.rows {
				has(t, text, r[1])
			}
		}
	})
}

// A note's results hang from it, the last closing the tree, and one that
// wraps goes on under its text.
func TestNoteResultsHang(t *testing.T) {
	m := startSmall(t, fixture())
	m.pushNote("✓", "merged 1, 1 failed", []noteItem{
		{key: key("acme", "api", 12), text: "merged (merge commit)", result: runner.Merged},
		{key: key("acme", "api", 30), text: "GraphQL: Pull request is not mergeable: the base branch policy prohibits the merge.", result: runner.Failed},
	})
	m.openNotes()
	var got []string
	for _, l := range m.renderNotes()[1:6] {
		got = append(got, strings.TrimRight(strings.TrimSuffix(strings.TrimPrefix(ansi.Strip(l), "│ "), "│"), " "))
	}
	want := []string{
		"▎ ✓ merged 1, 1 failed  1m ago",
		"  ├ acme/api#12                    merged (merge commit)",
		"  └ acme/api#30                    GraphQL: Pull request is not",
		"                                   mergeable: the base branch policy",
		"                                   prohibits the merge.",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("got\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// A merge names its method only when it is not the first of merge.methods:
// the repository does not allow the one the user expects.
func TestBulkNamesAnUnusualMethod(t *testing.T) {
	f := fixture()
	other := model.PR{ID: "5", Key: key("me", "site", 8), Title: "Fix the footer", Author: "me", State: model.StateOpen,
		HeadOID: "eeeeeee5", BaseRef: "main", Checks: model.CheckSuccess, Updated: now.Add(-time.Hour)}
	f.searches["mine"] = append(f.searches["mine"], other)
	f.info = map[model.PRKey]func(*model.MergeInfo){other.Key: func(mi *model.MergeInfo) {
		mi.Repo.Methods = []model.MergeMethod{model.MethodSquash}
	}}
	set := testSettings()
	set.Plan.Methods = []model.MergeMethod{model.MethodMerge, model.MethodSquash}
	m := New(f, &fakeRunner{results: map[model.PRKey]runner.Event{}}, &fakeClaude{}, keymap.Default(), nil, set)
	m.Update(tea.WindowSizeMsg{Width: 130, Height: 34})
	run(m, m.Init())
	press(m, "2")
	selectAll(m, []model.PRKey{key("me", "tool", 3), other.Key})
	press(m, "M")
	text := readPopup(t, m, m.renderBulk, "yes", "j")
	has(t, text, "Fix the footer merge now (squash)")
	has(t, text, "Document the flags merge now")
	if strings.Contains(text, "merge commit") {
		t.Errorf("the first of merge.methods is named:\n%s", text)
	}
}

// The confirm popup shows every note of a pull request under it, and
// what its steps mean once below the list: that a draft stays ready and
// that auto-merge takes later commits (ADR-046, ADR-075).
func TestBulkShowsEveryNote(t *testing.T) {
	f := fixture()
	draft := model.PR{ID: "5", Key: key("me", "tool", 4), Title: "Try a new parser", Author: "me", State: model.StateOpen, Draft: true,
		HeadOID: "eeeeeee5", BaseRef: "main", Checks: model.CheckSuccess, Updated: now.Add(-time.Hour)}
	f.searches["mine"] = append(f.searches["mine"], draft)
	f.info = map[model.PRKey]func(*model.MergeInfo){draft.Key: func(mi *model.MergeInfo) {
		mi.Draft = true
		mi.Repo.AutoMerge = true
		mi.Files = []string{".github/workflows/ci.yaml", "main.go", ".github/workflows/release.yaml"}
	}}
	m := start(t, f, keymap.Default())
	press(m, "2")
	selectAll(m, []model.PRKey{draft.Key})
	press(m, "M")
	text := readPopup(t, m, m.renderBulk, "yes", "j")
	has(t, text, "changes .github/workflows/ci.yaml, .github/workflows/release.yaml")
	has(t, text, "ready → turn on auto-merge")
	if strings.Contains(text, "merge commit") {
		t.Errorf("the first of merge.methods is named:\n%s", text)
	}
	has(t, text, "Marking a draft ready asks for its reviews; it stays ready if the merge stops.")
	has(t, text, "Auto-merge also merges commits pushed after it is on")
	if strings.Count(text, "stays ready") != 1 {
		t.Errorf("what marking it ready means is said more than once:\n%s", text)
	}
}
