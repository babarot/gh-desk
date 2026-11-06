package app

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/babarot/gh-desk/internal/config"
	"github.com/babarot/gh-desk/internal/release"
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

func TestConfigReference(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", dir)
	// The config file is not read, so a broken one does not matter.
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte("not toml ["), 0o644); err != nil {
		t.Fatal(err)
	}
	code, all, _ := runConfig(t, path, "reference")
	if code != 0 || !strings.Contains(all, "# refresh = ") || !strings.Contains(all, "# approve = ") || strings.Contains(all, "New since") {
		t.Fatalf("reference: code %d\n%s", code, all)
	}
	for _, tt := range []struct{ name, want string }{
		{"merge", "[merge]\n"},
		{"claude", "[claude.mcp]\n"},
		{"list.refresh", "[list]\n# Refresh the shown tab"},
		{"select", "# [[select]]\n# name = \"small\""},
		{"select.max_lines", "[[select]]\n# The most lines"},
		{"exclude", "# [[exclude]]\n"},
		{"tabs", "# [[tabs]]\n"},
		{"keys", "[keys]\n"},
		{"keys.approve", "[keys]\n# approve = \"A\"\n"},
	} {
		if code, out, _ := runConfig(t, path, "reference", tt.name); code != 0 || !strings.Contains(out, tt.want) {
			t.Errorf("reference %s: code %d\n%s", tt.name, code, out)
		}
	}
	if code, _, errb := runConfig(t, path, "reference", "lst"); code != 1 || !strings.Contains(errb, `did you mean "list"?`) {
		t.Errorf("unknown name: code %d, err %q", code, errb)
	}
	if code, _, _ := runConfig(t, path, "reference", "a", "b"); code != 2 {
		t.Errorf("two names: code %d", code)
	}

	// The settings an update found are marked, and those gone are left out.
	if err := release.Open(filepath.Join(dir, "gh-desk")).Save(release.Record{New: []string{"list.bell", "gone.key"}, NewSince: "0.9.0"}); err != nil {
		t.Fatal(err)
	}
	_, all, _ = runConfig(t, path, "reference")
	if !strings.HasPrefix(all, "# New since v0.9.0: list.bell\n\n") || !strings.Contains(all, "# New since v0.9.0.\n# bell = ") {
		t.Errorf("marks:\n%s", all[:200])
	}
	if strings.Count(all, "New since") != 2 {
		t.Errorf("marks %d times", strings.Count(all, "New since"))
	}
	if _, out, _ := runConfig(t, path, "reference", "list.bell"); !strings.Contains(out, "# New since v0.9.0.\n") {
		t.Errorf("list.bell:\n%s", out)
	}
	if _, out, _ := runConfig(t, path, "reference", "list.refresh"); strings.Contains(out, "New since") {
		t.Errorf("list.refresh:\n%s", out)
	}

	// A record that cannot be read marks nothing.
	if err := os.WriteFile(filepath.Join(dir, "gh-desk", "release.json"), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if code, out, _ := runConfig(t, path, "reference"); code != 0 || strings.Contains(out, "New since") {
		t.Errorf("unreadable record: code %d", code)
	}
}

func TestConfigShowDiffIsAFileOfItsOwn(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(config.Template()+`
[list]
refresh = "1m"
home_owners = ["babarot"]

[merge]
auto_merge = false

[keys]
approve = "ctrl+a"
`), 0o644); err != nil {
		t.Fatal(err)
	}
	code, diff, errb := runConfig(t, path, "show", "--diff")
	if code != 0 || !strings.Contains(diff, `refresh = "1m"`) || !strings.Contains(diff, `approve = "ctrl+a"`) {
		t.Fatalf("show --diff: code %d, err %q\n%s", code, errb, diff)
	}
	short := filepath.Join(t.TempDir(), "new.toml")
	if err := os.WriteFile(short, []byte(diff), 0o644); err != nil {
		t.Fatal(err)
	}
	_, before, _ := runConfig(t, path, "show")
	_, after, _ := runConfig(t, short, "show")
	if before != after {
		t.Errorf("settings changed:\n%s\n---\n%s", before, after)
	}
}

func TestConfigInitEditSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gh-desk", "config.toml")
	if code, out, _ := runConfig(t, path, "init"); code != 0 || !strings.Contains(out, "wrote") {
		t.Fatalf("init: code %d, out %q", code, out)
	}
	if b, err := os.ReadFile(path); err != nil || strings.Count(string(b), "\n") > 40 {
		t.Errorf("the file written is %d bytes, err %v", len(b), err)
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
