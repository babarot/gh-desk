package playground

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/babarot/gh-desk/internal/claude"
	"github.com/babarot/gh-desk/internal/diff"
	"github.com/babarot/gh-desk/internal/logging"
	"github.com/babarot/gh-desk/internal/model"
	"github.com/babarot/gh-desk/internal/plan"
	"github.com/babarot/gh-desk/internal/runner"
	"github.com/babarot/gh-desk/internal/tui"
)

// The site hands the world to the TUI and the runner, so a method added to
// one of their interfaces fails this build, not only the site's.
var (
	_ tui.Source    = Source{}
	_ tui.Claude    = Claude{}
	_ runner.GitHub = Runner{}
)

// clock is a time that moves only when told.
type clock struct{ t time.Time }

func (c *clock) now() time.Time { return c.t }

func newWorld() (*World, *clock) {
	c := &clock{time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC)}
	w := New(c.now)
	w.Instant()
	return w, c
}

// TestDiffs checks that every diff reads as the files gh-desk shows.
func TestDiffs(t *testing.T) {
	w, _ := newWorld()
	for _, p := range w.prs {
		files := diff.Parse([]byte(p.diff))
		if len(files) == 0 || len(files) != len(p.files) {
			t.Errorf("%s: %d files read, %d written", p.Key, len(files), len(p.files))
			continue
		}
		for i, f := range files {
			if f.Path() != p.files[i] {
				t.Errorf("%s: file %d is %q, want %q", p.Key, i, f.Path(), p.files[i])
			}
			a, d := f.Stat()
			if a+d == 0 {
				t.Errorf("%s: %s changes no line", p.Key, f.Path())
			}
		}
	}
}

// TestTabs checks that each tab of the config file written on first run
// finds pull requests, and the inbox has notifications.
func TestTabs(t *testing.T) {
	w, _ := newWorld()
	s := w.Source()
	ctx := context.Background()
	for _, q := range []string{
		"is:open is:pr review-requested:@me archived:false",
		"is:open is:pr author:@me archived:false",
		"is:open is:pr involves:@me -author:@me -review-requested:@me archived:false",
	} {
		prs, total, _, _, err := s.SearchPage(ctx, q, 100, "")
		if err != nil || total == 0 || len(prs) != total {
			t.Errorf("%q: %d of %d, %v", q, len(prs), total, err)
		}
	}
	ns, _ := s.Notifications(ctx, 100)
	if len(ns) == 0 {
		t.Error("the inbox is empty")
	}
}

// TestChecksFinish checks that a check still running at the start passes
// once it has run long enough.
func TestChecksFinish(t *testing.T) {
	w, c := newWorld()
	k := model.PRKey{Owner: "acme", Repo: "web", Number: 1290}
	got, _ := w.Source().PullRequests(context.Background(), []model.PRKey{k})
	if got[k].Checks != model.CheckPending {
		t.Fatalf("checks at the start: %s", got[k].Checks)
	}
	c.t = c.t.Add(time.Minute)
	got, _ = w.Source().PullRequests(context.Background(), []model.PRKey{k})
	if got[k].Checks != model.CheckSuccess {
		t.Errorf("checks a minute later: %s", got[k].Checks)
	}
}

// TestMergeBots runs the real runner over the bots' pull requests: each
// is approved and merged.
func TestMergeBots(t *testing.T) {
	w, _ := newWorld()
	rn := runner.New(w.Runner(), logging.Discard(), runner.Options{Viewer: Viewer, CanWait: true,
		Settings: plan.Settings{AutoMerge: true, Methods: []model.MergeMethod{model.MethodMerge}},
		PollFast: time.Millisecond, PollSlow: time.Millisecond, Recheck: time.Millisecond, RecheckFor: time.Millisecond, Quiet: time.Millisecond})
	var items []runner.Item
	for _, p := range w.prs {
		if p.AuthorIsBot {
			items = append(items, runner.Item{PR: p.row(), Head: p.HeadOID, AutoMerge: true})
		}
	}
	_, events := rn.Start(context.Background(), runner.Merge, items)
	merged := 0
	for ev := range events {
		switch ev.Result {
		case runner.Merged:
			merged++
		case runner.Failed, runner.Skipped, runner.Stopped:
			t.Errorf("%s: %s", ev.Key, ev.Text)
		}
	}
	rn.Wait()
	if merged != len(items) {
		t.Errorf("%d of %d merged", merged, len(items))
	}
}

// TestTriage checks the plan for the request the page suggests.
func TestTriage(t *testing.T) {
	w, _ := newWorld()
	ctx := context.Background()
	prs, _, _, _, _ := w.Source().SearchPage(ctx, "is:open is:pr review-requested:@me", 100, "")
	var rows []claude.SnapshotRow
	for _, p := range prs {
		full, _ := w.Source().PullRequests(ctx, []model.PRKey{p.Key})
		// The page's tab is split as the default review tab is; the plan
		// reads only the bots' section.
		section := "requested from you"
		if full[p.Key].AuthorIsBot {
			section = "bots"
		}
		rows = append(rows, claude.SnapshotRow{PR: full[p.Key], Section: section})
	}
	prompt := claude.TriagePrompt(claude.Snapshot{Now: w.start, Tab: "review", Rows: rows,
		Sections: []string{"requested from you", "requested from a team", "other", "bots"}, Pane: "off"}, "merge the bots with passing checks")
	a, err := w.Claude().Triage(ctx, claude.Request{Prompt: prompt, Schema: claude.TriageSchema})
	if err != nil {
		t.Fatal(err)
	}
	p, err := claude.ParseTriage(a)
	if err != nil {
		t.Fatal(err)
	}
	last := p.Actions[len(p.Actions)-1]
	if last.Do != claude.DoMerge || len(last.Keys) != 5 {
		t.Errorf("plan ends with %s of %v", last.Do, last.Keys)
	}
	if !slices.ContainsFunc(p.Actions, func(a claude.Action) bool { return a.Do == claude.DoSelect }) {
		t.Error("the plan selects nothing")
	}
	if !strings.Contains(p.Reply, "5 bot") {
		t.Errorf("reply: %s", p.Reply)
	}
}
