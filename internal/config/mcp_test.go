package config

import (
	"slices"
	"strings"
	"testing"
)

func TestMCPServers(t *testing.T) {
	cfg, _, err := Load(write(t, `[claude.mcp_servers.recall]
command = "recall"
args = ["mcp", "--repo", "${GH_TRIAGE_URL}"]
env = { TOKEN = "s3cret" }
tools = ["recall_search", "recall_list"]
`))
	if err != nil {
		t.Fatal(err)
	}
	srv := cfg.Claude.MCPServers["recall"]
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
		{"no tools", `[claude.mcp_servers.recall]
command = "recall"
`, "config.toml:1:21: claude.mcp_servers.recall needs tools"},
		{"empty tools", `[claude.mcp_servers.recall]
command = "recall"
tools = []
`, "claude.mcp_servers.recall needs tools"},
		{"a pattern", `[claude.mcp_servers.recall]
command = "recall"
tools = ["recall_search", "*"]
`, `config.toml:3:27: claude.mcp_servers.recall.tools[1] must name one tool`},
		{"a name with a dot", `[claude.mcp_servers."a.b"]
command = "x"
tools = ["t"]
`, "a server's name is made of letters"},
		{"args not strings", `[claude.mcp_servers.recall]
command = "recall"
args = ["mcp", 1]
tools = ["t"]
`, "config.toml:3:1: claude.mcp_servers.recall.args must be a list of strings"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := Load(write(t, tc.body))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("err = %v, want %q", err, tc.want)
			}
		})
	}
}
