package mcpserver

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/babarot/gh-desk/internal/logging"
	"github.com/babarot/gh-desk/internal/model"
)

// fakeGitHub answers as GitHub would for babarot/gh-desk, and keeps the
// repositories it was asked about.
type fakeGitHub struct {
	merge  string
	hits   []model.IssueHit
	issue  model.IssueDetail
	run    model.CheckRunDetail
	log    []byte
	hasLog bool
	asked  []string
	query  string
}

func (f *fakeGitHub) note(owner, repo string) { f.asked = append(f.asked, owner+"/"+repo) }

func (f *fakeGitHub) PRCommits(_ context.Context, k model.PRKey) (string, string, error) {
	f.note(k.Owner, k.Repo)
	return "head1", f.merge, nil
}

func (f *fakeGitHub) CommitChecks(_ context.Context, owner, repo, label, expr string, pr int) (model.CommitChecks, error) {
	f.note(owner, repo)
	return model.CommitChecks{Label: label, OID: expr, Checks: []model.RepoCheck{
		{ID: 126664958582, Name: "test", Workflow: "CI", App: "github-actions", State: model.CheckFailure, Required: pr > 0},
	}}, nil
}

func (f *fakeGitHub) CheckRun(_ context.Context, owner, repo string, id int64) (model.CheckRunDetail, error) {
	f.note(owner, repo)
	return f.run, nil
}

func (f *fakeGitHub) JobLog(_ context.Context, owner, repo string, id int64) ([]byte, bool, error) {
	f.note(owner, repo)
	return f.log, f.hasLog, nil
}

func (f *fakeGitHub) SearchIssues(_ context.Context, q string, first int) ([]model.IssueHit, error) {
	f.query = q
	return f.hits, nil
}

func (f *fakeGitHub) Issue(_ context.Context, owner, repo string, number int) (model.IssueDetail, error) {
	f.note(owner, repo)
	return f.issue, nil
}

var pr = model.PRKey{Owner: "babarot", Repo: "gh-desk", Number: 47}

// call runs tool with args through a client connected to the server, as
// Claude Code would.
func call(t *testing.T, s *Server, tool string, args map[string]any) (string, bool) {
	t.Helper()
	ctx := context.Background()
	ct, st := mcp.NewInMemoryTransports()
	ss, err := s.MCP().Connect(ctx, st, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ss.Close()
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "test"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()
	res, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: tool, Arguments: args})
	if err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			b.WriteString(tc.Text)
		}
	}
	return b.String(), res.IsError
}

func onlyOurs(t *testing.T, f *fakeGitHub) {
	t.Helper()
	for _, r := range f.asked {
		if r != "babarot/gh-desk" {
			t.Errorf("asked about %s", r)
		}
	}
}

func TestToolsAreListed(t *testing.T) {
	ctx := context.Background()
	ct, st := mcp.NewInMemoryTransports()
	s := New(&fakeGitHub{}, pr, logging.Discard())
	if _, err := s.MCP().Connect(ctx, st, nil); err != nil {
		t.Fatal(err)
	}
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "test"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	res, err := cs.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, tl := range res.Tools {
		got = append(got, tl.Name)
	}
	if strings.Join(got, ",") != "check_run_log,checks,read_issue,search_issues" {
		t.Errorf("tools = %v; Tools = %v", got, Tools)
	}
}

func TestChecksTakeTheMergeCommitOnceMerged(t *testing.T) {
	f := &fakeGitHub{merge: "merge1"}
	text, isErr := call(t, New(f, pr, logging.Discard()), "checks", nil)
	if isErr {
		t.Fatal(text)
	}
	for _, want := range []string{`commit="head1" label="head"`, `commit="merge1" label="merge commit"`, `check run id=126664958582 "test": FAILURE, workflow "CI", app github-actions, required`} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q in\n%s", want, text)
		}
	}
	onlyOurs(t, f)

	f = &fakeGitHub{}
	text, _ = call(t, New(f, pr, logging.Discard()), "checks", map[string]any{"ref": "main"})
	if !strings.Contains(text, `commit="main" label="main"`) || strings.Contains(text, "required") {
		t.Errorf("checks of main:\n%s", text)
	}
	onlyOurs(t, f)
}

func TestCheckRunLog(t *testing.T) {
	f := &fakeGitHub{run: model.CheckRunDetail{ID: 9, Name: "test", App: "github-actions", Conclusion: "failure", Summary: "1 failed"},
		log: []byte("2026-11-05T05:00:00.1234567Z \x1b[36mstarting\x1b[0m\n2026-11-05T05:00:01.0Z ##[error]Process completed with exit code 1.\n"), hasLog: true}
	text, isErr := call(t, New(f, pr, logging.Discard()), "check_run_log", map[string]any{"id": 9})
	if isErr || !strings.Contains(text, "<log>\nstarting\n##[error]Process completed with exit code 1.\n</log>") || !strings.Contains(text, "1 failed") {
		t.Errorf("text:\n%s", text)
	}
	onlyOurs(t, f)

	f.hasLog, f.log = false, nil
	if text, _ := call(t, New(f, pr, logging.Discard()), "check_run_log", map[string]any{"id": 9}); !strings.Contains(text, "has no log") {
		t.Errorf("no log:\n%s", text)
	}

	f.run.App = "atlantis"
	f.asked = nil
	text, _ = call(t, New(f, pr, logging.Discard()), "check_run_log", map[string]any{"id": 9})
	if !strings.Contains(text, "not a GitHub Actions job") || len(f.asked) != 1 {
		t.Errorf("an app's check run read %v:\n%s", f.asked, text)
	}
}

func TestTrimLogKeepsErrorsAndTheEnd(t *testing.T) {
	var b strings.Builder
	for i := range 1000 {
		if i == 100 {
			b.WriteString("##[error]boom\n")
			continue
		}
		fmt.Fprintf(&b, "line %d\n", i)
	}
	got := trimLog(b.String())
	for _, want := range []string{"line 95\n", "##[error]boom\n", "line 105\n", "line 999\n", "[…]\n"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q", want)
		}
	}
	for _, unwanted := range []string{"line 10\n", "line 500\n"} {
		if strings.Contains(got, unwanted) {
			t.Errorf("kept %q", unwanted)
		}
	}
}

func TestSearchQueryTakesNoQualifierFromClaude(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   searchIn
		want string
		err  bool
	}{
		{"all", searchIn{Kind: "pr", State: "merged", Author: "app/renovate", Labels: []string{"bug fix"}, Text: "parser"},
			`repo:babarot/gh-desk is:pr is:merged author:app/renovate label:"bug fix" "parser"`, false},
		{"a bot", searchIn{Author: "renovate[bot]"}, `repo:babarot/gh-desk author:renovate[bot]`, false},
		{"repo in the text", searchIn{Text: `x" repo:other/private "y`}, `repo:babarot/gh-desk "x repo:other/private y"`, false},
		{"a trailing backslash", searchIn{Text: `x\`, State: "open"}, `repo:babarot/gh-desk is:open "x"`, false},
		{"a line break", searchIn{Text: "a\nrepo:other/x"}, `repo:babarot/gh-desk "a repo:other/x"`, false},
		{"repo in a label", searchIn{Labels: []string{`bug" repo:other/x`}}, `repo:babarot/gh-desk label:"bug repo:other/x"`, false},
		{"repo in the author", searchIn{Author: "octocat repo:other/private"}, "", true},
		{"org in the author", searchIn{Author: "org:other"}, "", true},
		{"an unknown kind", searchIn{Kind: "repo:other/x"}, "", true},
		{"an unknown state", searchIn{State: "open repo:other/x"}, "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := searchQuery("babarot", "gh-desk", tc.in)
			if (err != nil) != tc.err || got != tc.want {
				t.Errorf("searchQuery = %q, %v; want %q", got, err, tc.want)
			}
			// Whatever the input, the only repo: is the first word.
			if strings.Count(got, "repo:") > 0 && !strings.HasPrefix(got, "repo:babarot/gh-desk") {
				t.Errorf("query %q", got)
			}
		})
	}
}

func TestSearchLeavesOutOtherRepositories(t *testing.T) {
	f := &fakeGitHub{hits: []model.IssueHit{
		{Repo: "babarot/gh-desk", Number: 1, Title: "ours", Updated: time.Now()},
		{Repo: "other/private", Number: 2, Title: "secret plan", Updated: time.Now()},
		{Repo: "Babarot/GH-Desk", Number: 3, Title: "ours in another case", Updated: time.Now()},
	}}
	text, isErr := call(t, New(f, pr, logging.Discard()), "search_issues", map[string]any{"text": "plan"})
	if isErr || !strings.Contains(text, "#1 ") || !strings.Contains(text, "#3 ") || strings.Contains(text, "secret") {
		t.Errorf("text:\n%s", text)
	}
	if f.query != `repo:babarot/gh-desk "plan"` {
		t.Errorf("query = %q", f.query)
	}
	if text, isErr := call(t, New(f, pr, logging.Discard()), "search_issues", map[string]any{"author": "x repo:other/y"}); !isErr {
		t.Errorf("a bad author was searched:\n%s", text)
	}
}

func TestReadIssueLeavesOutOtherRepositories(t *testing.T) {
	f := &fakeGitHub{issue: model.IssueDetail{Repo: "babarot/gh-desk", Number: 47, PR: true, Title: "t", Body: "b",
		Closes:  []model.IssueRef{{Repo: "babarot/gh-desk", Number: 12}, {Repo: "other/private", Number: 99}},
		Threads: []model.Thread{{Path: "a.go", Line: 3, CommentCount: 1, Comments: []model.Comment{{Author: "x", Body: "nit"}}}}}}
	text, isErr := call(t, New(f, pr, logging.Discard()), "read_issue", map[string]any{"number": 47})
	if isErr || !strings.Contains(text, "<closes>#12</closes>") || strings.Contains(text, "99") || !strings.Contains(text, "a.go:3 (open, comments: 1)") {
		t.Errorf("text:\n%s", text)
	}
	onlyOurs(t, f)

	f.issue.Repo = "other/private"
	if text, isErr := call(t, New(f, pr, logging.Discard()), "read_issue", map[string]any{"number": 47}); !isErr || strings.Contains(text, "<body>") {
		t.Errorf("another repository's issue was given:\n%s", text)
	}
}

func TestCallsAreBounded(t *testing.T) {
	s := New(&fakeGitHub{}, pr, logging.Discard())
	h := handle(s, "checks", s.checks)
	ctx := context.Background()
	for i := range maxCalls {
		if _, _, err := h(ctx, nil, checksIn{}); err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
	}
	if _, _, err := h(ctx, nil, checksIn{}); err == nil || !strings.Contains(err.Error(), "refused") {
		t.Errorf("call over the bound: %v", err)
	}
}
