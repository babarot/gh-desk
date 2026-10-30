package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/babarot/gh-triage/internal/keymap"
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
		"│ quoted",
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
	press(m, "v")
	if s := screen(m); !strings.Contains(s, "▾ description") || !strings.Contains(s, "│ status    open") {
		t.Fatalf("no parts or facts:\n%s", s)
	}
	// ] goes to the checks; z folds the part the pane starts in.
	press(m, "]")
	if !strings.Contains(screen(m), "│ ▾ checks 1") || strings.Contains(screen(m), "▾ description") {
		t.Fatalf("] did not go to the checks:\n%s", screen(m))
	}
	press(m, "z")
	if !strings.Contains(screen(m), "▸ checks 1") || strings.Contains(screen(m), "✓ test") {
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
	press(m, "v")
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
