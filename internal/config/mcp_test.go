package config

import (
	"slices"
	"strings"
	"testing"
)

func TestMCPServers(t *testing.T) {
	cfg, _, err := Load(write(t, `[claude.mcp]
builtin = false

[claude.mcp.servers.recall]
command = "recall"
args = ["mcp", "--repo", "${GH_DESK_URL}"]
env = { TOKEN = "s3cret" }
tools = ["recall_search", "recall_list"]
`))
	if err != nil {
		t.Fatal(err)
	}
	srv := cfg.Claude.MCP.Servers["recall"]
	if srv["command"] != "recall" || !slices.Equal(MCPTools(srv), []string{"recall_search", "recall_list"}) {
		t.Errorf("recall = %v", srv)
	}
	shown := Show(cfg, true)
	if strings.Contains(shown, "s3cret") || !strings.Contains(shown, `TOKEN = "(hidden)"`) || !strings.Contains(shown, `tools = ["recall_search", "recall_list"]`) {
		t.Errorf("Show = %s", shown)
	}
}

func TestMCPServerMistakes(t *testing.T) {
	for _, tc := range []struct {
		name, body, want string
	}{
		{"no tools", `[claude.mcp.servers.recall]
command = "recall"
`, "config.toml:1:21: claude.mcp.servers.recall needs tools"},
		{"empty tools", `[claude.mcp.servers.recall]
command = "recall"
tools = []
`, "claude.mcp.servers.recall needs tools"},
		{"a pattern", `[claude.mcp.servers.recall]
command = "recall"
tools = ["recall_search", "*"]
`, `config.toml:3:27: claude.mcp.servers.recall.tools[1] must name one tool`},
		{"a name with a dot", `[claude.mcp.servers."a.b"]
command = "x"
tools = ["t"]
`, "a server's name is made of letters"},
		{"args not strings", `[claude.mcp.servers.recall]
command = "recall"
args = ["mcp", 1]
tools = ["t"]
`, "config.toml:3:1: claude.mcp.servers.recall.args must be a list of strings"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := Load(write(t, tc.body))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("err = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestBuiltinMCP(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		want       bool
		err        string
	}{
		{"left out, with nothing to send out", ``, true, ""},
		{"left out, with WebFetch", "[claude]\nallowed_tools = [\"Read\", \"WebFetch\"]\n", false, "claude.mcp.builtin must be written as true or false"},
		{"left out, with a server", "[claude.mcp.servers.recall]\ncommand = \"recall\"\ntools = [\"t\"]\n", false, "config.toml:1:21: claude.mcp.builtin must be written"},
		{"written, with WebFetch", "[claude]\nallowed_tools = [\"WebFetch\"]\n[claude.mcp]\nbuiltin = true\n", true, ""},
		{"turned off", "[claude.mcp]\nbuiltin = false\n", false, ""},
		{"the built-in name taken", "[claude.mcp]\nbuiltin = true\n[claude.mcp.servers.gh-desk]\ncommand = \"x\"\ntools = [\"t\"]\n", false, `"gh-desk" is the name of gh-desk's own server`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg, _, err := Load(write(t, tc.body))
			if tc.err != "" {
				if err == nil || !strings.Contains(err.Error(), tc.err) {
					t.Errorf("err = %v, want %q", err, tc.err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got := cfg.BuiltinMCP(); got != tc.want {
				t.Errorf("BuiltinMCP = %v", got)
			}
			if again, _, err := Parse("config.toml", []byte(Show(cfg, true))); err != nil || again.BuiltinMCP() != tc.want {
				t.Errorf("Show does not read back: %v\n%s", err, Show(cfg, true))
			}
		})
	}
}
