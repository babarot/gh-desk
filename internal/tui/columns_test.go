package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/babarot/gh-desk/internal/model"
	"github.com/babarot/gh-desk/internal/runner"
	"github.com/babarot/gh-desk/internal/tui/keymap"
)

// columnsLine is the line of the list's column titles.
func columnsLine(t *testing.T, m *Model) string {
	t.Helper()
	for l := range strings.SplitSeq(screen(m), "\n") {
		if strings.Contains(l, "TITLE") {
			return strings.Join(strings.Fields(l), " ")
		}
	}
	t.Fatalf("no column titles:\n%s", screen(m))
	return ""
}

func TestColumnsInTheOrderGiven(t *testing.T) {
	set := testSettings()
	set.Columns = []string{"pr", "age", "title", "review", "ci", "author", "why", "size"}
	m := startSet(t, set, 130)
	if got, want := columnsLine(t, m), "PR AGE TITLE REVIEW CI AUTHOR WHY SIZE"; got != want {
		t.Errorf("columns = %q, want %q", got, want)
	}
	set.Columns = []string{"pr", "title", "age"}
	m = startSet(t, set, 130)
	if got, want := columnsLine(t, m), "PR TITLE AGE"; got != want {
		t.Errorf("columns = %q, want %q", got, want)
	}
}

func TestNarrowTerminalLeavesOutColumnsInOrder(t *testing.T) {
	// A grouped tab, all the columns: SIZE goes below 127 and AUTHOR below
	// 114 (120 and 107 before MERGE, 110 for AUTHOR when the widths were
	// fixed).
	for _, tt := range []struct {
		width int
		want  string
	}{
		{127, "PR TITLE AUTHOR CI MERGE REVIEW WHY SIZE AGE"},
		{126, "PR TITLE AUTHOR CI MERGE REVIEW WHY AGE"},
		{114, "PR TITLE AUTHOR CI MERGE REVIEW WHY AGE"},
		{113, "PR TITLE CI MERGE REVIEW WHY AGE"},
	} {
		m := startSet(t, testSettings(), tt.width)
		if got := columnsLine(t, m); got != tt.want {
			t.Errorf("width %d: columns = %q, want %q", tt.width, got, tt.want)
		}
	}
	// Past AUTHOR, the columns go in the order of dropFirst, each while
	// TITLE would be narrower than minTitle.
	for _, tt := range []struct {
		width int
		want  string
	}{
		{98, "PR TITLE CI MERGE REVIEW WHY AGE"},
		{97, "PR TITLE CI REVIEW WHY AGE"},
		{90, "PR TITLE CI REVIEW AGE"},
		{78, "PR TITLE CI REVIEW"},
		{72, "PR TITLE CI"},
		{63, "PR TITLE"},
	} {
		m := startSet(t, testSettings(), tt.width)
		if got := columnsLine(t, m); got != tt.want {
			t.Errorf("width %d: columns = %q, want %q", tt.width, got, tt.want)
		}
	}
	// A tab with no WHY has room for SIZE for longer.
	m := startSet(t, testSettings(), 117)
	press(m, "3")
	if got, want := columnsLine(t, m), "PR TITLE AUTHOR CI MERGE REVIEW SIZE AGE"; got != want {
		t.Errorf("involved at 117: columns = %q, want %q", got, want)
	}
}

func TestProgressShowsWhateverTheColumns(t *testing.T) {
	for _, cols := range [][]string{
		{"pr", "title", "author", "ci", "review", "why", "size", "age"},
		{"pr", "title", "age"},
		{"pr", "title"},
		{"pr", "size", "title", "why"},
	} {
		set := testSettings()
		set.Columns = cols
		m := startSet(t, set, 130)
		m.progress[key("acme", "api", 12)] = progress{text: "waiting for checks", result: runner.Running}
		line := ""
		for l := range strings.SplitSeq(screen(m), "\n") {
			// The first: the pane under the list names it too.
			if strings.Contains(l, "api#12") {
				line = l
				break
			}
		}
		if !strings.Contains(line, "waiting for checks") {
			t.Errorf("columns %v: the progress is not on the row:\n%q", cols, line)
		}
		if w := ansi.StringWidth(strings.TrimRight(line, " ")); w > 130 {
			t.Errorf("columns %v: the row is %d wide", cols, w)
		}
	}
}

// MERGE marks a pull request that conflicts with its base or is behind it,
// as the heavy fields read it, and says nothing of one that would merge.
func TestMergeColumnMarksConflictAndBehind(t *testing.T) {
	for _, tt := range []struct {
		state model.MergeState
		want  string
	}{
		{model.MergeDirty, "✓   ✗      ○"},
		{model.MergeBehind, "✓   ↓      ○"},
		{model.MergeClean, "✓          ○"},
	} {
		f := fixture()
		f.searches["review"][0].Merge = tt.state
		m := start(t, f, keymap.Default())
		_, y := lineOf(t, m, "Add retries")
		if l := strings.Split(screen(m), "\n")[y]; !strings.Contains(l, tt.want) {
			t.Errorf("%s: row %q, want %q in it", tt.state, l, tt.want)
		}
	}
}

// The pane reads what a merge would do of the pull request it shows, and
// reads it again after r.
func TestPaneReadsWhatAMergeWouldDo(t *testing.T) {
	f := fixture()
	k := f.searches["review"][0].Key
	f.info = map[model.PRKey]func(*model.MergeInfo){k: func(mi *model.MergeInfo) { mi.MergeState = model.MergeDirty }}
	m := start(t, f, keymap.Default())
	if s := screen(m); !strings.Contains(s, "merge     conflicts") {
		t.Fatalf("the pane does not say it conflicts:\n%s", s)
	}
	reads := len(f.infoReads)
	f.info = nil
	press(m, "r")
	if len(f.infoReads) == reads {
		t.Fatal("r did not read it again")
	}
	if s := screen(m); !strings.Contains(s, "merge     approve → merge now") {
		t.Errorf("the pane kept what it read before r:\n%s", s)
	}
}

// A read that finds GitHub still working out whether the pull request can
// be merged is read again, so the pane does not keep "unknown" until r.
func TestPaneReadsAgainWhileUnknown(t *testing.T) {
	f := fixture()
	k := f.searches["review"][0].Key
	state := model.MergeUnknown
	f.info = map[model.PRKey]func(*model.MergeInfo){k: func(mi *model.MergeInfo) { mi.MergeState = state }}
	m := start(t, f, keymap.Default())
	if s := screen(m); strings.Contains(s, "merge     conflicts") {
		t.Fatalf("conflicts before GitHub said so:\n%s", s)
	}
	state = model.MergeDirty
	_, cmd := m.Update(infoAgainMsg{k})
	run(m, cmd)
	if s := screen(m); !strings.Contains(s, "merge     conflicts") {
		t.Errorf("the pane was not read again:\n%s", s)
	}
	// Once known, it is not read again.
	reads := len(f.infoReads)
	_, cmd = m.Update(infoAgainMsg{k})
	run(m, cmd)
	if len(f.infoReads) != reads {
		t.Error("read again once known")
	}
}

func TestPRCellKeepsTheNumber(t *testing.T) {
	m := startSet(t, testSettings(), 130)
	long := row{pr: model.PR{Key: key("octo", "a-repository-with-a-long-name", 1373)}}
	got := ansi.Strip(m.prCell(long, 24))
	if want := "octo/a-repository-…#1373"; got != want {
		t.Errorf("long repository: PR = %q, want %q", got, want)
	}
	short := row{pr: model.PR{Key: key("octo", "cli", 7)}}
	if got, want := ansi.Strip(m.prCell(short, 24)), "octo/cli#7"; got != want {
		t.Errorf("short repository: PR = %q, want %q", got, want)
	}
}
