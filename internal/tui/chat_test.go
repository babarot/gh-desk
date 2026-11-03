package tui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/babarot/gh-desk/internal/claude"
	"github.com/babarot/gh-desk/internal/logging"
	"github.com/babarot/gh-desk/internal/model"
	"github.com/babarot/gh-desk/internal/tui/chat"
	"github.com/babarot/gh-desk/internal/tui/keymap"
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
	c := kept(t, "",
		chat.Turn{You: true, Text: "Review this pull request.", At: now},
		chat.Turn{Text: "Fine.", Steps: []claude.Step{{Tool: "Grep"}, {Tool: "Read"}, {Tool: "Grep"}}, At: now.Add(72 * time.Second), Cost: 0.41},
		chat.Turn{You: true, Text: "And the tests?", At: now.Add(5 * time.Minute)},
		chat.Turn{Text: "Fine too.", At: now.Add(5*time.Minute + 3*time.Second)},
	)
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
	c := answering(t, dir, "Why?", now.Add(-11*time.Second),
		claude.Step{Tool: "Grep", Arg: "retry"},
		claude.Step{Tool: "Read", Arg: dir + "/a.go"},
		claude.Step{Tool: "Grep", Arg: "func Do"},
		claude.Step{Tool: "Read", Arg: dir + "/internal/retry/retry.go"},
	)
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
	c := kept(t, "", chat.Turn{You: true, Text: "日本語でまとめ直して。\nあと 2 の件、影響範囲はどこまで？"})
	want := "> 日本語でまとめ直して。\n  あと 2 の件、影響範囲はどこまで？\n"
	if got := stripped(m.chatLines(c, 60)); !strings.HasPrefix(got, want) {
		t.Errorf("lines:\n%s\nwant them to start with:\n%s", got, want)
	}
}

// On a wide terminal an answer is wrapped at chatMaxWidth.
func TestChatAnswerWrapsAtMostWidth(t *testing.T) {
	m := start(t, fixture(), keymap.Default())
	c := kept(t, "", chat.Turn{Text: strings.Repeat("word ", 80)})
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

// The text box stays in its box in the claude pane, as wide as the chat's
// text, when the terminal is made narrower or wider while something is
// typed: what is typed wraps inside the box rather than being cut at its
// edge.
func TestChatInputStaysInItsBox(t *testing.T) {
	m, _ := startWith(t, fixture(), keymap.Default())
	fakeClaudeOf(t, m)
	press(m, "i")
	typed := strings.Repeat("x", 120)
	press(m, strings.Split(typed, "")...)
	for _, w := range []int{60, 180} {
		m.Update(tea.WindowSizeMsg{Width: w, Height: 30})
		shown := 0
		for l := range strings.SplitSeq(screen(m), "\n") {
			if !strings.Contains(l, "xx") {
				continue
			}
			shown += strings.Count(l, "x")
			// The pane's left border and a space come before the box.
			inner, ok := strings.CutPrefix(l, "│ ")
			bw := min(w-4, chatMaxWidth)
			if row := ansi.Truncate(inner, bw, ""); !ok || !strings.HasPrefix(row, "│") || !strings.HasSuffix(row, "│") || ansi.StringWidth(row) != bw {
				t.Errorf("at %d a typed row is %q", w, l)
			}
		}
		if shown != len(typed) {
			t.Errorf("at %d %d of %d typed characters are shown", w, shown, len(typed))
		}
	}
}

// testKey is the pull request the conversations built here are about.
var testKey = model.PRKey{Owner: "acme", Repo: "api", Number: 12}

// noStatus drops what a conversation says went wrong.
type noStatus struct{}

func (noStatus) Fail(string)   {}
func (noStatus) Refuse(string) {}

// kept is a conversation as read from what is kept: its worktree and
// its turns.
func kept(t *testing.T, dir string, turns ...chat.Turn) *chatScreen {
	t.Helper()
	store := newFakeChats()
	store.recs[testKey] = chat.Record{Key: testKey, Dir: dir, Rev: 1, Turns: turns}
	s := chat.NewState(store, logging.Discard(), func() time.Time { return now }, noStatus{})
	c := s.New(model.PR{Key: testKey})
	s.Sync(c)
	if len(c.Turns()) != len(turns) {
		t.Fatalf("read %d turns of %d", len(c.Turns()), len(turns))
	}
	return &chatScreen{Conversation: c}
}

// answering is a conversation whose question was asked at since in the
// worktree dir, with what Claude did for it so far.
func answering(t *testing.T, dir, question string, since time.Time, steps ...claude.Step) *chatScreen {
	t.Helper()
	s := chat.NewState(nil, logging.Discard(), func() time.Time { return since }, noStatus{})
	c := s.New(model.PR{Key: testKey})
	c.Prepared(dir, "")
	if _, ok := s.Begin(c, question); !ok {
		t.Fatal("the question was not asked")
	}
	for _, st := range steps {
		c.AddStep(st)
	}
	return &chatScreen{Conversation: c}
}
