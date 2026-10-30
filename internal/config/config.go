// Package config reads gh-triage's config file, writes it out with every
// setting at its default on first run, and reports each mistake where it is
// in the file.
//
// It returns plain values: the packages that act on a setting check what
// only they know (a log level, a key) and report through Problem, so
// config needs none of them.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/pelletier/go-toml/v2"
)

// Themes are the values of ui.theme.
var Themes = []string{"auto", "mocha", "latte"}

// File is the config file.
type File struct {
	// Sections are the tabs before the inbox, each a GitHub search. Given
	// in the file, they replace the default ones.
	Sections []Section `toml:"sections"`
	Inbox    Inbox     `toml:"inbox"`
	Log      Log       `toml:"log"`
	UI       UI        `toml:"ui"`
	Refresh  Refresh   `toml:"refresh"`
	Preview  Preview   `toml:"preview"`
	Diff     Diff      `toml:"diff"`
	Merge    Merge     `toml:"merge"`
	Bulk     Bulk      `toml:"bulk"`
	// Keys is [keys] as written; the keymap package checks it.
	Keys Keys `toml:"-"`
}

// Section is a tab of pull requests found by a GitHub search.
type Section struct {
	Name  string `toml:"name"`
	Query string `toml:"query"`
	// Group is "requested" to split rows into you, team and bots, or "".
	Group string `toml:"group"`
	// Folded are the groups folded at start.
	Folded []string `toml:"folded"`
	// Sort is "updated" or "drafts_last".
	Sort string `toml:"sort"`
	// Limit is how many pull requests to read.
	Limit int `toml:"limit"`
}

// Inbox configures the notifications tab.
type Inbox struct {
	// HideClosed hides notifications whose pull request is merged or
	// closed until they are shown with a key.
	HideClosed bool `toml:"hide_closed"`
	// DoneAfterReview marks a pull request's notification done after a
	// review is sent from gh-triage.
	DoneAfterReview bool `toml:"done_after_review"`
	// Limit is how many notifications to read.
	Limit int `toml:"limit"`
}

// Preview configures the pane under the list.
type Preview struct {
	// Height is the pane's lines; 0 hides it until it is toggled.
	Height int `toml:"height"`
}

// Diff configures how diffs are shown.
type Diff struct {
	// Collapse are glob patterns of files shown folded to one line.
	Collapse []string `toml:"collapse"`
}

// Merge configures merging.
type Merge struct {
	// Methods is the order of preference among the methods a repository
	// allows: "merge", "squash", "rebase".
	Methods []string `toml:"methods"`
	// AutoMerge uses GitHub's auto-merge where the repository allows it.
	AutoMerge bool `toml:"auto_merge"`
	// ChecksTimeout, BlockedTimeout and WatchTimeout are Go durations.
	ChecksTimeout  string `toml:"checks_timeout"`
	BlockedTimeout string `toml:"blocked_timeout"`
	WatchTimeout   string `toml:"watch_timeout"`
	// UpdateBranch is who gets their branch updated when behind: "humans",
	// "all" or "never".
	UpdateBranch string `toml:"update_branch"`
}

// Bulk configures approving and merging many at once.
type Bulk struct {
	// AttentionPaths are glob patterns of files that put a mark on a pull
	// request in the confirm popup.
	AttentionPaths []string `toml:"attention_paths"`
}

// Merge values.
var (
	MergeMethods       = []string{"merge", "squash", "rebase"}
	UpdateBranchValues = []string{"humans", "all", "never"}
)

// Duration reads a duration setting Load has checked.
func Duration(s string) time.Duration {
	d, _ := time.ParseDuration(s)
	return d
}

// Section groupings, sorts and groups.
var (
	SectionGroups = []string{"", "requested"}
	SectionSorts  = []string{"updated", "drafts_last"}
	GroupNames    = []string{"you", "team", "bots"}
)

// Log configures the log file.
type Log struct {
	// Level is debug, info, warn or error; --debug and GH_TRIAGE_LOG_LEVEL
	// take precedence.
	Level string `toml:"level"`
}

// UI configures how the TUI looks.
type UI struct {
	// Theme is one of Themes.
	Theme string `toml:"theme"`
	// HomeOwners are owners whose name is left out of the PR column.
	HomeOwners []string `toml:"home_owners"`
	// StaleDays is the age, in days, past which AGE is marked.
	StaleDays int `toml:"stale_days"`
	// ApprovalChecks are glob patterns of checks that pass only after an
	// approval, such as a policy bot's; while only they are pending, a pull
	// request is shown as waiting for an approval, and counts as passing.
	ApprovalChecks []string `toml:"approval_checks"`
}

// Refresh configures the automatic refresh of the shown tab.
type Refresh struct {
	// Interval is a Go duration, or "0" to refresh only with R.
	Interval string `toml:"interval"`
}

// RefreshInterval is Refresh.Interval as a duration; zero turns automatic
// refresh off. Load has checked that it parses.
func (f File) RefreshInterval() time.Duration {
	d, _ := time.ParseDuration(f.Refresh.Interval)
	return d
}

// Keys are the actions under [keys], by name.
type Keys map[string]KeyList

// KeyList is an action's keys as written: a key or a list of keys. A value
// of another type is kept as an error, for the caller to report with the
// other mistakes under [keys].
type KeyList struct {
	Keys []string
	Err  error
}

// Default is the config with every setting at its default.
func Default() File {
	return File{
		Sections: []Section{
			{Name: "review", Query: "is:open is:pr review-requested:@me archived:false", Group: "requested", Folded: []string{"team", "bots"}, Sort: "updated", Limit: 100},
			{Name: "mine", Query: "is:open is:pr author:@me archived:false", Sort: "drafts_last", Limit: 100},
			{Name: "involved", Query: "is:open is:pr involves:@me -author:@me -review-requested:@me archived:false", Sort: "updated", Limit: 100},
		},
		Inbox:   Inbox{HideClosed: true, DoneAfterReview: true, Limit: 100},
		Log:     Log{Level: "info"},
		UI:      UI{Theme: "auto", StaleDays: 30},
		Refresh: Refresh{Interval: "3m"},
		Preview: Preview{Height: 14},
		Diff:    Diff{Collapse: []string{"*.lock", "go.sum", "pnpm-lock.yaml", "package-lock.json", "yarn.lock"}},
		Merge: Merge{Methods: []string{"merge", "squash", "rebase"}, AutoMerge: true, ChecksTimeout: "6h",
			BlockedTimeout: "30m", WatchTimeout: "24h", UpdateBranch: "humans"},
		Bulk: Bulk{AttentionPaths: []string{".github/workflows/*", "CODEOWNERS", ".github/CODEOWNERS", "docs/CODEOWNERS"}},
	}
}

func homeDir() string {
	if h, err := os.UserHomeDir(); err == nil {
		return h
	}
	return "."
}

// FilePath is where the config file is: $XDG_CONFIG_HOME/gh-triage, or
// ~/.config/gh-triage.
func FilePath() string {
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		dir = filepath.Join(homeDir(), ".config")
	}
	return filepath.Join(dir, "gh-triage", "config.toml")
}

// Template is the config file written on first run: every setting at its
// default, commented out, so a default changed later still applies until
// the user picks a value. keys is the [keys] part, every action with its
// default keys, which the caller writes from the keymap.
func Template(keys string) string {
	return `# gh-triage settings. Uncomment a line to change it.

# The tabs before the inbox, each a GitHub search. Tabs given here replace
# all three below. group = "requested" splits a tab into requests to you,
# requests to your teams, and pull requests opened by bots; folded are the
# groups folded at start. sort is "updated" (default) or "drafts_last".
# limit is how many pull requests to read (at most 1000).
#
# [[sections]]
# name = "review"
# query = "is:open is:pr review-requested:@me archived:false"
# group = "requested"
# folded = ["team", "bots"]
#
# [[sections]]
# name = "mine"
# query = "is:open is:pr author:@me archived:false"
# sort = "drafts_last"
#
# [[sections]]
# name = "involved"
# query = "is:open is:pr involves:@me -author:@me -review-requested:@me archived:false"

[inbox]
# Hide notifications whose pull request is merged or closed until they are
# shown with a key.
# hide_closed = true
# Mark a pull request's notification done after you review it here.
# done_after_review = true
# How many notifications to read.
# limit = 100

[log]
# How much goes to the log file: "debug", "info" (default), "warn" or
# "error". --debug and GH_TRIAGE_LOG_LEVEL take precedence. gh triage log
# prints where the file is.
# level = "info"

[ui]
# Color scheme: "auto" (default) picks mocha on a dark terminal and latte on a
# light one; or "mocha", "latte".
# theme = "auto"
# Owners whose name is left out of the PR column, such as your company.
# home_owners = []
# Mark AGE when a pull request has not been updated for this many days.
# stale_days = 30
# Checks that pass only after an approval, such as a policy bot's: glob
# patterns of check names. While only they are pending, CI shows ◌, and the
# checks count as passing when deciding a merge.
# approval_checks = []

[refresh]
# Refresh the shown tab this often, as a Go duration such as "3m" or "90s";
# "0" refreshes only when R is pressed.
# interval = "3m"

[preview]
# Lines of the preview pane under the list; 0 hides it until it is shown
# with a key.
# height = 14

[diff]
# Files shown folded, as their name and line counts, until unfolded: glob
# patterns, matched against the base name unless they have a slash.
# collapse = ["*.lock", "go.sum", "pnpm-lock.yaml", "package-lock.json", "yarn.lock"]

[merge]
# The order of preference among the methods a repository allows: "merge",
# "squash", "rebase". The first one the repository allows is used.
# methods = ["merge", "squash", "rebase"]
# Use GitHub's auto-merge where the repository allows it; elsewhere
# gh-triage waits for the checks and merges when they pass. Commits pushed
# after auto-merge is turned on are merged too, as GitHub does.
# auto_merge = true
# How long to wait for required checks still running.
# checks_timeout = "6h"
# Give up on a pull request still blocked this long after its required
# checks finished, as blocked by another rule (a second approval, code
# owners).
# blocked_timeout = "30m"
# Stop watching a pull request put on auto-merge after this long.
# watch_timeout = "24h"
# Whose branch is updated when it is behind its base: "humans" (default),
# "all" or "never". Bots rebase their own branches, and some (renovate) stop
# updating a branch someone else committed to.
# update_branch = "humans"

[bulk]
# Pull requests changing files that match these glob patterns get a mark in
# the confirm popup.
# attention_paths = [".github/workflows/*", "CODEOWNERS", ".github/CODEOWNERS", "docs/CODEOWNERS"]

[keys]
# Which keys do what, by action: a key or a list of keys, replacing the
# action's own, or [] to turn it off. A key given to an action is taken from
# an action that has it by default where they meet. ctrl+c and esc are fixed.
# Keys are written as key presses are read: "a", "A", "enter", "space",
# "ctrl+s", "shift+tab".
#
` + keys
}

// WriteTemplate writes Template(keys) to path unless a file is already
// there.
func WriteTemplate(path, keys string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.WriteString(Template(keys)); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// Load reads the config file at path. A missing file yields the defaults;
// keys left out of the file keep their defaults. A key it does not know is
// an error, since it would otherwise be ignored without a word.
func Load(path string) (File, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return Default(), nil
		}
		return File{}, fmt.Errorf("read %s: %w", path, err)
	}
	cfg := Default()
	// [keys] is read as it is and checked by the keymap.
	doc := struct {
		Sections []Section      `toml:"sections"`
		Inbox    Inbox          `toml:"inbox"`
		Log      Log            `toml:"log"`
		UI       UI             `toml:"ui"`
		Refresh  Refresh        `toml:"refresh"`
		Preview  Preview        `toml:"preview"`
		Diff     Diff           `toml:"diff"`
		Merge    Merge          `toml:"merge"`
		Bulk     Bulk           `toml:"bulk"`
		Keys     map[string]any `toml:"keys"`
	}{Inbox: cfg.Inbox, Log: cfg.Log, UI: cfg.UI, Refresh: cfg.Refresh, Preview: cfg.Preview, Diff: cfg.Diff, Merge: cfg.Merge, Bulk: cfg.Bulk}
	if err := toml.NewDecoder(bytes.NewReader(src)).DisallowUnknownFields().Decode(&doc); err != nil {
		return File{}, Report(path, decodeProblems(err))
	}
	sections := cfg.Sections
	if doc.Sections != nil {
		sections = doc.Sections
	}
	cfg = File{Sections: sections, Inbox: doc.Inbox, Log: doc.Log, UI: doc.UI, Refresh: doc.Refresh,
		Preview: doc.Preview, Diff: doc.Diff, Merge: doc.Merge, Bulk: doc.Bulk, Keys: keysFrom(doc.Keys)}
	ps := checkSections(cfg.Sections)
	if !slices.Contains(Themes, cfg.UI.Theme) {
		ps = append(ps, problem("ui.theme", "ui.theme must be one of %s, got %q", quoted(Themes), cfg.UI.Theme))
	}
	if d, err := time.ParseDuration(cfg.Refresh.Interval); err != nil || d < 0 {
		ps = append(ps, problem("refresh.interval", `refresh.interval must be a duration such as "3m", or "0", got %q`, cfg.Refresh.Interval))
	}
	if cfg.UI.StaleDays < 0 {
		ps = append(ps, problem("ui.stale_days", "ui.stale_days must be 0 or more, got %d", cfg.UI.StaleDays))
	}
	if cfg.Preview.Height < 0 {
		ps = append(ps, problem("preview.height", "preview.height must be 0 or more, got %d", cfg.Preview.Height))
	}
	if len(cfg.Merge.Methods) == 0 {
		ps = append(ps, problem("merge.methods", "merge.methods must name at least one of %s", quoted(MergeMethods)))
	}
	for i, m := range cfg.Merge.Methods {
		if !slices.Contains(MergeMethods, m) {
			ps = append(ps, Problem{Key: []string{"merge", "methods"}, Index: i, Message: fmt.Sprintf("merge.methods[%d] must be one of %s, got %q", i, quoted(MergeMethods), m)})
		}
	}
	for _, d := range []struct{ key, val string }{
		{"merge.checks_timeout", cfg.Merge.ChecksTimeout}, {"merge.blocked_timeout", cfg.Merge.BlockedTimeout}, {"merge.watch_timeout", cfg.Merge.WatchTimeout},
	} {
		if v, err := time.ParseDuration(d.val); err != nil || v <= 0 {
			ps = append(ps, problem(d.key, `%s must be a duration such as "30m", got %q`, d.key, d.val))
		}
	}
	if !slices.Contains(UpdateBranchValues, cfg.Merge.UpdateBranch) {
		ps = append(ps, problem("merge.update_branch", "merge.update_branch must be one of %s, got %q", quoted(UpdateBranchValues), cfg.Merge.UpdateBranch))
	}
	if cfg.Inbox.Limit < 1 || cfg.Inbox.Limit > 1000 {
		ps = append(ps, problem("inbox.limit", "inbox.limit must be between 1 and 1000, got %d", cfg.Inbox.Limit))
	}
	if len(ps) > 0 {
		return File{}, Report(path, ps)
	}
	return cfg, nil
}

// checkSections reports what is wrong with the tabs, and fills in the
// defaults of the keys left out of each.
func checkSections(ss []Section) Problems {
	var ps Problems
	bad := func(i int, format string, args ...any) {
		ps = append(ps, Problem{Key: []string{"sections"}, Index: -1, Message: fmt.Sprintf("sections[%d]: ", i) + fmt.Sprintf(format, args...)})
	}
	if len(ss) == 0 {
		ps = append(ps, Problem{Key: []string{"sections"}, Index: -1, Message: "sections: at least one tab is needed"})
	}
	seen := map[string]bool{"inbox": true}
	for i := range ss {
		s := &ss[i]
		if s.Sort == "" {
			s.Sort = "updated"
		}
		if s.Limit == 0 {
			s.Limit = 100
		}
		switch {
		case strings.TrimSpace(s.Name) == "":
			bad(i, "name must not be empty")
		case seen[s.Name]:
			bad(i, "name %q is used twice (inbox is taken)", s.Name)
		}
		seen[s.Name] = true
		if strings.TrimSpace(s.Query) == "" {
			bad(i, "query must not be empty")
		}
		if !slices.Contains(SectionGroups, s.Group) {
			bad(i, `group must be "requested" or left out, got %q`, s.Group)
		}
		if !slices.Contains(SectionSorts, s.Sort) {
			bad(i, "sort must be one of %s, got %q", quoted(SectionSorts), s.Sort)
		}
		for _, g := range s.Folded {
			if !slices.Contains(GroupNames, g) {
				bad(i, "folded must name groups among %s, got %q", quoted(GroupNames), g)
			}
		}
		if s.Limit < 1 || s.Limit > 1000 {
			bad(i, "limit must be between 1 and 1000, got %d", s.Limit)
		}
	}
	return ps
}

// keysFrom reads [keys] as decoded.
func keysFrom(t map[string]any) Keys {
	if len(t) == 0 {
		return nil
	}
	k := Keys{}
	for name, v := range t {
		k[name] = keyListFrom(v)
	}
	return k
}

// keyListFrom takes a string or an array of strings.
func keyListFrom(v any) KeyList {
	bad := KeyList{Err: errors.New("must be a key or a list of keys, as strings")}
	switch v := v.(type) {
	case string:
		return KeyList{Keys: []string{v}}
	case []any:
		keys := []string{}
		for _, e := range v {
			s, ok := e.(string)
			if !ok {
				return bad
			}
			keys = append(keys, s)
		}
		return KeyList{Keys: keys}
	}
	return bad
}

// problem is a mistake in the value of setting (table.key).
func problem(setting, format string, args ...any) Problem {
	return Problem{Key: strings.Split(setting, "."), Index: -1, Message: fmt.Sprintf(format, args...)}
}

// SettingProblem is a mistake in the value of setting, written as
// table.key, for a caller that checks a value config does not.
func SettingProblem(setting, format string, args ...any) Problem {
	return problem(setting, format, args...)
}

func quoted(ss []string) string {
	q := make([]string, len(ss))
	for i, s := range ss {
		q[i] = fmt.Sprintf("%q", s)
	}
	return strings.Join(q, ", ")
}

// decodeProblems turns what the decoder found wrong into problems: keys
// that are no setting, or a file that does not parse or has a value of the
// wrong type, where the decoder says it is.
func decodeProblems(err error) Problems {
	var strict *toml.StrictMissingError
	if errors.As(err, &strict) {
		var ps Problems
		for _, e := range strict.Errors {
			key := []string(e.Key())
			ps = append(ps, Problem{Key: key, Index: -1, AtKey: true, Message: fmt.Sprintf("%q is not a setting", strings.Join(key, "."))})
		}
		return ps
	}
	var de *toml.DecodeError
	if errors.As(err, &de) {
		msg := strings.TrimPrefix(de.Error(), "toml: ")
		if m := wrongType.FindStringSubmatch(msg); m != nil && len(de.Key()) > 0 {
			setting := strings.Join(de.Key(), ".")
			return Problems{problem(setting, "%s must be %s, got %s", setting, goTypeWords(m[2]), m[1])}
		}
		line, col := de.Position()
		return Problems{{Line: line, Col: col, Index: -1, Message: msg}}
	}
	return Problems{{Index: -1, Message: err.Error()}}
}

// wrongType is how the decoder says a value has the wrong type.
var wrongType = regexp.MustCompile(`^cannot decode TOML (\w+) into struct field \S+ of type (\S+)$`)

// goTypeWords says what a setting's Go type takes.
func goTypeWords(t string) string {
	switch {
	case t == "bool":
		return "true or false"
	case t == "string":
		return "a string"
	case strings.HasPrefix(t, "int"), strings.HasPrefix(t, "uint"), strings.HasPrefix(t, "float"):
		return "a number"
	}
	return t
}
