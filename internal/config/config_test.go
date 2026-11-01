package config

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
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

// wantErrs loads body and checks that the error has each of wants.
func wantErrs(t *testing.T, body string, wants ...string) {
	t.Helper()
	_, _, err := Load(write(t, body))
	for _, want := range wants {
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("err = %v\nwant one containing %q", err, want)
		}
	}
}

func TestLoadMissingFileIsDefault(t *testing.T) {
	cfg, _, err := Load(filepath.Join(t.TempDir(), "none.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Theme.Dark != "default" || cfg.Theme.Light != "latte" || cfg.RefreshInterval() != 3*time.Minute || cfg.Log.Level != "info" {
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
	cfg, _, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(cfg, Default()) {
		t.Errorf("cfg = %+v\nwant %+v", cfg, Default())
	}
}

// TestTemplateTabsAreTheDefault uncomments the tabs the template shows and
// reads them as the default tabs.
func TestTemplateTabsAreTheDefault(t *testing.T) {
	var b strings.Builder
	in := false
	for _, line := range strings.Split(Template(""), "\n") {
		switch {
		case line == "# [[tabs]]":
			in = true
		case in && line == "":
			in = false
		}
		if in {
			b.WriteString(strings.TrimPrefix(strings.TrimPrefix(line, "#"), " ") + "\n")
		}
	}
	cfg, _, err := Load(write(t, b.String()))
	if err != nil {
		t.Fatalf("%v\n%s", err, b.String())
	}
	if !reflect.DeepEqual(cfg.Tabs, Default().Tabs) {
		t.Errorf("tabs = %+v\nwant %+v", cfg.Tabs, Default().Tabs)
	}
}

func TestLoadValues(t *testing.T) {
	cfg, _, err := Load(write(t, `
[log]
level = "debug"
[theme]
light = "github-light"
[list]
refresh = "off"
stale_days = "off"
[preview]
tab = "diff"
[description]
style = "plain"
[review]
editor = "code --wait"
mark_done = false
[merge]
checks_timeout = "1h30m"
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
	if cfg.Log.Level != "debug" || cfg.Theme.Light != "github-light" || cfg.RefreshInterval() != 0 || !cfg.List.StaleDays.Off ||
		cfg.Claude.Language != "Japanese" || cfg.Preview.Tab != "diff" || cfg.Description.Style != "plain" ||
		cfg.Review.Editor != "code --wait" || cfg.Review.MarkDone || cfg.Merge.ChecksTimeout.D != 90*time.Minute {
		t.Errorf("cfg = %+v", cfg)
	}
	if cfg.Merge.BlockedTimeout.D != 30*time.Minute {
		t.Errorf("blocked_timeout left out = %v, want the default", cfg.Merge.BlockedTimeout.D)
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
		{"unknown key", "[list]\nstal_days = 3\n", `config.toml:2:1: "list.stal_days" is not a setting`},
		{"bad theme", "[theme]\nlight = \"dark\"\n", `config.toml:2:9: theme.light must be one of "default", "latte", "github-dark", "github-light", "tokyonight", "tokyonight-day", "gruvbox-dark", "gruvbox-light", "nord", got "dark"`},
		{"theme.name", "[theme]\nname = \"latte\"\n", `"theme.name" replaced by theme.dark and theme.light`},
		{"bad refresh", "[list]\nrefresh = \"soon\"\n", `config.toml:2:11: list.refresh must be a duration such as "3m", or "off", got "soon"`},
		{"refresh with no unit", "[list]\nrefresh = 180\n", `list.refresh must be a duration with its unit, such as "3m", or "off", got 180`},
		{"refresh at zero", "[list]\nrefresh = \"0\"\n", `list.refresh must be more than 0, or "off", got 0`},
		{"timeout off", "[merge]\nchecks_timeout = \"off\"\n", `merge.checks_timeout must be a duration such as "3m", got "off"`},
		{"stale days at zero", "[list]\nstale_days = 0\n", `list.stale_days must be a number of days, 1 or more, or "off", got 0`},
		{"bad glob", "[diff]\ncollapse = [\"*.lock\", \"[\"]\n", `config.toml:2:23: diff.collapse[1] is not a glob pattern: "["`},
		{"bad description", "[description]\nstyle = \"markdown\"\n", `config.toml:2:9: description.style must be one of "rendered", "plain", got "markdown"`},
		{"empty editor", "[review]\neditor = \"\"\n", "review.editor must not be empty"},
		{"moved setting", "[claude]\nmodel = \"opus\"\ncache_dir = \"~/cache\"\n", `config.toml:3:1: "claude.cache_dir" moved to "cache.dir"`},
		{"moved setting, dotted", "claude.cache_dir = \"~/cache\"\n", `config.toml:1:8: "claude.cache_dir" moved to "cache.dir"`},
		{"moved table", "[ui]\ntheme = \"auto\"\n", `config.toml:1:2: "ui" moved: theme is theme.dark and theme.light`},
		{"moved array of tables", "[[sections]]\nname = \"a\"\n", `config.toml:1:3: "sections" moved to "tabs"`},
		{"moved inbox", "[inbox]\nhide_closed = true\n", `"inbox" moved: the inbox is a [[tabs]] element`},
		{"watch timeout", "[merge]\nwatch_timeout = \"1h\"\n", `"merge.watch_timeout" is no longer a setting`},
		{"empty cache dir", "[cache]\ndir = \" \"\n", `config.toml:2:7: cache.dir must not be empty`},
		{"wrong type", "[theme]\nlight = 3\n", "theme.light must be a string, got integer"},
		{"does not parse", "[theme\n", "config.toml:1:"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := Load(write(t, tt.body))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("err = %v\nwant one containing %q", err, tt.want)
			}
		})
	}
}

func TestReportsEveryMistakeAtOnce(t *testing.T) {
	// Values checked as they are read are kept, so a second one is
	// reported as well as the first.
	wantErrs(t, "[list]\nrefresh = \"soon\"\n[merge]\nchecks_timeout = \"later\"\n",
		"config.toml:2:11: list.refresh", "config.toml:4:18: merge.checks_timeout")
}

func TestReportPlacesKeyProblems(t *testing.T) {
	path := write(t, "[keys]\napprove = [\"x\", \"shift+a\"]\n")
	err := Report(path, Problems{{Key: []string{"keys", "approve"}, Index: 1, Message: "never read"}})
	if !strings.Contains(err.Error(), "config.toml:2:17: never read") || !strings.Contains(err.Error(), "^~~~~~~~~") {
		t.Errorf("report = %v", err)
	}
}

func TestTabs(t *testing.T) {
	cfg, _, err := Load(write(t, `
[[tabs]]
kind = "inbox"
name = "通知"
show_settled = true

[[tabs]]
name = "bots"
query = "is:open is:pr author:app/renovate"
groups = ["bots", "you"]
folded = ["you"]
`))
	if err != nil {
		t.Fatal(err)
	}
	want := []Tab{
		{Kind: "inbox", Name: "通知", Limit: 100, ShowSettled: true},
		{Kind: "search", Name: "bots", Query: "is:open is:pr author:app/renovate", Groups: []string{"bots", "you"}, Folded: []string{"you"}, Sort: "updated", Limit: 100},
	}
	if !reflect.DeepEqual(cfg.Tabs, want) {
		t.Errorf("tabs = %+v\nwant %+v", cfg.Tabs, want)
	}
}

func TestTabMistakes(t *testing.T) {
	wantErrs(t, `
[[tabs]]
name = "a"
query = "is:pr"

[[tabs]]
name = "a"
query = " "
sort = "newest"
limit = 0
groups = ["robots", "you", "you"]
folded = ["team"]
show_settled = true

[[tabs]]
kind = "inbox"
query = "is:pr"
`,
		`config.toml:7:8: tabs[1].name "a" is used twice`,
		`config.toml:8:9: tabs[1].query must not be empty`,
		`config.toml:9:8: tabs[1].sort must be one of "updated", "drafts_last", got "newest"`,
		`config.toml:10:9: tabs[1].limit must be between 1 and 1000, got 0`,
		`config.toml:11:11: tabs[1].groups must name groups among`,
		`config.toml:11:28: tabs[1].groups names "you" twice`,
		`config.toml:12:11: tabs[1].folded must name groups among its groups, got "team"`,
		`config.toml:13:16: tabs[1].show_settled is read only by the inbox`,
		`config.toml:17:9: tabs[2].query is not read by the inbox`,
	)
	wantErrs(t, "[[tabs]]\nname = \"a\"\n", `config.toml:1:3: tabs[0]: needs a query`, "exactly one tab must be the inbox")
	wantErrs(t, "[[tabs]]\nkind = \"inbx\"\n", `tabs[0].kind must be one of "search", "inbox", got "inbx"`)
	wantErrs(t, "[[tabs]]\nkind = \"inbox\"\n[[tabs]]\nkind = \"inbox\"\nname = \"other\"\n", `exactly one tab must be the inbox (kind = "inbox"), got 2`)
	wantErrs(t, "[[tabs]]\nname = \"x\"\nquery = \"q\"\ngroups = []\n[[tabs]]\nkind = \"inbox\"\n", "tabs[0].groups must name at least one group")
	var b strings.Builder
	for i := range 10 {
		fmt.Fprintf(&b, "[[tabs]]\nname = \"t%d\"\nquery = \"q\"\n", i)
	}
	b.WriteString("[[tabs]]\nkind = \"inbox\"\n")
	wantErrs(t, b.String(), "tabs: at most 9 tabs")
}

func TestSelectRules(t *testing.T) {
	cfg, _, err := Load(write(t, `
[[select]]
name = "safe renovate"
author = "renovate"
packages = ["actions/checkout"]
title = "(?i)bump"
checks = "passing"
`))
	if err != nil || len(cfg.Select) != 1 || cfg.Select[0].Packages[0] != "actions/checkout" || !cfg.Select[0].Title.MatchString("Bump x") {
		t.Fatalf("cfg.Select = %+v, %v", cfg.Select, err)
	}
	wantErrs(t, `
[[select]]
name = "approved by me"
title = "("
checks = "green"
max_files = 0

[[select]]
name = "all"
`,
		`config.toml:3:8: select[0].name "approved by me" is used twice`,
		`config.toml:4:9: select[0].title is not a regular expression`,
		`config.toml:5:10: select[0].checks must be "passing"`,
		`config.toml:6:13: select[0].max_files must be 1 or more, got 0`,
		`config.toml:8:3: select[1]: "all" matches every pull request`,
	)
}

func TestCacheDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	cfg, _, err := Load(write(t, "[cache]\ndir = \"~/triage\"\n"))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := cfg.CacheDir(), filepath.Join(home, "triage"); got != want {
		t.Errorf("CacheDir() = %q, want %q", got, want)
	}
}

func TestDidYouMean(t *testing.T) {
	tests := []struct{ name, body, want string }{
		{"key", "[list]\nstal_days = 3\n", `"list.stal_days" is not a setting; did you mean "list.stale_days"?`},
		{"table", "[thme]\nlight = \"latte\"\n", `"thme" is not a setting; did you mean "theme"?`},
		{"key in a tab", "[[tabs]]\nkind = \"inbox\"\nshow_setled = true\n", `"tabs.show_setled" is not a setting; did you mean "tabs.show_settled"?`},
		{"value", "[theme]\nlight = \"lattte\"\n", `got "lattte"; did you mean "latte"?`},
		{"kind", "[[tabs]]\nkind = \"inbx\"\n", `got "inbx"; did you mean "inbox"?`},
		{"group", "[[tabs]]\nname = \"r\"\nquery = \"is:pr\"\ngroups = [\"bot\"]\n[[tabs]]\nkind = \"inbox\"\n", `got "bot"; did you mean "bots"?`},
		{"moved wins", "[ui]\ntheme = \"auto\"\n", `"ui" moved:`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) { wantErrs(t, tt.body, tt.want) })
	}
	wantNo := func(body, not string) {
		t.Helper()
		_, _, err := Load(write(t, body))
		if err == nil || strings.Contains(err.Error(), not) {
			t.Errorf("err = %v; want no %q", err, not)
		}
	}
	wantNo("[list]\nnewest = 1\n", "did you mean")
	wantNo("[ui]\ntheme = \"auto\"\n", "did you mean")
}

func TestWarnings(t *testing.T) {
	path := write(t, `
[[tabs]]
name = "a"
query = "is:open author:@me"

[[tabs]]
name = "b"
query = "is:open  author:@me"

[[tabs]]
kind = "inbox"

[checks]
approval = ["*"]

[[select]]
name = "bots"
author = "app/renovate"
`)
	cfg, warns, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Tabs) != 3 {
		t.Fatalf("tabs = %+v", cfg.Tabs)
	}
	got := strings.Join(Lines(path, warns), "\n")
	for _, want := range []string{
		"config.toml:4:9: tabs[0].query has no is:pr",
		"config.toml:8:9: tabs[1].query has no is:pr",
		"config.toml:8:9: tabs[1].query is the same as tabs[0]'s",
		`config.toml:14:13: checks.approval[0] "*" matches every check`,
		`config.toml:18:10: select[0].author "app/renovate" never matches`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("warnings =\n%s\nwant one containing %q", got, want)
		}
	}
	// With a mistake as well, the warnings are reported with it, marked.
	_, _, err = Load(write(t, "[[tabs]]\nname = \"a\"\nquery = \"author:@me\"\n[[tabs]]\nkind = \"inbox\"\n[theme]\nlight = \"x\"\n"))
	if err == nil || !strings.Contains(err.Error(), "warning: tabs[0].query has no is:pr") || !strings.Contains(err.Error(), "theme.light must be one of") {
		t.Errorf("err = %v", err)
	}
	// The default tabs have none.
	if _, warns, _ := Load(write(t, "")); len(warns) > 0 {
		t.Errorf("the defaults warn: %v", warns)
	}
}

func TestColumns(t *testing.T) {
	cfg, _, err := Load(write(t, "[list]\ncolumns = [\"pr\", \"age\", \"title\"]\n"))
	if err != nil || !slices.Equal(cfg.List.Columns, []string{"pr", "age", "title"}) {
		t.Fatalf("columns = %v, %v", cfg.List.Columns, err)
	}
	wantErrs(t, "[list]\ncolumns = [\"pr\", \"age\", \"age\", \"autor\"]\n",
		`config.toml:2:25: list.columns names "age" twice`,
		`list.columns must have "title"`,
		`list.columns[3] must be one of "pr", "title", "author", "ci", "review", "why", "size", "age", got "autor"; did you mean "author"?`)
}
