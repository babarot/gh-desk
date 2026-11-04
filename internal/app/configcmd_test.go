package app

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/babarot/gh-desk/internal/config"
)

// runConfig runs gh desk --config path config args.
func runConfig(t *testing.T, path string, args ...string) (int, string, string) {
	t.Helper()
	var out, errb bytes.Buffer
	code := Run(append([]string{"--config", path, "config"}, args...), &out, &errb)
	return code, out.String(), errb.String()
}

func writeFile(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestConfigCheck(t *testing.T) {
	t.Setenv(EnvLogLevel, "")
	tests := []struct {
		name, body string
		code       int
		want       []string
	}{
		{"fine", "[theme]\nlight = \"latte\"\n", 0, []string{"config.toml: no mistakes"}},
		{"warning", "[[tabs]]\nname = \"a\"\nquery = \"author:@me\"\n[[tabs]]\nkind = \"inbox\"\n", 0,
			[]string{"config.toml:3:9: warning: tabs[0].query has no is:pr", "no mistakes, 1 warning"}},
		{"mistake", "[theme]\nlight = \"dark\"\n", 1, []string{"theme.light must be one of"}},
		{"key mistake", "[keys]\naprove = \"a\"\n", 1, []string{`"aprove" is not an action; did you mean "approve"?`}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, out, _ := runConfig(t, writeFile(t, tt.body), "check")
			if code != tt.code {
				t.Errorf("code = %d, want %d\n%s", code, tt.code, out)
			}
			for _, w := range tt.want {
				if !strings.Contains(out, w) {
					t.Errorf("out =\n%s\nwant one containing %q", out, w)
				}
			}
		})
	}
	// A path after check wins over --config, and one not there is the
	// defaults.
	none := filepath.Join(t.TempDir(), "none.toml")
	if code, out, _ := runConfig(t, writeFile(t, "[theme]\nlight = \"dark\"\n"), "check", none); code != 0 || !strings.Contains(out, "the defaults are used") {
		t.Errorf("check %s: code %d, out %q", none, code, out)
	}
}

func TestConfigShow(t *testing.T) {
	path := writeFile(t, `
[[tabs]]
kind = "inbox"
[[tabs]]
name = "bots"
query = "is:open is:pr author:app/renovate"
groups = ["bots"]

[[select]]
name = "lock files"
authors = ["renovate"]
title = "(?i)lock file"
checks = ["passing"]

[list]
refresh = "off"
[review]
editor = "code --wait"
[keys]
quit = ["q", "Q"]
`)
	code, diff, _ := runConfig(t, path, "show", "--diff")
	if code != 0 {
		t.Fatalf("show --diff: code %d", code)
	}
	for _, want := range []string{"[[tabs]]\nkind = \"inbox\"", "groups = [\"bots\"]", "[[select]]\nname = \"lock files\"",
		"title = \"(?i)lock file\"", "[list]\nrefresh = \"off\"", "editor = \"code --wait\"", "[keys]\nquit = [\"q\", \"Q\"]"} {
		if !strings.Contains(diff, want) {
			t.Errorf("show --diff =\n%s\nwant one containing %q", diff, want)
		}
	}
	for _, not := range []string{"[theme]", "stale_days", "approve ="} {
		if strings.Contains(diff, not) {
			t.Errorf("show --diff has %q, which is at its default:\n%s", not, diff)
		}
	}
	// What show prints reads back as the same settings.
	_, all, _ := runConfig(t, path, "show")
	_, again, _ := runConfig(t, writeFile(t, all), "show")
	if all != again || !strings.Contains(all, "[theme]\ndark = \"default\"") {
		t.Errorf("show does not read back as itself:\n%s\n---\n%s", all, again)
	}
}

func TestConfigInitEditSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gh-desk", "config.toml")
	if code, out, _ := runConfig(t, path, "init"); code != 0 || !strings.Contains(out, "wrote") {
		t.Fatalf("init: code %d, out %q", code, out)
	}
	if code, out, _ := runConfig(t, path, "init"); code != 0 || !strings.Contains(out, "nothing was written") {
		t.Errorf("init again: code %d, out %q", code, out)
	}
	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", "true")
	if code, out, errb := runConfig(t, path, "edit"); code != 0 || !strings.Contains(out, "no mistakes") {
		t.Errorf("edit: code %d, out %q, err %q", code, out, errb)
	}
	t.Setenv("EDITOR", "")
	if code, _, errb := runConfig(t, path, "edit"); code != 1 || !strings.Contains(errb, "set review.editor") {
		t.Errorf("edit with no editor: code %d, err %q", code, errb)
	}
	if code, out, _ := runConfig(t, path, "schema"); code != 0 || out != string(config.Schema()) {
		t.Errorf("schema: code %d", code)
	}
	if code, _, _ := runConfig(t, path, "nope"); code != 2 {
		t.Errorf("unknown: code %d", code)
	}
	if code, _, _ := runConfig(t, path); code != 2 {
		t.Errorf("no command: code %d", code)
	}
}
