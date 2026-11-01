package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/babarot/gh-triage/internal/runner"
)

// columnsLine is the line of the list's column titles.
func columnsLine(t *testing.T, m *Model) string {
	t.Helper()
	for _, l := range strings.Split(screen(m), "\n") {
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

func TestNarrowTerminalLeavesOutSizeThenAuthor(t *testing.T) {
	// A grouped tab, all the columns: as before, SIZE goes below 120 and
	// AUTHOR below 107 (110 before, when the widths were fixed).
	for _, tt := range []struct {
		width int
		want  string
	}{
		{120, "PR TITLE AUTHOR CI REVIEW WHY SIZE AGE"},
		{119, "PR TITLE AUTHOR CI REVIEW WHY AGE"},
		{107, "PR TITLE AUTHOR CI REVIEW WHY AGE"},
		{106, "PR TITLE CI REVIEW WHY AGE"},
	} {
		m := startSet(t, testSettings(), tt.width)
		if got := columnsLine(t, m); got != tt.want {
			t.Errorf("width %d: columns = %q, want %q", tt.width, got, tt.want)
		}
	}
	// A tab with no WHY has room for SIZE for longer.
	m := startSet(t, testSettings(), 110)
	press(m, "3")
	if got, want := columnsLine(t, m), "PR TITLE AUTHOR CI REVIEW SIZE AGE"; got != want {
		t.Errorf("involved at 110: columns = %q, want %q", got, want)
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
		for _, l := range strings.Split(screen(m), "\n") {
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
