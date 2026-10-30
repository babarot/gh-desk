package claude

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/babarot/gh-triage/internal/logging"
	"github.com/babarot/gh-triage/internal/model"
)

const stream = `{"type":"system","subtype":"init"}
{"type":"assistant","message":{"content":[{"type":"text","text":"Looking."},{"type":"tool_use","name":"Read","input":{"file_path":"/w/main.go"}}]}}
{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Grep","input":{"pattern":"TODO"}},{"type":"tool_use","name":"StructuredOutput","input":{}}]}}
{"type":"result","is_error":false,"result":"  Looks fine.  ","total_cost_usd":0.25,"modelUsage":{"claude-opus-5-5":{"costUSD":0.2},"claude-haiku-5-5":{"costUSD":0.05}},"structured_output":{"event":"APPROVE","body":"LGTM"}}
`

func TestReadStream(t *testing.T) {
	var steps []Step
	a, err := readStream(strings.NewReader(stream), func(s Step) { steps = append(steps, s) })
	if err != nil {
		t.Fatal(err)
	}
	want := []Step{{"Read", "/w/main.go"}, {"Grep", "TODO"}}
	if !slices.Equal(steps, want) {
		t.Errorf("steps = %v", steps)
	}
	if a.Text != "Looks fine." || a.Cost != 0.25 || a.Model != "claude-opus-5-5" {
		t.Errorf("answer = %+v", a)
	}
	d, err := ParseDraft(a)
	if err != nil || d.Event != model.EventApprove || d.Body != "LGTM" {
		t.Errorf("draft = %+v, %v", d, err)
	}
}

func TestReadStreamError(t *testing.T) {
	_, err := readStream(strings.NewReader(`{"type":"result","is_error":true,"result":"rate limited"}`+"\n"), func(Step) {})
	if err == nil || !strings.Contains(err.Error(), "rate limited") {
		t.Errorf("err = %v", err)
	}
	if _, err := readStream(strings.NewReader(`{"type":"system"}`+"\n"), func(Step) {}); err == nil {
		t.Error("a stream without a result should fail")
	}
}

// fakeClaude is a claude that writes its arguments and stdin to files in
// dir and answers.
func fakeClaude(t *testing.T) (bin, dir string) {
	t.Helper()
	dir = t.TempDir()
	bin = filepath.Join(dir, "claude")
	script := `#!/bin/sh
printf '%s\0' "$@" > "` + dir + `/args"
cat > "` + dir + `/stdin"
pwd > "` + dir + `/pwd"
echo '{"type":"result","is_error":false,"result":"ok","total_cost_usd":0.1}'
`
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return bin, dir
}

// args are the arguments the fake claude in dir was last run with.
func args(t *testing.T, dir string) []string {
	t.Helper()
	return strings.Split(strings.TrimSuffix(read(t, filepath.Join(dir, "args")), "\x00"), "\x00")
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestAskIsRestricted(t *testing.T) {
	bin, out := fakeClaude(t)
	wt := t.TempDir()
	c := New(Options{Bin: bin, Model: "opus", Tools: []string{"Read", "Grep", "Glob"}, Language: "Japanese", Instructions: "Point out missing tests.", Log: logging.Discard()})
	a, err := c.Ask(context.Background(), Request{Dir: wt, Session: "s-1", First: true, Prompt: "Review this.\n<diff>…</diff>"}, func(Step) {})
	if err != nil || a.Text != "ok" {
		t.Fatalf("answer %+v, %v", a, err)
	}
	got := args(t, out)
	for _, want := range [][]string{
		{"--restricted"}, {"--strict-mcp-config"}, {"--mcp-config", `{"mcpServers":{}}`}, {"--tools", "Read,Grep,Glob"},
		{"--allowedTools", "Read,Grep,Glob"}, {"--session-id", "s-1"}, {"--model", "opus"},
	} {
		i := slices.Index(got, want[0])
		if i < 0 || (len(want) == 2 && (i+1 >= len(got) || got[i+1] != want[1])) {
			t.Errorf("%v not in %q", want, got)
		}
	}
	if slices.Contains(got, "--resume") || slices.Contains(got, "Review this.") {
		t.Errorf("got = %q", got)
	}
	if i := slices.Index(got, "--append-system-prompt"); i < 0 || !strings.HasSuffix(got[i+1], "Answer in Japanese.\n\nPoint out missing tests.") {
		t.Errorf("language and instructions not appended: %q", got)
	}
	if got := read(t, filepath.Join(out, "stdin")); !strings.HasPrefix(got, "Review this.") {
		t.Errorf("stdin = %q", got)
	}
	if got, _ := filepath.EvalSymlinks(strings.TrimSpace(read(t, filepath.Join(out, "pwd")))); got != mustEval(t, wt) {
		t.Errorf("ran in %s, not %s", got, wt)
	}

	if _, err := c.Ask(context.Background(), Request{Dir: wt, Session: "s-1", Prompt: "more?", Schema: DraftSchema}, func(Step) {}); err != nil {
		t.Fatal(err)
	}
	got = args(t, out)
	if i := slices.Index(got, "--resume"); i < 0 || got[i+1] != "s-1" || slices.Contains(got, "--session-id") || !slices.Contains(got, "--json-schema") {
		t.Errorf("resume got = %q", got)
	}

	cmd := c.Interactive(wt, "s-1")
	if !slices.Contains(cmd.Args, "--restricted") || slices.Contains(cmd.Args, "--allowedTools") || cmd.Dir != wt {
		t.Errorf("interactive = %q in %s", cmd.Args, cmd.Dir)
	}
}

func TestOnceHasNoTools(t *testing.T) {
	bin, out := fakeClaude(t)
	c := New(Options{Bin: bin, Model: "opus", Tools: []string{"Read"}, Log: logging.Discard()})
	if _, err := c.Once(context.Background(), t.TempDir(), "notes", NotesSchema); err != nil {
		t.Fatal(err)
	}
	got := args(t, out)
	if i := slices.Index(got, "--tools"); i < 0 || got[i+1] != "" || !slices.Contains(got, "--no-session-persistence") {
		t.Errorf("got = %q", got)
	}
}

func mustEval(t *testing.T, p string) string {
	t.Helper()
	r, err := filepath.EvalSymlinks(p)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestParseCautions(t *testing.T) {
	asked := []Notes{{Key: model.PRKey{Owner: "acme", Repo: "api", Number: 1}}, {Key: model.PRKey{Owner: "acme", Repo: "api", Number: 2}}}
	a := Answer{Structured: []byte(`{"cautions":[{"key":"acme/api#2","why":"drops Node 18"},{"key":"other/x#9","why":"not asked"}]}`)}
	got, err := ParseCautions(a, asked)
	if err != nil || len(got) != 1 || got[0].Key.Number != 2 || got[0].Why != "drops Node 18" {
		t.Errorf("got %+v, %v", got, err)
	}
}

func TestReviewPromptCutsLongDiffs(t *testing.T) {
	p := model.PR{Key: model.PRKey{Owner: "acme", Repo: "api", Number: 1}, Title: "Add a thing"}
	got := ReviewPrompt(p, model.Detail{Body: "body"}, []byte(strings.Repeat("x", maxDiff+10)))
	if !strings.Contains(got, "the diff is cut here") || strings.Count(got, "x") > maxDiff+5 {
		t.Error("a long diff is not cut")
	}
}

// gitRun runs git in dir for a test.
func gitRun(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com")
	b, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, b)
	}
	return strings.TrimSpace(string(b))
}

// origin makes a repository with a pull request's head at refs/pull/1/head
// and returns it and the head.
func origin(t *testing.T) (string, string) {
	dir := t.TempDir()
	gitRun(t, dir, "init", "-q", "-b", "main")
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("one\n"), 0o644)
	gitRun(t, dir, "add", ".")
	gitRun(t, dir, "commit", "-qm", "one")
	gitRun(t, dir, "checkout", "-qb", "feature")
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("two\n"), 0o644)
	gitRun(t, dir, "commit", "-qam", "two")
	head := gitRun(t, dir, "rev-parse", "HEAD")
	gitRun(t, dir, "update-ref", "refs/pull/1/head", head)
	gitRun(t, dir, "checkout", "-q", "main")
	gitRun(t, dir, "config", "uploadpack.allowFilter", "true")
	return dir, head
}

func TestWorkspace(t *testing.T) {
	src, head := origin(t)
	w := &Workspace{Dir: t.TempDir(), Log: logging.Discard(), Remote: func(string, string) string { return "file://" + src }}
	k := model.PRKey{Owner: "acme", Repo: "api", Number: 1}
	wt, err := w.Prepare(context.Background(), k, head)
	if err != nil {
		t.Fatal(err)
	}
	if got := read(t, filepath.Join(wt, "a.txt")); got != "two\n" {
		t.Errorf("a.txt = %q", got)
	}

	// A new push: the worktree moves to it.
	gitRun(t, src, "checkout", "-q", "feature")
	os.WriteFile(filepath.Join(src, "a.txt"), []byte("three\n"), 0o644)
	gitRun(t, src, "commit", "-qam", "three")
	head2 := gitRun(t, src, "rev-parse", "HEAD")
	gitRun(t, src, "update-ref", "refs/pull/1/head", head2)
	if _, err := w.Prepare(context.Background(), k, head2); err != nil {
		t.Fatal(err)
	}
	if got := read(t, filepath.Join(wt, "a.txt")); got != "three\n" {
		t.Errorf("a.txt after the push = %q", got)
	}

	wts, err := w.Worktrees()
	if err != nil || len(wts) != 1 || wts[0].Key != k {
		t.Errorf("worktrees = %v, %v", wts, err)
	}
	if err := w.Remove(context.Background(), k); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(wt); !os.IsNotExist(err) {
		t.Errorf("the worktree is still there: %v", err)
	}
}

// A clone of the user's lends its objects, and the cache's clone does not
// depend on it afterwards.
func TestWorkspaceBorrows(t *testing.T) {
	src, head := origin(t)
	root := t.TempDir()
	mine := filepath.Join(root, "github.com", "acme", "api")
	gitRun(t, root, "clone", "-q", src, mine)
	w := &Workspace{Dir: t.TempDir(), Borrow: []string{root}, Log: logging.Discard(), Remote: func(string, string) string { return "file://" + src }}
	k := model.PRKey{Owner: "acme", Repo: "api", Number: 1}
	if _, err := w.Prepare(context.Background(), k, head); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(w.Dir, "repos", "acme", "api", ".git", "objects", "info", "alternates")); err == nil {
		t.Error("the clone still borrows from the user's")
	}
	before := gitRun(t, mine, "status", "--porcelain")
	if before != "" {
		t.Errorf("the user's clone changed: %q", before)
	}
	if out := gitRun(t, mine, "worktree", "list"); strings.Count(out, "\n") != 0 {
		t.Errorf("a worktree was added to the user's clone: %s", out)
	}
}

// A hook the pull request adds never runs, even when the user's config
// points core.hooksPath into the checkout.
func TestWorkspaceRunsNoHooks(t *testing.T) {
	src, _ := origin(t)
	gitRun(t, src, "checkout", "-q", "feature")
	os.MkdirAll(filepath.Join(src, ".githooks"), 0o755)
	ran := filepath.Join(t.TempDir(), "ran")
	os.WriteFile(filepath.Join(src, ".githooks", "post-checkout"), []byte("#!/bin/sh\ntouch "+ran+"\n"), 0o755)
	gitRun(t, src, "add", ".")
	gitRun(t, src, "commit", "-qm", "hook")
	head := gitRun(t, src, "rev-parse", "HEAD")
	gitRun(t, src, "update-ref", "refs/pull/1/head", head)
	gitRun(t, src, "checkout", "-q", "main")

	global := filepath.Join(t.TempDir(), "gitconfig")
	os.WriteFile(global, []byte("[core]\n\thooksPath = .githooks\n"), 0o644)
	t.Setenv("GIT_CONFIG_GLOBAL", global)
	w := &Workspace{Dir: t.TempDir(), Log: logging.Discard(), Remote: func(string, string) string { return "file://" + src }}
	k := model.PRKey{Owner: "acme", Repo: "api", Number: 1}
	if _, err := w.Prepare(context.Background(), k, head); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Prepare(context.Background(), k, head); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(ran); err == nil {
		t.Error("a hook from the pull request ran")
	}
}

// A checkout cut short leaves its lock; the next Prepare makes the
// worktree again.
func TestWorkspaceRecoversFromALock(t *testing.T) {
	src, head := origin(t)
	w := &Workspace{Dir: t.TempDir(), Log: logging.Discard(), Remote: func(string, string) string { return "file://" + src }}
	k := model.PRKey{Owner: "acme", Repo: "api", Number: 1}
	wt, err := w.Prepare(context.Background(), k, head)
	if err != nil {
		t.Fatal(err)
	}
	gitDir := gitRun(t, wt, "rev-parse", "--absolute-git-dir")
	if err := os.WriteFile(filepath.Join(gitDir, "index.lock"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	base := gitRun(t, src, "rev-parse", "main")
	if _, err := w.Prepare(context.Background(), k, base); err != nil {
		t.Fatal(err)
	}
	if got := read(t, filepath.Join(wt, "a.txt")); got != "one\n" {
		t.Errorf("a.txt = %q", got)
	}
}
