package app

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/babarot/gh-triage/internal/config"
	"github.com/babarot/gh-triage/internal/logging"
)

func TestKeysTemplateLoadsUncommented(t *testing.T) {
	// Every line of the [keys] template, uncommented, is a valid setting
	// that changes nothing.
	var lines []string
	for _, l := range strings.Split(KeysTemplate(), "\n") {
		if strings.HasPrefix(l, "# ") && strings.Contains(l, " = ") {
			lines = append(lines, strings.TrimPrefix(l, "# "))
		}
	}
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("[keys]\n"+strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, ps := applyKeys(cfg.Keys); len(ps) > 0 {
		t.Fatalf("problems: %v", ps)
	}
}

func TestSetupWritesTemplateAndReportsMistakes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gh-triage", "config.toml")
	var buf bytes.Buffer
	log := logging.New(&buf, slog.LevelInfo)
	level := new(slog.LevelVar)
	if _, _, err := setup(path, false, level, log); err != nil {
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
	_, _, err := setup(path, false, level, log)
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
	if _, _, err := setup(path, false, level, log); err != nil || level.Level() != slog.LevelWarn {
		t.Errorf("config: level = %v, err = %v", level.Level(), err)
	}
	t.Setenv(EnvLogLevel, "error")
	if _, _, err := setup(path, false, level, log); err != nil || level.Level() != slog.LevelError {
		t.Errorf("env: level = %v, err = %v", level.Level(), err)
	}
	if _, _, err := setup(path, true, level, log); err != nil || level.Level() != slog.LevelDebug {
		t.Errorf("--debug: level = %v, err = %v", level.Level(), err)
	}
}

func TestRunSubcommands(t *testing.T) {
	var out, errb bytes.Buffer
	if code := Run([]string{"version"}, &out, &errb); code != 0 || strings.TrimSpace(out.String()) != Version {
		t.Errorf("version: code %d, out %q", code, out.String())
	}
	out.Reset()
	t.Setenv("XDG_STATE_HOME", "/tmp/state")
	if code := Run([]string{"log"}, &out, &errb); code != 0 || strings.TrimSpace(out.String()) != "/tmp/state/gh-triage/logs/gh-triage.log" {
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
	_, _, err := setup(path, false, new(slog.LevelVar), logging.Discard())
	if err == nil || !strings.Contains(err.Error(), `claude.allowed_tools[1]`) || !strings.Contains(err.Error(), `"Bash"`) {
		t.Errorf("err = %v", err)
	}
}
