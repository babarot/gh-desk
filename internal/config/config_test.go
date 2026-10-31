package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func write(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadMissingFileIsDefault(t *testing.T) {
	cfg, err := Load(filepath.Join(t.TempDir(), "none.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.UI.Theme != "auto" || cfg.RefreshInterval() != 3*time.Minute || cfg.Log.Level != "info" {
		t.Errorf("cfg = %+v", cfg)
	}
}

func TestTemplateLoadsAsDefault(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gh-triage", "config.toml")
	if err := WriteTemplate(path, "# approve = \"A\"\n"); err != nil {
		t.Fatal(err)
	}
	if err := WriteTemplate(path, ""); err == nil {
		t.Error("WriteTemplate overwrote an existing file")
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(cfg, Default()) {
		t.Errorf("cfg = %+v\nwant %+v", cfg, Default())
	}
}

func TestLoadValues(t *testing.T) {
	cfg, err := Load(write(t, `
[log]
level = "debug"
[ui]
theme = "latte"
[refresh]
interval = "0"
[preview]
tab = "diff"
description = "plain"
[keys]
approve = "a"
merge = ["m", "M"]
claude = 1
[claude]
language = "Japanese"
`))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Log.Level != "debug" || cfg.UI.Theme != "latte" || cfg.RefreshInterval() != 0 || cfg.Claude.Language != "Japanese" ||
		cfg.Preview.Tab != "diff" || cfg.Preview.Description != "plain" {
		t.Errorf("cfg = %+v", cfg)
	}
	if got := cfg.Keys["approve"].Keys; len(got) != 1 || got[0] != "a" {
		t.Errorf("approve = %v", got)
	}
	if got := cfg.Keys["merge"].Keys; len(got) != 2 {
		t.Errorf("merge = %v", got)
	}
	if cfg.Keys["claude"].Err == nil {
		t.Error("claude = 1 should be kept as an error")
	}
}

func TestLoadReportsWhereTheMistakeIs(t *testing.T) {
	tests := []struct {
		name, body, want string
	}{
		{"unknown key", "[ui]\nthme = \"auto\"\n", `config.toml:2:1: "ui.thme" is not a setting`},
		{"bad theme", "[ui]\ntheme = \"dark\"\n", `config.toml:2:9: ui.theme must be one of "auto", "mocha", "latte", got "dark"`},
		{"bad interval", "[refresh]\ninterval = \"soon\"\n", `config.toml:2:12: refresh.interval must be a duration`},
		{"bad description", "[preview]\ndescription = \"markdown\"\n", `config.toml:2:15: preview.description must be one of "rendered", "plain", got "markdown"`},
		{"moved setting", "[claude]\nmodel = \"opus\"\ncache_dir = \"~/cache\"\n", `config.toml:3:1: "claude.cache_dir" moved to "cache.dir"`},
		{"moved setting, dotted", "claude.cache_dir = \"~/cache\"\n", `config.toml:1:8: "claude.cache_dir" moved to "cache.dir"`},
		{"empty cache dir", "[cache]\ndir = \" \"\n", `config.toml:2:7: cache.dir must not be empty`},
		{"wrong type", "[ui]\ntheme = 3\n", "ui.theme must be a string, got integer"},
		{"does not parse", "[ui\n", "config.toml:1:"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Load(write(t, tt.body))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("err = %v; want one containing %q", err, tt.want)
			}
		})
	}
}

func TestReportPlacesKeyProblems(t *testing.T) {
	path := write(t, "[keys]\napprove = [\"x\", \"shift+a\"]\n")
	err := Report(path, Problems{{Key: []string{"keys", "approve"}, Index: 1, Message: "never read"}})
	if !strings.Contains(err.Error(), "config.toml:2:17: never read") || !strings.Contains(err.Error(), "^~~~~~~~~") {
		t.Errorf("report = %v", err)
	}
}

func TestSections(t *testing.T) {
	cfg, err := Load(write(t, `
[[sections]]
name = "bots"
query = "is:open is:pr author:app/renovate"
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Sections) != 1 || cfg.Sections[0].Sort != "updated" || cfg.Sections[0].Limit != 100 {
		t.Errorf("sections = %+v", cfg.Sections)
	}
	_, err = Load(write(t, `
[[sections]]
name = "inbox"
query = ""
group = "teams"
folded = ["robots"]
`))
	for _, want := range []string{`name "inbox" is used twice`, "query must not be empty", `group must be "requested"`, `got "robots"`} {
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("err = %v; want one containing %q", err, want)
		}
	}
}

func TestSelectRules(t *testing.T) {
	cfg, err := Load(write(t, `
[[select]]
name = "safe renovate"
author = "renovate"
packages = ["actions/checkout"]
checks = "passing"
`))
	if err != nil || len(cfg.Select) != 1 || cfg.Select[0].Packages[0] != "actions/checkout" {
		t.Fatalf("cfg.Select = %+v, %v", cfg.Select, err)
	}
	_, err = Load(write(t, `
[[select]]
name = "approved by me"
title = "("
checks = "green"

[[select]]
name = "all"
`))
	for _, want := range []string{"is used twice", "not a regular expression", `checks must be "passing"`, "matches every pull request"} {
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("err = %v; want one containing %q", err, want)
		}
	}
}

func TestCacheDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	cfg, err := Load(write(t, "[cache]\ndir = \"~/triage\"\n"))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := cfg.CacheDir(), filepath.Join(home, "triage"); got != want {
		t.Errorf("CacheDir() = %q, want %q", got, want)
	}
}
