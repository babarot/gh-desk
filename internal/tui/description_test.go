package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/babarot/gh-triage/internal/model"
	"github.com/babarot/gh-triage/internal/tui/chat"
	"github.com/babarot/gh-triage/internal/tui/keymap"
)

func TestMarkdownLines(t *testing.T) {
	m := start(t, fixture(), keymap.Default())
	body := "## Why\n\nUploads gave up on a `502`.\n\n" +
		"- retry up to five times, waiting twice as long each time\n- [x] tests pass\n- [ ] tried on staging\n\n" +
		"> quoted\n\n```go\nwait := time.Second\n```\n\n| code | retry |\n|---|---|\n| 502 | yes |\n| 404 | no |"
	var got []string
	for _, l := range m.bodyLines(body, 30) {
		got = append(got, ansi.Strip(l.text))
	}
	want := []string{
		"Why",
		"Uploads gave up on a 502.",
		"",
		"• retry up to five times,",
		"  waiting twice as long each",
		"  time",
		"☑ tests pass",
		"☐ tried on staging",
		"",
		"┃ quoted",
		"",
		" wait := time.Second  go ",
		"",
		"code │ retry",
		"─────┼──────",
		"502  │ yes  ",
		"404  │ no   ",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("lines:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// A line does not start with the comma after code, nor with a Japanese
// full stop; each goes down with what comes before it, and the lines of
// a list item stay under its text.
func TestWrapKeepsWhatMayNotStartALine(t *testing.T) {
	m := start(t, fixture(), keymap.Default())
	for _, tc := range []struct {
		body  string
		width int
		want  []string
	}{
		{"through `validator.ts`, then `open.ts`", 20, []string{"through", "validator.ts, then", "open.ts"}},
		{"読み込みを直しました。", 20, []string{"読み込みを直しまし", "た。"}},
		{"1. see `a.go`, `b.go`, and `c.go`", 11, []string{"1. see", "   a.go,", "   b.go,", "   and c.go"}},
	} {
		var got []string
		for _, l := range m.markdownLines(tc.body, tc.width) {
			got = append(got, ansi.Strip(l.text))
		}
		if strings.Join(got, "\n") != strings.Join(tc.want, "\n") {
			t.Errorf("%q at %d:\n%s\nwant:\n%s", tc.body, tc.width, strings.Join(got, "\n"), strings.Join(tc.want, "\n"))
		}
	}
}

func TestPlainDescriptionIsAsWritten(t *testing.T) {
	m := start(t, fixture(), keymap.Default())
	m.set.PlainDescription = true
	got := ansi.Strip(m.bodyLines("## Why\n- [x] `go test`", 40)[1].text)
	if got != "- [x] `go test`" {
		t.Errorf("line = %q, want it as written", got)
	}
}

func TestDescriptionPaneParts(t *testing.T) {
	f := fixture()
	d := f.details[key("acme", "api", 12)]
	d.Body += strings.Repeat("\n\nMore about the retries.", 6)
	f.details[key("acme", "api", 12)] = d
	m := start(t, f, keymap.Default())
	if s := screen(m); !strings.Contains(s, "▾ description") || !strings.Contains(s, "│ status    open") {
		t.Fatalf("no parts or facts:\n%s", s)
	}
	// ] goes to the checks; z folds the part the pane starts in.
	press(m, "]")
	if !strings.Contains(screen(m), "│ ▾ checks (1)") || strings.Contains(screen(m), "▾ description") {
		t.Fatalf("] did not go to the checks:\n%s", screen(m))
	}
	press(m, "z")
	if !strings.Contains(screen(m), "▸ checks (1)") || strings.Contains(screen(m), "✓ test") {
		t.Errorf("z did not fold the checks:\n%s", screen(m))
	}
	// A click on a part's header folds or unfolds it.
	press(m, "[")
	x, y := lineOf(t, m, "▾ description")
	mouseAt(m, tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
	if !strings.Contains(screen(m), "▸ description") || strings.Contains(screen(m), "Retry failed requests.") {
		t.Errorf("a click did not fold the description:\n%s", screen(m))
	}
}

func TestNarrowPaneHasFactsAbove(t *testing.T) {
	m := start(t, fixture(), keymap.Default())
	m.Update(tea.WindowSizeMsg{Width: 90, Height: 40})
	s := screen(m)
	if strings.Contains(s, " │ status") {
		t.Fatalf("a column of facts at 90 cells:\n%s", s)
	}
	_, facts := lineOf(t, m, "│ status    open")
	_, parts := lineOf(t, m, "▾ description")
	if facts > parts {
		t.Errorf("the facts are under the parts:\n%s", s)
	}
}

func TestClickFactOpensItsPage(t *testing.T) {
	f := fixture()
	f.searches["review"][0].Labels = []string{"good first issue"}
	m := start(t, f, keymap.Default())
	click := func(s string) {
		t.Helper()
		x, y := lineOf(t, m, s)
		clickTwice(m, x+1, y)
	}
	click("@core")
	click("good first issue")
	click("+40 −3 · 2 files")
	click("status") // has no page
	want := []string{
		"https://github.com/orgs/acme/teams/core",
		`https://github.com/acme/api/pulls?q=is%3Apr+is%3Aopen+label%3A%22good+first+issue%22`,
		"https://github.com/acme/api/pull/12/files",
	}
	if strings.Join(f.browsed, " ") != strings.Join(want, " ") {
		t.Fatalf("browsed %q, want %q", f.browsed, want)
	}
}

func TestFactsWrapAndCountWhatIsCut(t *testing.T) {
	f := fixture()
	f.searches["review"][0].HeadRef = "babarot/fix-the-retry-that-gave-up-too-early"
	m := start(t, f, keymap.Default())
	if s := screen(m); !strings.Contains(s, "branch    babarot/fix-the-retry-th │") || !strings.Contains(s, "│           at-gave-up-too-early ") {
		t.Fatalf("the branch is not wrapped:\n%s", s)
	}
	// Six lines left in the pane: the last says what is cut.
	press(m, "-", "-", "-", "-", "-", "-")
	if s := screen(m); !strings.Contains(s, "│ +1 more") {
		t.Errorf("no count of the facts cut:\n%s", s)
	}
}

func TestCheckOrderPutsFailedFirst(t *testing.T) {
	cs := []model.Check{
		{Name: "test", State: model.CheckSuccess},
		{Name: "CodeQL", State: model.CheckPending},
		{Name: "lint", State: model.CheckFailure},
		{Name: "build", State: model.CheckSuccess},
		{Name: "deploy", State: model.CheckError},
	}
	var got []string
	for _, c := range checkOrder(cs) {
		got = append(got, c.Name)
	}
	if want := "lint deploy CodeQL test build"; strings.Join(got, " ") != want {
		t.Errorf("order = %q, want %q", strings.Join(got, " "), want)
	}
}

func TestClickTitleLineOpensThePROrItsAuthor(t *testing.T) {
	f := fixture()
	m := start(t, f, keymap.Default())
	x, y := lineOf(t, m, "│ #12 Add retries")
	clickTwice(m, x+3, y)
	x, y = lineOf(t, m, "alice │ status")
	clickTwice(m, x+1, y)
	// The title itself has no page.
	x, y = lineOf(t, m, "│ #12 Add retries")
	clickTwice(m, x+8, y)
	want := []string{"https://github.com/acme/api/pull/12", "https://github.com/alice"}
	if strings.Join(f.browsed, " ") != strings.Join(want, " ") {
		t.Fatalf("browsed %q, want %q", f.browsed, want)
	}
}

// A comment in the pane's comments part is drawn from its Markdown, as
// the description is.
func TestCommentIsDrawn(t *testing.T) {
	f := fixture()
	d := f.details[key("acme", "api", 12)]
	d.Comments = []model.Comment{{Author: "dave", Body: "## Timeout\n\n- [x] covered by `wait`"}}
	f.details[key("acme", "api", 12)] = d
	m := start(t, f, keymap.Default())
	press(m, "+", "+", "+", "+", "+", "+")
	if s := screen(m); !strings.Contains(s, "☑ covered by wait") || strings.Contains(s, "## Timeout") {
		t.Errorf("the comment was not drawn from its Markdown:\n%s", s)
	}
	m.set.PlainDescription = true
	if s := screen(m); !strings.Contains(s, "- [x] covered by `wait`") {
		t.Errorf("the comment was not shown as written:\n%s", s)
	}
}

func TestChatAnswerIsDrawnFromMarkdown(t *testing.T) {
	m := start(t, fixture(), keymap.Default())
	c := kept(t, "", chat.Turn{Text: "## Verdict\n\n**Safe** to merge, see [the docs](https://example.com).\n\n- `go test` passes"})
	var got []string
	for _, l := range m.chatLines(c, 40) {
		got = append(got, ansi.Strip(l))
	}
	want := []string{
		"  ⏺ Verdict",
		"    Safe to merge, see the docs.",
		"",
		"    • go test passes",
		"",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("lines:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}
