package app

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/babarot/gh-desk/internal/claude"
	"github.com/babarot/gh-desk/internal/config"
	"github.com/babarot/gh-desk/internal/logging"
	"github.com/babarot/gh-desk/internal/model"
	"github.com/babarot/gh-desk/internal/state"
	"github.com/babarot/gh-desk/internal/tui/keymap"
	"github.com/babarot/gh-desk/internal/version"
)

func TestKeysTemplateLoadsUncommented(t *testing.T) {
	// Every line of the [keys] template, uncommented, is a valid setting
	// that changes nothing.
	var lines []string
	for l := range strings.SplitSeq(KeysTemplate(), "\n") {
		if strings.HasPrefix(l, "# ") && strings.Contains(l, " = ") {
			lines = append(lines, strings.TrimPrefix(l, "# "))
		}
	}
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("[keys]\n"+strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, _, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, ps := applyKeys(cfg.Keys); len(ps) > 0 {
		t.Fatalf("problems: %v", ps)
	}
}

func TestEveryScopeHasWords(t *testing.T) {
	// The [keys] template heads each scope with its words; a scope added to
	// the keymap without them would get an empty heading.
	for _, d := range keymap.Defaults {
		if strings.TrimSpace(scopeWords[d.Scope]) == "" {
			t.Errorf("scope %q of %s has no words in scopeWords", d.Scope, d.Action)
		}
	}
	if strings.Contains(KeysTemplate(), "# :") {
		t.Errorf("template has an empty heading:\n%s", KeysTemplate())
	}
}

func TestEditorPrecedence(t *testing.T) {
	tests := []struct {
		name, set, visual, editor, want string
	}{
		{"config first", "code --wait", "vi", "nano", "code --wait"},
		{"then VISUAL", "", "vi", "nano", "vi"},
		{"then EDITOR", "", "", "nano", "nano"},
		{"blank config is unset", "  ", "", "nano", "nano"},
		{"none", "", "", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := map[string]string{"VISUAL": tt.visual, "EDITOR": tt.editor}
			if got := editor(tt.set, func(k string) string { return env[k] }); got != tt.want {
				t.Errorf("editor = %q, want %q", got, tt.want)
			}
		})
	}
	// Through the config file, arguments and all.
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("[review]\neditor = \"code --wait\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("VISUAL", "vi")
	cfg, _, _, err := setup(path, false, new(slog.LevelVar), logging.Discard())
	if err != nil {
		t.Fatal(err)
	}
	if got := Settings(cfg).Editor; got != "code --wait" {
		t.Errorf("Settings.Editor = %q", got)
	}
}

func TestSetupWritesTemplateAndReportsMistakes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gh-desk", "config.toml")
	var buf bytes.Buffer
	log := logging.New(&buf, slog.LevelInfo)
	level := new(slog.LevelVar)
	if _, _, _, err := setup(path, false, level, log); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("template not written: %v", err)
	}
	if !strings.Contains(buf.String(), "wrote the config file") {
		t.Errorf("log = %s", buf.String())
	}
	if err := os.WriteFile(path, []byte("[log]\nlevel = \"loud\"\n[keys]\napprove = \"shift+a\"\nmerge = 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, _, _, err := setup(path, false, level, log)
	if err == nil {
		t.Fatal("no error")
	}
	for _, want := range []string{"config.toml:2:9: log.level", "config.toml:4:11: keys.approve", `write "A"`, "keys.merge must be a key"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error does not contain %q:\n%v", want, err)
		}
	}
}

func TestLevelPrecedence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("[log]\nlevel = \"warn\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	level := new(slog.LevelVar)
	log := logging.Discard()
	if _, _, _, err := setup(path, false, level, log); err != nil || level.Level() != slog.LevelWarn {
		t.Errorf("config: level = %v, err = %v", level.Level(), err)
	}
	t.Setenv(EnvLogLevel, "error")
	if _, _, _, err := setup(path, false, level, log); err != nil || level.Level() != slog.LevelError {
		t.Errorf("env: level = %v, err = %v", level.Level(), err)
	}
	if _, _, _, err := setup(path, true, level, log); err != nil || level.Level() != slog.LevelDebug {
		t.Errorf("--debug: level = %v, err = %v", level.Level(), err)
	}
}

func TestRunSubcommands(t *testing.T) {
	var out, errb bytes.Buffer
	if code := Run([]string{"--version"}, &out, &errb); code != 0 || strings.TrimSpace(out.String()) != version.String() {
		t.Errorf("--version: code %d, out %q", code, out.String())
	}
	out.Reset()
	t.Setenv("XDG_STATE_HOME", "/tmp/state")
	if code := Run([]string{"log"}, &out, &errb); code != 0 || strings.TrimSpace(out.String()) != "/tmp/state/gh-desk/logs/gh-desk.log" {
		t.Errorf("log: code %d, out %q", code, out.String())
	}
	if code := Run([]string{"nope"}, &out, &errb); code != 2 {
		t.Errorf("unknown command: code %d", code)
	}
}

func TestOnlyReadingToolsForClaude(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("[claude]\nallowed_tools = [\"Read\", \"Bash\"]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, _, _, err := setup(path, false, new(slog.LevelVar), logging.Discard())
	if err == nil || !strings.Contains(err.Error(), `claude.allowed_tools[1]`) || !strings.Contains(err.Error(), `"Bash"`) {
		t.Errorf("err = %v", err)
	}
}

type fakePRs map[model.PRKey]model.PR

func (f fakePRs) PullRequests(_ context.Context, keys []model.PRKey) (map[model.PRKey]model.PR, error) {
	out := map[model.PRKey]model.PR{}
	for _, k := range keys {
		if p, ok := f[k]; ok {
			out[k] = p
		}
	}
	return out, nil
}

func TestGCRemovesClosedOnly(t *testing.T) {
	dir := t.TempDir()
	ws := &claude.Workspace{Dir: dir, Log: logging.Discard()}
	open, merged, gone := model.PRKey{Owner: "acme", Repo: "api", Number: 1}, model.PRKey{Owner: "acme", Repo: "api", Number: 2}, model.PRKey{Owner: "acme", Repo: "web", Number: 3}
	for _, k := range []model.PRKey{open, merged, gone} {
		if err := os.MkdirAll(ws.WorktreePath(k), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	// Conversations kept: one of the merged pull request, one of a closed
	// pull request with no worktree, and one of a closed pull request a
	// gh-desk is talking about.
	closed, talking := model.PRKey{Owner: "acme", Repo: "web", Number: 4}, model.PRKey{Owner: "acme", Repo: "web", Number: 5}
	chats := state.OpenChats(t.TempDir(), "github.com")
	for _, k := range []model.PRKey{open, merged, closed, talking} {
		if err := chats.Save(state.ChatRecord{Key: k, Rev: 1}); err != nil {
			t.Fatal(err)
		}
	}
	unlock, _, _, err := chats.Lock(talking)
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	prs := fakePRs{open: {Key: open, State: model.StateOpen}, merged: {Key: merged, State: model.StateMerged},
		closed: {Key: closed, State: model.StateClosed}, talking: {Key: talking, State: model.StateClosed}}
	var out bytes.Buffer
	if err := gc(context.Background(), ws, chats, state.Runs{Dir: filepath.Join(t.TempDir(), "runs")}, prs, &out); err != nil {
		t.Fatal(err)
	}
	for k, want := range map[model.PRKey]bool{open: true, merged: false, gone: true} {
		_, err := os.Stat(ws.WorktreePath(k))
		if (err == nil) != want {
			t.Errorf("%s kept = %v, want %v", k, err == nil, want)
		}
	}
	for k, want := range map[model.PRKey]bool{open: true, merged: false, closed: false, talking: true} {
		if _, ok, _ := chats.Load(k); ok != want {
			t.Errorf("conversation of %s kept = %v, want %v", k, ok, want)
		}
	}
	if !strings.Contains(out.String(), "2 removed, 3 kept") || !strings.Contains(out.String(), "kept acme/web#5") {
		t.Errorf("out = %s", out.String())
	}
}
