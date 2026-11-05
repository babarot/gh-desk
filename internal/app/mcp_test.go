package app

import (
	"slices"
	"testing"

	"github.com/babarot/gh-desk/internal/config"
	"github.com/babarot/gh-desk/internal/mcpserver"
	"github.com/babarot/gh-desk/internal/model"
)

func TestTheBuiltinServerNameIsReserved(t *testing.T) {
	if config.BuiltinServer != mcpserver.Name {
		t.Errorf("config reserves %q, the server is %q", config.BuiltinServer, mcpserver.Name)
	}
}

func TestMCPServersTakeTheBuiltinOne(t *testing.T) {
	cfg := config.Default()
	servers := mcpServers(cfg, "/home/me/.config/gh-desk/config.toml", "/bin/gh-desk")
	srv, ok := servers[mcpserver.Name]
	if !ok {
		t.Fatalf("servers = %v", servers)
	}
	args, _ := srv["args"].([]any)
	if srv["command"] != "/bin/gh-desk" || !slices.Equal(args, []any{"mcp", "--config", "/home/me/.config/gh-desk/config.toml"}) ||
		len(config.MCPTools(srv)) != len(mcpserver.Tools) {
		t.Errorf("server = %v", srv)
	}
	if !slices.Contains(skillTools(cfg), "mcp__gh-desk__check_run_log") {
		t.Errorf("skills may not grant the built-in tools: %v", skillTools(cfg))
	}
	if len(mcpServers(cfg, "c", "")) != 0 {
		t.Error("given without a program to start")
	}

	off := false
	cfg.Claude.MCP.Builtin = &off
	cfg.Claude.MCP.Servers = map[string]map[string]any{"recall": {"command": "recall", "tools": []any{"recall_search"}}}
	servers = mcpServers(cfg, "c", "/bin/gh-desk")
	if _, ok := servers[mcpserver.Name]; ok || len(servers) != 1 {
		t.Errorf("servers with builtin = false: %v", servers)
	}
	if slices.Contains(skillTools(cfg), "mcp__gh-desk__checks") {
		t.Error("skills may grant the tools of a server not given")
	}
	// The user's servers are not changed by adding the built-in one.
	on := true
	cfg.Claude.MCP.Builtin = &on
	_ = mcpServers(cfg, "c", "/bin/gh-desk")
	if len(cfg.Claude.MCP.Servers) != 1 {
		t.Errorf("the config's servers were changed: %v", cfg.Claude.MCP.Servers)
	}
}

func TestPRFromEnv(t *testing.T) {
	env := map[string]string{envOwner: "babarot", envRepo: "gh-desk", envNumber: "47"}
	k, err := prFromEnv(func(k string) string { return env[k] })
	if err != nil || k != (model.PRKey{Owner: "babarot", Repo: "gh-desk", Number: 47}) {
		t.Errorf("prFromEnv = %v, %v", k, err)
	}
	for _, missing := range []string{envOwner, envRepo, envNumber} {
		e := map[string]string{envOwner: "babarot", envRepo: "gh-desk", envNumber: "47"}
		delete(e, missing)
		if _, err := prFromEnv(func(k string) string { return e[k] }); err == nil {
			t.Errorf("without %s: no error", missing)
		}
	}
}
