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
// what gh-desk draws from the same pull requests, for scripts/ui.mjs
// to compare the page's drawing with (mise run config-ui:test).
const configuiGolden = "../config/ui/testdata/list.golden"

type configuiPR struct {
	Repo         string   `json:"repo"`
	N            int      `json:"n"`
	Author       string   `json:"author"`
	Bot          bool     `json:"bot"`
	Title        string   `json:"title"`
	Add          int      `json:"add"`
	Del          int      `json:"del"`
	Files        int      `json:"files"`
	Draft        bool     `json:"draft"`
	Checks       string   `json:"checks"`
	Decision     string   `json:"decision"`
	Requested    []string `json:"requested"`
	UpdatedHours int      `json:"updatedHours"`
}

func configuiFixture(t *testing.T) *fakeSource {
	t.Helper()
	b, err := os.ReadFile("../config/ui/web/prs.json")
	if err != nil {
		t.Fatal(err)
	}
	var ps []configuiPR
	if err := json.Unmarshal(b, &ps); err != nil {
		t.Fatal(err)
	}
	var prs []model.PR
	for i, p := range ps {
		owner, repo, _ := strings.Cut(p.Repo, "/")
		pr := model.PR{ID: p.Repo + string(rune('a'+i)), Key: key(owner, repo, p.N), HeadOID: fmt.Sprintf("head%d", i), Title: p.Title, Author: p.Author, AuthorIsBot: p.Bot, State: model.StateOpen,
			Draft: p.Draft, Additions: p.Add, Deletions: p.Del, ChangedFiles: p.Files, Updated: now.Add(-time.Duration(p.UpdatedHours) * time.Hour),
			Checks:   map[string]model.CheckState{"passing": model.CheckSuccess, "failing": model.CheckFailure, "pending": model.CheckPending}[p.Checks],
			Decision: map[string]model.ReviewDecision{"approved": model.DecisionApproved, "changes_requested": model.DecisionChangesRequested, "review_required": model.DecisionReviewRequired}[p.Decision]}
		for _, r := range p.Requested {
			if team, ok := strings.CutPrefix(r, "@"); ok {
				pr.Requested = append(pr.Requested, model.Reviewer{Login: team, Team: true})
			} else {
				pr.Requested = append(pr.Requested, model.Reviewer{Login: r})
			}
		}
		prs = append(prs, pr)
	}
	return &fakeSource{searches: map[string][]model.PR{"review": prs}}
}

func configuiModel(t *testing.T) *Model {
	t.Helper()
	set := testSettings()
	set.Tabs = set.Tabs[:1]
	set.Tabs = append(set.Tabs, Tab{Name: "inbox", Inbox: true, Limit: 100})
	set.Tabs[0].Folded, set.HomeOwners = nil, nil
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
	}{{"menu", []string{"enter"}}, {"compose", []string{"R"}}, {"merge", []string{"space", "M"}}} {
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
