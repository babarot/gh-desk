package app

import (
	"context"
	"os"
	"testing"

	"github.com/babarot/gh-triage/internal/claude"
	"github.com/babarot/gh-triage/internal/github"
	"github.com/babarot/gh-triage/internal/logging"
	"github.com/babarot/gh-triage/internal/model"
)

// TestLiveClaude reviews a public pull request with Claude Code, as C
// does: it reads the pull request from GitHub (only reads), makes its
// worktree in a temporary cache, asks for a review, asks more in the same
// session, and drafts a review. It costs a few Claude calls. Run it with
// GH_TRIAGE_LIVE_CLAUDE=owner/repo#n, and GH_TRIAGE_LIVE_MODEL to pick the
// model (sonnet by default).
func TestLiveClaude(t *testing.T) {
	target := os.Getenv("GH_TRIAGE_LIVE_CLAUDE")
	if target == "" {
		t.Skip("set GH_TRIAGE_LIVE_CLAUDE=owner/repo#n to ask Claude about a pull request")
	}
	key, err := model.ParsePRKey(target)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	gh, err := github.New(logging.Discard(), github.Options{})
	if err != nil {
		t.Fatal(err)
	}
	ps, err := gh.PullRequests(ctx, []model.PRKey{key})
	if err != nil || len(ps) != 1 {
		t.Fatalf("read %s: %v", key, err)
	}
	p := ps[key]
	d, err := gh.Detail(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	diff, err := gh.Diff(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	ws := &claude.Workspace{Dir: t.TempDir(), Borrow: claude.GhqRoots(ctx), Log: logging.Discard()}
	dir, err := ws.Prepare(ctx, key, p.HeadOID)
	if err != nil {
		t.Fatal(err)
	}
	model := os.Getenv("GH_TRIAGE_LIVE_MODEL")
	if model == "" {
		model = "sonnet"
	}
	c := claude.New(claude.Options{Model: model, Tools: []string{"Read", "Grep", "Glob"}, Log: logging.Discard()})
	session := claude.NewSession()
	steps := func(s claude.Step) { t.Logf("  ⏺ %s %s", s.Tool, s.Arg) }
	a, err := c.Ask(ctx, claude.Request{Dir: dir, Session: session, First: true, Prompt: claude.PRPrompt(p, d, diff) + "\n" + claude.ReviewQuestion}, steps)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("review (%s, $%.2f):\n%s", a.Model, a.Cost, a.Text)
	a, err = c.Ask(ctx, claude.Request{Dir: dir, Session: session, Prompt: "In one sentence: what is the riskiest part?"}, steps)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("more: %s", a.Text)
	a, err = c.Ask(ctx, claude.Request{Dir: dir, Session: session, Prompt: claude.DraftPrompt, Schema: claude.DraftSchema}, steps)
	if err != nil {
		t.Fatal(err)
	}
	dr, err := claude.ParseDraft(a)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("draft %s:\n%s", dr.Event, dr.Body)
}
