package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/babarot/gh-triage/internal/claude"
	"github.com/babarot/gh-triage/internal/keymap"
)

func stripped(ls []string) string {
	out := make([]string, len(ls))
	for i, l := range ls {
		out[i] = strings.TrimRight(ansi.Strip(l), " ")
	}
	return strings.Join(out, "\n")
}

// Once Claude has answered, the tools it used are one line that counts
// them in the order they were first used, and the answer ends with what
// it took.
func TestChatAnswerCountsItsTools(t *testing.T) {
	m := start(t, fixture(), keymap.Default())
	c := &chat{turns: []turn{
		{you: true, text: "Review this pull request.", at: now},
		{text: "Fine.", steps: []claude.Step{{Tool: "Grep"}, {Tool: "Read"}, {Tool: "Grep"}}, at: now.Add(72 * time.Second), cost: 0.41},
		{you: true, text: "And the tests?", at: now.Add(5 * time.Minute)},
		{text: "Fine too.", at: now.Add(5*time.Minute + 3*time.Second)},
	}}
	want := strings.Join([]string{
		"> Review this pull request.",
		"",
		"  ⏺ 3 tools  Grep ×2 · Read ×1",
		"",
		"  ⏺ Fine.",
		"",
		"    3 tools · 1m12s · $0.41",
		"",
		"> And the tests?",
		"",
		"  ⏺ Fine too.",
		"",
		"    3s",
		"",
	}, "\n")
	if got := stripped(m.chatLines(c, 60)); got != want {
		t.Errorf("lines:\n%s\nwant:\n%s", got, want)
	}
}

// While Claude answers, the last tools it used are shown with what each
// was used on, a path in the worktree from the worktree, and how many
// came before them.
func TestChatShowsTheLastToolsWhileAnswering(t *testing.T) {
	m := start(t, fixture(), keymap.Default())
	dir := "/cache/worktrees/acme/api/12"
	c := &chat{dir: dir, running: true, since: now.Add(-11 * time.Second), turns: []turn{{you: true, text: "Why?"}}, steps: []claude.Step{
		{Tool: "Grep", Arg: "retry"},
		{Tool: "Read", Arg: dir + "/a.go"},
		{Tool: "Grep", Arg: "func Do"},
		{Tool: "Read", Arg: dir + "/internal/retry/retry.go"},
	}}
	got := stripped(m.chatLines(c, 60))
	for _, w := range []string{
		"  … 1 earlier\n  ⏺ Read(a.go)\n  ⏺ Grep(func Do)\n  ⏺ Read(internal/retry/retry.go)\n\n",
		"thinking… 11s",
	} {
		if !strings.Contains(got, w) {
			t.Errorf("no %q in:\n%s", w, got)
		}
	}
	if strings.Contains(got, "Grep(retry)") || strings.Contains(got, dir) {
		t.Errorf("an earlier tool or the worktree is shown:\n%s", got)
	}
}

// A path too long for the line is cut in the middle, so its file name
// stays.
func TestStepArgKeepsTheFileName(t *testing.T) {
	for _, tc := range []struct{ arg, want string }{
		{"/wt/src/features/channel/data/repositories/SlackRepository.ts", "src/features/c…/SlackRepository.ts"},
		{"/elsewhere/a/very/long/path/to/SlackRepository.ts", "/elsewhere/a/v…/SlackRepository.ts"},
		{"/wt/src/AVeryLongFileNameThatDoesNotFitAtAll.ts", "AVeryLongFileNameThatDoesNotFitAt…"},
		{"from .*/legacy/slack/apiError|isSlackAPIError|other", "from .*/legacy/slack/apiError|isS…"},
	} {
		if got := stepArg("/wt", tc.arg, 34); got != tc.want || ansi.StringWidth(got) > 34 {
			t.Errorf("stepArg(%q) = %q, want %q", tc.arg, got, tc.want)
		}
	}
}

// A question is shown in full, each of its lines wrapped under the
// first.
func TestChatShowsTheWholeQuestion(t *testing.T) {
	m := start(t, fixture(), keymap.Default())
	c := &chat{turns: []turn{{you: true, text: "日本語でまとめ直して。\nあと 2 の件、影響範囲はどこまで？"}}}
	want := "> 日本語でまとめ直して。\n  あと 2 の件、影響範囲はどこまで？\n"
	if got := stripped(m.chatLines(c, 60)); !strings.HasPrefix(got, want) {
		t.Errorf("lines:\n%s\nwant them to start with:\n%s", got, want)
	}
}

// On a wide terminal an answer is wrapped at chatMaxWidth.
func TestChatAnswerWrapsAtMostWidth(t *testing.T) {
	m := start(t, fixture(), keymap.Default())
	c := &chat{turns: []turn{{text: strings.Repeat("word ", 80)}}}
	ls := m.chatLines(c, 180)
	if len(ls) < 3 {
		t.Fatalf("not wrapped:\n%s", stripped(ls))
	}
	for _, l := range ls {
		if w := ansi.StringWidth(strings.TrimRight(ansi.Strip(l), " ")); w > chatMaxWidth {
			t.Errorf("a line is %d cells wide: %q", w, ansi.Strip(l))
		}
	}
}
