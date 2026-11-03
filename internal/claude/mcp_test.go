package claude

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/babarot/gh-desk/internal/logging"
	"github.com/babarot/gh-desk/internal/model"
)

func TestWriteMCPConfig(t *testing.T) {
	dir, cwd := t.TempDir(), t.TempDir()
	servers := map[string]map[string]any{
		"recall": {"command": "recall", "args": []any{"mcp", "--repo", "${GH_DESK_URL}"}, "env": map[string]any{"TOKEN": "${TOKEN}"}, "tools": []any{"recall_search", "recall_list"}},
		"web":    {"type": "http", "url": "https://example.com/mcp", "tools": []any{"fetch"}},
	}
	path, tools, err := WriteMCPConfig(dir, cwd, servers)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"mcp__recall__recall_search", "mcp__recall__recall_list", "mcp__web__fetch"}; !slices.Equal(tools, want) {
		t.Errorf("tools = %q, want %q", tools, want)
	}
	if fi, err := os.Stat(path); err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("file %v, %v; want readable only by the user", fi.Mode(), err)
	}
	var got struct {
		MCPServers map[string]map[string]any `json:"mcpServers"`
	}
	if err := json.Unmarshal([]byte(read(t, path)), &got); err != nil {
		t.Fatal(err)
	}
	recall, web := got.MCPServers["recall"], got.MCPServers["web"]
	if _, ok := recall["tools"]; ok {
		t.Errorf("tools went to Claude Code: %v", recall)
	}
	if recall["command"] != "/bin/sh" || recall["env"].(map[string]any)["TOKEN"] != "${TOKEN}" {
		t.Errorf("recall = %v", recall)
	}
	if web["url"] != "https://example.com/mcp" || web["command"] != nil {
		t.Errorf("web = %v", web)
	}
	// The wrapped command runs in cwd, its arguments as they are.
	var args []string
	for _, a := range recall["args"].([]any) {
		args = append(args, a.(string))
	}
	args[3], args[4] = mustEval(t, cwd), "sh"
	args = append(args[:5], "-c", `pwd; printf '%s\n' "$@"`, "x", "a b", "$(echo no)")
	out, err := exec.Command("/bin/sh", args...).Output()
	if err != nil {
		t.Fatal(err)
	}
	if want := mustEval(t, cwd) + "\na b\n$(echo no)\n"; string(out) != want {
		t.Errorf("ran %q, want %q", out, want)
	}
}

func TestConversationTakesServersAndGuard(t *testing.T) {
	bin, out := fakeClaude(t)
	wt := t.TempDir()
	c := New(Options{Bin: bin, Model: "opus", Tools: []string{"Read"}, MCPConfig: "/cache/runs/run-1/mcp.json",
		MCPTools: []string{"mcp__recall__recall_search"}, Log: logging.Discard()})
	pr := model.PRKey{Owner: "babarot", Repo: "gh-desk", Number: 7}
	if _, err := c.Ask(context.Background(), Request{PR: pr, Dir: wt, Session: "s-1", First: true, Prompt: "Review."}, func(Step) {}); err != nil {
		t.Fatal(err)
	}
	conv := args(t, out)
	for _, want := range [][]string{{"--settings", guard}, {"--mcp-config", "/cache/runs/run-1/mcp.json"}, {"--allowedTools", "Read,mcp__recall__recall_search"}, {"--strict-mcp-config"}} {
		if i := slices.Index(conv, want[0]); i < 0 || len(want) == 2 && conv[i+1] != want[1] {
			t.Errorf("%v not in %q", want, conv)
		}
	}
	if env := read(t, filepath.Join(out, "env")); !strings.Contains(env, "GH_DESK_URL=https://github.com/babarot/gh-desk\n") || !strings.Contains(env, "GH_DESK_NUMBER=7\n") {
		t.Errorf("env = %q", env)
	}
	cmd := c.Interactive(pr, wt, "s-1")
	if i := slices.Index(cmd.Args, "--settings"); i < 0 || cmd.Args[i+1] != guard || !slices.Contains(cmd.Args, "/cache/runs/run-1/mcp.json") || !slices.Contains(cmd.Env, "GH_DESK_REPO=gh-desk") {
		t.Errorf("interactive = %q", cmd.Args)
	}

	// The runs with no tools take neither.
	if _, err := c.Once(context.Background(), t.TempDir(), "notes", ""); err != nil {
		t.Fatal(err)
	}
	if got := args(t, out); slices.Contains(got, "--settings") || slices.Contains(got, "/cache/runs/run-1/mcp.json") {
		t.Errorf("once = %q", got)
	}
	if _, err := c.Triage(context.Background(), Request{Dir: t.TempDir(), Session: "s-2", First: true, Prompt: "plan"}); err != nil {
		t.Fatal(err)
	}
	if got := args(t, out); slices.Contains(got, "--settings") || slices.Contains(got, "/cache/runs/run-1/mcp.json") || read(t, filepath.Join(out, "env")) != "" {
		t.Errorf("triage = %q", got)
	}
}
