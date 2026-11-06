package tui

import (
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/babarot/gh-desk/internal/model"
	"github.com/babarot/gh-desk/internal/plan"
	"github.com/babarot/gh-desk/internal/runner"
	"github.com/babarot/gh-desk/internal/tui/keymap"
)

// bump is a bot's dependency update whose checks passed.
func bump(id string, n int) model.PR {
	return model.PR{ID: id, Key: key("acme", "api", n), Title: "chore(deps): update octo/app to v1.2." + id, Author: "renovate", AuthorIsBot: true,
		State: model.StateOpen, HeadOID: "head-" + id, Checks: model.CheckSuccess, Updated: now.Add(-time.Hour)}
}

var renovatePatches = AutoApprove{Rule: plan.Rule{Name: "renovate patches", Authors: []string{"renovate"}, Title: regexp.MustCompile(`^chore\(deps\)`)}, Query: "auto"}

// startAuto starts the TUI with rules that approve, the search "auto"
// finding prs; edit changes the settings first.
func startAuto(t *testing.T, prs []model.PR, edit func(*Settings), rules ...AutoApprove) (*Model, *fakeRunner, *fakeSource) {
	t.Helper()
	f := fixture()
	f.searches["auto"] = prs
	set := testSettings()
	set.AutoApprove = rules
	if edit != nil {
		edit(&set)
	}
	r := &fakeRunner{results: map[model.PRKey]runner.Event{}}
	m := New(f, r, &fakeClaude{}, keymap.Default(), nil, set)
	m.Update(tea.WindowSizeMsg{Width: 130, Height: 34})
	run(m, m.Init())
	return m, r, f
}

// approvedKeys are the pull requests the batches of rules were given.
func approvedKeys(r *fakeRunner) []string {
	var out []string
	for _, items := range r.items {
		for _, it := range items {
			out = append(out, it.PR.Key.String())
		}
	}
	return out
}

func autoNoteTitles(m *Model) string {
	var out []string
	for _, n := range m.notes {
		out = append(out, n.title)
	}
	return strings.Join(out, "\n")
}

// A rule approves what it matches, on the head it judged, as a rule's
// approval that waits for the jobs, and tells of it.
func TestAutoApproveApprovesWhatTheRuleMatches(t *testing.T) {
	person := bump("p", 40)
	person.Author, person.AuthorIsBot = "alice", false
	m, r, _ := startAuto(t, []model.PR{bump("a", 41), person}, nil, renovatePatches)
	if got := approvedKeys(r); len(got) != 1 || got[0] != "acme/api#41" {
		t.Fatalf("approved %v; want acme/api#41 only", got)
	}
	it := r.items[0][0]
	if !it.Auto || !it.Wait || it.IncludeFailing || it.Head != "head-a" || r.reviews[0].Event != model.EventApprove {
		t.Errorf("item = %+v, review %+v", it, r.reviews[0])
	}
	has(t, autoNoteTitles(m), "auto-approve done: approved 1")
	if n := m.notes[0]; len(n.items) != 1 || !strings.Contains(n.items[0].text, "renovate patches") {
		t.Errorf("note items = %+v; want the rule named", n.items)
	}
}

// A head is tried once a session; a new head is judged again.
func TestAutoApproveTriesAHeadOnce(t *testing.T) {
	m, r, f := startAuto(t, []model.PR{bump("a", 41)}, nil, renovatePatches)
	run(m, m.autoApprove())
	if len(r.items) != 1 {
		t.Fatalf("batches = %d; want the head tried once", len(r.items))
	}
	p := f.searches["auto"][0]
	p.HeadOID = "head-a2"
	f.searches["auto"][0] = p
	run(m, m.autoApprove())
	if len(r.items) != 2 || r.items[1][0].Head != "head-a2" {
		t.Errorf("items = %+v; want the new head approved", r.items)
	}
}

// What a rule matches is not approved when an exclusion matches it, it is
// a draft, or the head moved since the search. A job that fails is the
// runner's to stop, as it waits.
func TestAutoApprovePassesOver(t *testing.T) {
	held := bump("h", 42)
	held.Labels = []string{"hold"}
	draft := bump("d", 43)
	draft.Draft = true
	moved := bump("m", 44)
	_, r, _ := startAuto(t, []model.PR{held, draft, moved}, func(s *Settings) {
		s.Exclude = append(s.Exclude, plan.Rule{Name: "held", Labels: []string{"hold"}})
	}, renovatePatches)
	if got := approvedKeys(r); len(got) != 1 || got[0] != "acme/api#44" {
		t.Fatalf("approved %v; want only acme/api#44", got)
	}

	f := fixture()
	f.searches["auto"] = []model.PR{moved}
	f.info = map[model.PRKey]func(*model.MergeInfo){moved.Key: func(mi *model.MergeInfo) { mi.HeadOID = "pushed" }}
	r2 := &fakeRunner{results: map[model.PRKey]runner.Event{}}
	set := testSettings()
	set.AutoApprove = []AutoApprove{renovatePatches}
	m2 := New(f, r2, &fakeClaude{}, keymap.Default(), nil, set)
	run(m2, m2.Init())
	if len(r2.items) != 0 {
		t.Errorf("approved %v after the head moved", approvedKeys(r2))
	}
}

// A pull request the user has in a confirm popup is left to it.
func TestAutoApproveLeavesThePopupsPullRequests(t *testing.T) {
	m, r, f := startAuto(t, nil, nil, renovatePatches)
	p := bump("a", 41)
	f.searches["auto"] = []model.PR{p}
	m.bulk = &bulk{kind: runner.Review, items: []bulkItem{{pr: p}}}
	run(m, m.autoApprove())
	if len(r.items) != 0 {
		t.Errorf("approved %v while it is in the popup", approvedKeys(r))
	}
}

// A dry run approves nothing and tells what it would approve, once.
func TestAutoApproveDryRun(t *testing.T) {
	dry := renovatePatches
	dry.DryRun = true
	m, r, _ := startAuto(t, []model.PR{bump("a", 41)}, nil, dry)
	run(m, m.autoApprove())
	if len(r.items) != 0 {
		t.Fatalf("a dry run approved %v", approvedKeys(r))
	}
	if titles := autoNoteTitles(m); strings.Count(titles, "auto-approve (dry run): would approve 1") != 1 {
		t.Errorf("notifications:\n%s", titles)
	}
}

// A gh-desk without the record of pending merges approves nothing, and
// says so once.
func TestAutoApproveOffWithoutTheLock(t *testing.T) {
	m, r, _ := startAuto(t, []model.PR{bump("a", 41)}, func(s *Settings) { s.CanWait = false }, renovatePatches)
	run(m, m.autoApprove())
	if len(r.items) != 0 {
		t.Fatalf("approved %v without the lock", approvedKeys(r))
	}
	if titles := autoNoteTitles(m); strings.Count(titles, "auto-approve is off in this gh-desk") != 1 {
		t.Errorf("notifications:\n%s", titles)
	}
}

// Every page of the search is read: what the first pages pass over does
// not keep a later one from being approved.
func TestAutoApproveReadsTheLaterPages(t *testing.T) {
	var prs []model.PR
	for i := range 4 {
		p := bump(fmt.Sprint(i), 50+i)
		p.Author = "alice"
		prs = append(prs, p)
	}
	prs = append(prs, bump("last", 60))
	_, r, _ := startAuto(t, prs, nil, renovatePatches)
	if got := approvedKeys(r); len(got) != 1 || got[0] != "acme/api#60" {
		t.Errorf("approved %v; want acme/api#60 from the last page", got)
	}
}

// A search that finds more than is read is told once, and asked to be
// narrowed.
func TestAutoApproveTellsOfTooManyOnce(t *testing.T) {
	var prs []model.PR
	for i := range autoMax + 1 {
		p := bump(fmt.Sprint(i), 100+i)
		p.Author = "alice"
		prs = append(prs, p)
	}
	m, _, _ := startAuto(t, prs, nil, renovatePatches)
	run(m, m.autoApprove())
	if titles := autoNoteTitles(m); strings.Count(titles, "narrow its query") != 1 {
		t.Errorf("notifications:\n%s", titles)
	}
}
