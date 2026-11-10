package tui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/babarot/gh-desk/internal/model"
)

// the settings page draws the list again in JavaScript. This writes
// what gh-desk draws from the same pull requests, for check.mjs
// to compare the page's drawing with (mise run config-ui:test).
const configuiGolden = "../config/ui/testdata/list.golden"

func configuiFixture(t *testing.T) *fakeSource {
	t.Helper()
	b, err := os.ReadFile("../config/ui/web/prs.json")
	if err != nil {
		t.Fatal(err)
	}
	var prs []model.PR
	if err := json.Unmarshal(b, &prs); err != nil {
		t.Fatal(err)
	}
	// As if read now; ui.SampleTime is the time the file is as of.
	shift := now.Sub(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	for i := range prs {
		prs[i].Updated, prs[i].Created = prs[i].Updated.Add(shift), prs[i].Created.Add(shift)
	}
	return &fakeSource{searches: map[string][]model.PR{"review": prs}}
}

func configuiModel(t *testing.T) *Model {
	t.Helper()
	set := testSettings()
	set.Tabs = set.Tabs[:1]
	set.Tabs = append(set.Tabs, Tab{Name: "inbox", Inbox: true, Limit: 100})
	set.Tabs[0].Sections, set.HomeOwners = reviewSections(), nil
	m := startSource(t, configuiFixture(t), set)
	m.Update(tea.WindowSizeMsg{Width: 132, Height: 34})
	return m
}

// popupOf is the first frame on the screen, its text as drawn.
func popupOf(screen string) string {
	lines := strings.Split(screen, "\n")[1:]
	var out []string
	x, w := -1, 0
	for _, l := range lines {
		r := []rune(l)
		if x < 0 {
			i := strings.IndexRune(l, '╭')
			if i < 0 {
				continue
			}
			x = len([]rune(l[:i]))
			w = strings.IndexRune(string(r[x:]), '╮')
			w = len([]rune(string(r[x:])[:w])) + 1
		}
		s := string(r[x:min(x+w, len(r))])
		out = append(out, strings.TrimRight(s, " "))
		if r[x] == '╰' {
			break
		}
	}
	return strings.Join(out, "\n")
}

func TestConfigUIPopups(t *testing.T) {
	var b strings.Builder
	for _, s := range []struct {
		name string
		keys []string
	}{{"merge", []string{"space", "M"}}} {
		m := configuiModel(t)
		press(m, s.keys...)
		fmt.Fprintf(&b, "=== %s\n%s\n", s.name, popupOf(screen(m)))
	}
	const path = "../config/ui/testdata/scenes.golden"
	if *update {
		if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != b.String() {
		t.Errorf("%s is not what gh-desk draws; run go test ./internal/tui -run TestConfigUIPopups -update:\n%s", path, b.String())
	}
}

func TestConfigUIList(t *testing.T) {
	m := configuiModel(t)
	lines := strings.Split(screen(m), "\n")
	var got []string
	for _, l := range lines[1:] {
		l = strings.TrimRight(l, " ")
		if l == "" {
			break
		}
		got = append(got, l)
	}
	want := strings.Join(got, "\n") + "\n"
	if *update {
		if err := os.MkdirAll(filepath.Dir(configuiGolden), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(configuiGolden, []byte(want), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	b, err := os.ReadFile(configuiGolden)
	if err != nil || string(b) != want {
		t.Errorf("%s is not what gh-desk draws; run go test ./internal/tui -run TestConfigUIList -update:\n%s", configuiGolden, want)
	}
}
