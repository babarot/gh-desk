package claude

import (
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"slices"
)

// cdThenRun is the script a server started with a command runs through:
// it changes to its first argument and runs the rest, read as arguments,
// never as shell.
const cdThenRun = `cd "$1" && shift && exec "$@"`

// WriteMCPConfig writes servers, by name as Claude Code configures a
// server with the tools that may run without a prompt in "tools", as the
// file --mcp-config takes, in dir and readable only by the user, since
// their env and headers may hold secrets. A server started with a command
// is started in cwd, so it reads nothing of the worktree Claude runs in.
// It returns the file and the tools as --allowedTools names them.
func WriteMCPConfig(dir, cwd string, servers map[string]map[string]any) (path string, tools []string, err error) {
	out := map[string]any{}
	for _, name := range slices.Sorted(maps.Keys(servers)) {
		srv := maps.Clone(servers[name])
		if ts, ok := srv["tools"].([]any); ok {
			for _, t := range ts {
				if s, ok := t.(string); ok {
					tools = append(tools, "mcp__"+name+"__"+s)
				}
			}
		}
		delete(srv, "tools")
		if command, ok := srv["command"].(string); ok {
			args := []any{"-c", cdThenRun, "sh", cwd, command}
			if a, ok := srv["args"].([]any); ok {
				args = append(args, a...)
			}
			srv["command"], srv["args"] = "/bin/sh", args
		}
		out[name] = srv
	}
	b, err := json.Marshal(map[string]any{"mcpServers": out})
	if err != nil {
		return "", nil, err
	}
	path = filepath.Join(dir, "mcp.json")
	if err := os.WriteFile(path, b, 0o600); err != nil {
		return "", nil, err
	}
	return path, tools, nil
}
