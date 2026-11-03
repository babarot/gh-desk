package claude

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/babarot/gh-triage/internal/logging"
)

// plugin makes a plugin in a new directory, files by path under it.
func plugin(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	files[".claude-plugin/plugin.json"] = `{"name":"made-up"}`
	for p, body := range files {
		full := filepath.Join(dir, p)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func skillMD(front string) string { return "---\nname: x\n" + front + "---\nDo the thing.\n" }

var allowed = []string{"Read", "Grep", "Skill", "mcp__recall__recall_search"}

func TestPluginSkillsTakeOnlySkills(t *testing.T) {
	dir := plugin(t, map[string]string{
		"skills/review/SKILL.md":          "---\nname: review\nallowed-tools: Read, Grep\n---\nRead references/how.md.\n",
		"skills/review/references/how.md": "how",
		"hooks/hooks.json":                `{"hooks":{}}`,
		".lsp.json":                       `{}`,
		".mcp.json":                       `{"mcpServers":{}}`,
		"commands/c.md":                   "a command",
	})
	skills, ps := PluginSkills([]string{dir}, allowed)
	if len(ps) > 0 || len(skills) != 1 || skills[0].Name != "gh-triage:review" {
		t.Fatalf("skills %+v, problems %+v", skills, ps)
	}
	root, err := WritePlugin(t.TempDir(), skills, allowed)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	_ = filepath.WalkDir(root, func(p string, d os.DirEntry, _ error) error {
		if !d.IsDir() {
			rel, _ := filepath.Rel(root, p)
			got = append(got, rel)
		}
		return nil
	})
	want := []string{".claude-plugin/plugin.json", "skills/review/SKILL.md", "skills/review/references/how.md"}
	if !slices.Equal(got, want) {
		t.Errorf("plugin has %q, want %q", got, want)
	}
}

func TestCheckSkillAllowedTools(t *testing.T) {
	for _, tc := range []struct {
		name, front string
		ok          bool
	}{
		{"none", "", true},
		{"a string", "allowed-tools: Read, Grep\n", true},
		{"spaces", "allowed-tools: Read Grep\n", true},
		{"a list", "allowed-tools:\n  - Read\n  - mcp__recall__recall_search\n", true},
		{"a tool not given", "allowed-tools: Read, WebFetch\n", false},
		{"an MCP tool not in tools", "allowed-tools: mcp__recall__recall_delete\n", false},
		{"a pattern of a server", "allowed-tools: mcp__recall__*\n", false},
		{"a whole server", "allowed-tools: mcp__recall\n", false},
		{"a command", "allowed-tools: Bash(git status:*)\n", false},
		{"the other spelling", "allowed_tools: Bash\n", false},
		{"not a list", "allowed-tools: {a: 1}\n", false},
		{"no frontmatter that reads", "allowed-tools: [Read\n", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := plugin(t, map[string]string{"skills/s/SKILL.md": skillMD(tc.front)})
			_, err := CheckSkill(filepath.Join(dir, "skills", "s"), allowed)
			if (err == nil) != tc.ok {
				t.Errorf("err = %v, want ok %v", err, tc.ok)
			}
		})
	}
	dir := plugin(t, map[string]string{"skills/s/notes.md": "no SKILL.md"})
	if _, err := CheckSkill(filepath.Join(dir, "skills", "s"), allowed); err == nil {
		t.Error("a skill with no SKILL.md was taken")
	}
}

func TestSplitTools(t *testing.T) {
	got := splitTools("Read, Bash(git status:*) Grep,mcp__a__b")
	if want := []string{"Read", "Bash(git status:*)", "Grep", "mcp__a__b"}; !slices.Equal(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestPluginSkillsRefuse(t *testing.T) {
	a := plugin(t, map[string]string{"skills/one/SKILL.md": skillMD("")})
	b := plugin(t, map[string]string{"skills/one/SKILL.md": skillMD("")})
	link := plugin(t, map[string]string{"skills/two/SKILL.md": skillMD("")})
	if err := os.Symlink("/etc", filepath.Join(link, "skills", "two", "etc")); err != nil {
		t.Fatal(err)
	}
	notPlugin := t.TempDir()
	_, ps := PluginSkills([]string{a, b, link, notPlugin}, allowed)
	var msgs []string
	for _, p := range ps {
		msgs = append(msgs, p.Message)
	}
	all := strings.Join(msgs, "\n")
	for _, want := range []string{"both a skill named one", "is a symlink", "is not a plugin"} {
		if !strings.Contains(all, want) {
			t.Errorf("no %q in %q", want, all)
		}
	}
}

func TestConversationTakesSkills(t *testing.T) {
	bin, out := fakeClaude(t)
	c := New(Options{Bin: bin, Model: "opus", Tools: []string{"Read"}, PluginDir: "/cache/runs/run-1/plugin", Log: logging.Discard()})
	if _, err := c.Ask(context.Background(), Request{Dir: t.TempDir(), Session: "s-1", First: true, Prompt: "Review."}, func(Step) {}); err != nil {
		t.Fatal(err)
	}
	got := args(t, out)
	for _, want := range [][]string{{"--tools", "Read,Skill"}, {"--allowedTools", "Read,Skill"}, {"--plugin-dir", "/cache/runs/run-1/plugin"}, {"--add-dir", "/cache/runs/run-1/plugin"}, {"--settings", guard}} {
		if i := slices.Index(got, want[0]); i < 0 || got[i+1] != want[1] {
			t.Errorf("%v not in %q", want, got)
		}
	}
	if slices.Contains(got, "--disable-slash-commands") {
		t.Errorf("skills are off: %q", got)
	}
	if _, err := c.Triage(context.Background(), Request{Dir: t.TempDir(), Session: "s-2", First: true, Prompt: "plan"}); err != nil {
		t.Fatal(err)
	}
	if got := args(t, out); slices.Contains(got, "--plugin-dir") || !slices.Contains(got, "--disable-slash-commands") {
		t.Errorf("triage = %q", got)
	}
}
