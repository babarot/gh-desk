// Package config reads gh-triage's config file, writes it out with every
// setting at its default on first run, and reports each mistake where it is
// in the file.
//
// It returns plain values: the packages that act on a setting check what
// only they know (a log level, a key) and report through Problem, so
// config needs none of them.
//
// The file's shape follows ADR-016: order is given by arrays, a table is
// made per feature or screen, and a value written is read as itself, never
// as "use the default".
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

// The values of the settings that take one of a few.
var (
	Themes             = []string{"auto", "mocha", "latte"}
	TabKinds           = []string{"search", "inbox"}
	TabSorts           = []string{"updated", "drafts_last"}
	GroupNames         = []string{"you", "team", "other", "bots"}
	PaneTabs           = []string{"overview", "diff"}
	DescriptionStyles  = []string{"rendered", "plain"}
	MergeMethods       = []string{"merge", "squash", "rebase"}
	UpdateBranchValues = []string{"humans", "all", "never"}
)

// MaxTabs is how many tabs there may be: the keys 1 to 9 go to them.
const MaxTabs = 9

// DefaultPreviewHeight is preview.height's default, and the pane's lines
// when it is opened with preview.height at 0 and no height kept.
const DefaultPreviewHeight = 14

// File is the config file.
type File struct {
	// Tabs are the tabs in the order shown, one of them the inbox. Given
	// in the file, they replace the default ones.
	Tabs        []Tab
	List        List
	Preview     Preview
	Description Description
	Diff        Diff
	Checks      Checks
	Review      Review
	Merge       Merge
	Bulk        Bulk
	// Select are the rules S offers besides "approved by me".
	Select []Select
	Claude Claude
	Theme  Theme
	Log    Log
	Cache  Cache
	// Keys is [keys] as written; the keymap package checks it.
	Keys Keys
}

// Tab is a tab: a GitHub search, or the inbox.
type Tab struct {
	// Kind is "search" or "inbox".
	Kind  string
	Name  string
	Query string
	// Groups are the groups a search tab is split into, in the order
	// shown; none leaves it unsplit.
	Groups []string
	// Folded are the groups folded at start.
	Folded []string
	// Sort is "updated" or "drafts_last".
	Sort string
	// Limit is how many pull requests, or notifications, to read.
	Limit int
	// ShowSettled shows the inbox's notifications whose pull request is
	// merged or closed from the start.
	ShowSettled bool
}

// Inbox reports whether t is the inbox.
func (t Tab) Inbox() bool { return t.Kind == "inbox" }

// List configures the list of pull requests.
type List struct {
	// Refresh is how often the shown tab is read again; Off reads it only
	// with R.
	Refresh Duration
	// StaleDays is the age past which AGE is marked; Off marks none.
	StaleDays Days
	// HomeOwners are owners whose name is left out of the PR column.
	HomeOwners []string
}

// Preview configures the pane under the list.
type Preview struct {
	// Height is the pane's lines; 0 hides it until it is toggled. A height
	// set with the mouse or keys is kept in the state directory and takes
	// its place.
	Height int
	// Tab is the tab the pane opens on: one of PaneTabs.
	Tab string
}

// Description configures how a pull request's description is drawn, in
// the pane and on the PR screen.
type Description struct {
	// Style is one of DescriptionStyles: "rendered" draws the Markdown,
	// "plain" shows it as written.
	Style string
}

// Diff configures how diffs are shown.
type Diff struct {
	// Collapse are glob patterns of files shown folded to one line.
	Collapse []Glob
}

// Checks configures how checks are read.
type Checks struct {
	// Approval are glob patterns of checks that pass only after an
	// approval, such as a policy bot's; while only they are pending, a pull
	// request is shown as waiting for an approval, and counts as passing.
	Approval []Glob
}

// Review configures sending a review.
type Review struct {
	// MarkDone marks a pull request's notification done after a review
	// is sent from gh-triage.
	MarkDone bool
	// Editor is the command ctrl+e runs to write a review, with its
	// arguments; "" when not set, which takes $VISUAL, then $EDITOR.
	Editor string
}

// Merge configures merging.
type Merge struct {
	// Methods is the order of preference among the methods a repository
	// allows: "merge", "squash", "rebase".
	Methods []string
	// AutoMerge uses GitHub's auto-merge where the repository allows it.
	AutoMerge bool
	// ChecksTimeout is how long to wait for required checks still running,
	// and BlockedTimeout how long a pull request may stay blocked after
	// they finished. Both count only time gh-triage is running.
	ChecksTimeout  Duration
	BlockedTimeout Duration
	// UpdateBranch is who gets their branch updated when behind: "humans",
	// "all" or "never".
	UpdateBranch string
}

// Bulk configures approving and merging many at once.
type Bulk struct {
	// AttentionPaths are glob patterns of files that put a mark on a pull
	// request in the confirm popup.
	AttentionPaths []Glob
}

// Select is a rule S offers: a pull request matches when it meets every
// key given.
type Select struct {
	Name   string
	Author string
	// Packages are the dependencies a bot's pull request may update, read
	// from its body.
	Packages []string
	// Title is a regular expression the title must match; nil for none.
	Title *regexp.Regexp
	// MaxFiles is the most files it may change; 0 for no limit.
	MaxFiles int
	Bot      bool
	// Checks is "passing" to require passing checks, or "".
	Checks string
}

// Claude configures asking Claude Code.
type Claude struct {
	Model string `toml:"model"`
	// AllowedTools are the tools Claude may use; the claude package checks
	// them.
	AllowedTools []string `toml:"allowed_tools"`
	// Language is what Claude answers in, such as "Japanese".
	Language string `toml:"language"`
	// Instructions are appended to the system prompt of every review and
	// triage.
	Instructions string `toml:"instructions"`
	// ReleaseNotesCheck lets Claude read the release notes of the bot pull
	// requests in a merge popup.
	ReleaseNotesCheck bool `toml:"release_notes_check"`
}

// Theme configures the colors.
type Theme struct {
	// Name is one of Themes.
	Name string `toml:"name"`
}

// Log configures the log file.
type Log struct {
	// Level is debug, info, warn or error; --debug and GH_TRIAGE_LOG_LEVEL
	// take precedence.
	Level string `toml:"level"`
}

// Cache configures where gh-triage keeps what it can read again.
type Cache struct {
	// Dir holds the clones and worktrees Claude reads, and the lists last
	// read; ~ is the home directory.
	Dir string `toml:"dir"`
}

// CacheDir is Cache.Dir with ~ expanded.
func (f File) CacheDir() string {
	d := f.Cache.Dir
	if d == "~" || strings.HasPrefix(d, "~/") {
		d = filepath.Join(homeDir(), d[1:])
	}
	return d
}

// RefreshInterval is list.refresh as a duration; zero turns automatic
// refresh off.
func (f File) RefreshInterval() time.Duration {
	if f.List.Refresh.Off {
		return 0
	}
	return f.List.Refresh.D
}

func defaultCacheDir() string {
	dir := os.Getenv("XDG_CACHE_HOME")
	if dir == "" {
		dir = filepath.Join(homeDir(), ".cache")
	}
	return filepath.Join(dir, "gh-triage")
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
		Tabs: []Tab{
			{Kind: "search", Name: "review", Query: "is:open is:pr review-requested:@me archived:false", Groups: slices.Clone(GroupNames),
				Folded: []string{"team", "bots"}, Sort: "updated", Limit: 100},
			{Kind: "search", Name: "mine", Query: "is:open is:pr author:@me archived:false", Sort: "drafts_last", Limit: 100},
			{Kind: "search", Name: "involved", Query: "is:open is:pr involves:@me -author:@me -review-requested:@me archived:false", Sort: "updated", Limit: 100},
			{Kind: "inbox", Name: "inbox", Limit: 100},
		},
		List:        List{Refresh: dur("3m"), StaleDays: days(30)},
		Preview:     Preview{Height: DefaultPreviewHeight, Tab: "overview"},
		Description: Description{Style: "rendered"},
		Diff:        Diff{Collapse: globs("*.lock", "go.sum", "pnpm-lock.yaml", "package-lock.json", "yarn.lock")},
		Review:      Review{MarkDone: true},
		Merge: Merge{Methods: []string{"merge", "squash", "rebase"}, AutoMerge: true, ChecksTimeout: dur("6h"),
			BlockedTimeout: dur("30m"), UpdateBranch: "humans"},
		Bulk:   Bulk{AttentionPaths: globs(".github/workflows/*", "CODEOWNERS", ".github/CODEOWNERS", "docs/CODEOWNERS")},
		Claude: Claude{Model: "opus", AllowedTools: []string{"Read", "Grep", "Glob"}, Language: "English", ReleaseNotesCheck: true},
		Theme:  Theme{Name: "auto"},
		Log:    Log{Level: "info"},
		Cache:  Cache{Dir: defaultCacheDir()},
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

# The tabs, in the order shown; the keys 1 to 9 go to them. Tabs given here
# replace all four below, and one of them must be the inbox. A tab is a
# GitHub search, or with kind = "inbox", the pull requests in your
# notifications. groups splits a search tab by whom a review is requested
# from: "you", a "team", or neither ("other"), with pull requests opened by
# "bots" apart; the groups are shown in the order given, a group left out is
# not shown, and one with no rows is not shown either. folded are the groups
# folded at start. sort is "updated" (default) or "drafts_last". limit is
# how many pull requests, or notifications, to read (at most 1000; 100 by
# default). The inbox hides notifications whose pull request is merged or
# closed until they are shown with a key; show_settled = true shows them
# from the start.
#
# [[tabs]]
# name = "review"
# query = "is:open is:pr review-requested:@me archived:false"
# groups = ["you", "team", "other", "bots"]
# folded = ["team", "bots"]
#
# [[tabs]]
# name = "mine"
# query = "is:open is:pr author:@me archived:false"
# sort = "drafts_last"
#
# [[tabs]]
# name = "involved"
# query = "is:open is:pr involves:@me -author:@me -review-requested:@me archived:false"
#
# [[tabs]]
# kind = "inbox"
# name = "inbox"
# show_settled = false

[list]
# Refresh the shown tab this often, as a Go duration such as "3m" or "90s";
# "off" refreshes only when R is pressed.
# refresh = "3m"
# Mark AGE when a pull request has not been updated for this many days, or
# "off".
# stale_days = 30
# Owners whose name is left out of the PR column, such as your company.
# home_owners = []

[preview]
# Lines of the preview pane under the list; 0 hides it until it is shown
# with a key. Dragging the pane's top edge, or + and -, changes them, and
# the lines set that way are kept over this one.
# height = 14
# The tab the pane opens on: "overview" (default), the description beside
# a column of facts, or "diff".
# tab = "overview"

[description]
# How a description is drawn, in the pane and on the PR screen: "rendered"
# (default) draws headings, lists, code, quotes and tables; "plain" shows
# the Markdown as written.
# style = "rendered"

[diff]
# Files shown folded, as their name and line counts, until unfolded: glob
# patterns, matched against the base name unless they have a slash.
# collapse = ["*.lock", "go.sum", "pnpm-lock.yaml", "package-lock.json", "yarn.lock"]

[checks]
# Checks that pass only after an approval, such as a policy bot's: glob
# patterns of check names. While only they are pending, CI shows ◌, and the
# checks count as passing when deciding a merge.
# approval = []

[review]
# Mark a pull request's notification done after you review it here, from
# any tab.
# mark_done = true
# The editor ctrl+e opens to write a review, with its arguments, such as
# "code --wait". Left out, $VISUAL, then $EDITOR.
# editor = "code --wait"

[merge]
# The order of preference among the methods a repository allows: "merge",
# "squash", "rebase". The first one the repository allows is used.
# methods = ["merge", "squash", "rebase"]
# Use GitHub's auto-merge where the repository allows it; elsewhere
# gh-triage waits for the checks and merges when they pass. Commits pushed
# after auto-merge is turned on are merged too, as GitHub does.
# auto_merge = true
# Where gh-triage waits, how long to wait for required checks still
# running, and how long a pull request may stay blocked by another rule (a
# second approval, code owners) after its required checks finished. Only
# the time gh-triage is running counts.
# checks_timeout = "6h"
# blocked_timeout = "30m"
# Whose branch is updated when it is behind its base: "humans" (default),
# "all" or "never". Bots rebase their own branches, and some (renovate) stop
# updating a branch someone else committed to.
# update_branch = "humans"

# Rules S offers to select pull requests, besides "approved by me" (bot pull
# requests you approved that are still open). A pull request matches when it
# meets every key given: author (a login; bots without app/), packages (the
# dependencies a renovate or dependabot pull request updates, read from its
# body; every one it updates must be listed), title (a regular expression),
# max_files (1 or more), bot = true, and checks = "passing" (passed, or only
# checks.approval pending).
#
# [[select]]
# name = "safe renovate"
# author = "renovate"
# packages = ["actions/checkout", "songmu/tagpr"]
# checks = "passing"
#
# [[select]]
# name = "lock files"
# author = "renovate"
# title = "(?i)lock file maintenance"
# checks = "passing"

[bulk]
# Pull requests changing files that match these glob patterns get a mark in
# the confirm popup.
# attention_paths = [".github/workflows/*", "CODEOWNERS", ".github/CODEOWNERS", "docs/CODEOWNERS"]

[claude]
# C asks Claude Code (claude -p, with the account you are signed in with).
# On a pull request's screen, it reviews that pull request in a worktree of
# its head. On the list, it plans what to show and select in the tab shown,
# with no tools, and answers in the language you ask in; enter applies the
# plan, and an approve or a merge in it opens the confirm popup. Claude only
# reads: no settings files or MCP servers are loaded, its file tools see
# only the worktree, and it has none that run commands.
# model = "opus"
# The tools Claude may use, among "Read", "Grep", "Glob" and "WebFetch".
# allowed_tools = ["Read", "Grep", "Glob"]
# The language Claude answers in about a pull request, such as "Japanese".
# A review drafted with D is written in the language of the pull request
# instead.
# language = "English"
# Appended to the instructions of every review and triage, such as
# "Point out changes that have no tests."
# instructions = ""
# Let Claude read the release notes of the bot pull requests in the merge
# popup, and mark those that need care.
# release_notes_check = true

[theme]
# Color scheme: "auto" (default) picks mocha on a dark terminal and latte on a
# light one; or "mocha", "latte".
# name = "auto"

[log]
# How much goes to the log file: "debug", "info" (default), "warn" or
# "error". --debug and GH_TRIAGE_LOG_LEVEL take precedence. gh triage log
# prints where the file is.
# level = "info"

[cache]
# Where gh-triage keeps its clones and the worktrees Claude reads, and the
# lists last read (lists.json, shown at once at the next launch) (~ is your
# home); by default $XDG_CACHE_HOME/gh-triage, or ~/.cache/gh-triage.
# dir = "~/.cache/gh-triage"

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

// The file as decoded. A key whose zero value could be written is a
// pointer, so that a key left out (which takes its default) is told from
// one written.
type (
	doc struct {
		Tabs        []tabDoc       `toml:"tabs"`
		List        listDoc        `toml:"list"`
		Preview     previewDoc     `toml:"preview"`
		Description descriptionDoc `toml:"description"`
		Diff        diffDoc        `toml:"diff"`
		Checks      checksDoc      `toml:"checks"`
		Review      reviewDoc      `toml:"review"`
		Merge       mergeDoc       `toml:"merge"`
		Bulk        bulkDoc        `toml:"bulk"`
		Select      []selectDoc    `toml:"select"`
		Claude      Claude         `toml:"claude"`
		Theme       Theme          `toml:"theme"`
		Log         Log            `toml:"log"`
		Cache       Cache          `toml:"cache"`
		Keys        map[string]any `toml:"keys"`
	}
	tabDoc struct {
		Kind        *string   `toml:"kind"`
		Name        *string   `toml:"name"`
		Query       *string   `toml:"query"`
		Groups      *[]string `toml:"groups"`
		Folded      *[]string `toml:"folded"`
		Sort        *string   `toml:"sort"`
		Limit       *int      `toml:"limit"`
		ShowSettled *bool     `toml:"show_settled"`
	}
	listDoc struct {
		Refresh    *Duration `toml:"refresh"`
		StaleDays  *Days     `toml:"stale_days"`
		HomeOwners *[]string `toml:"home_owners"`
	}
	previewDoc struct {
		Height *int    `toml:"height"`
		Tab    *string `toml:"tab"`
	}
	descriptionDoc struct {
		Style *string `toml:"style"`
	}
	diffDoc struct {
		Collapse *[]Glob `toml:"collapse"`
	}
	checksDoc struct {
		Approval *[]Glob `toml:"approval"`
	}
	reviewDoc struct {
		MarkDone *bool   `toml:"mark_done"`
		Editor   *string `toml:"editor"`
	}
	mergeDoc struct {
		Methods        *[]string `toml:"methods"`
		AutoMerge      *bool     `toml:"auto_merge"`
		ChecksTimeout  *Duration `toml:"checks_timeout"`
		BlockedTimeout *Duration `toml:"blocked_timeout"`
		UpdateBranch   *string   `toml:"update_branch"`
	}
	bulkDoc struct {
		AttentionPaths *[]Glob `toml:"attention_paths"`
	}
	selectDoc struct {
		Name     string   `toml:"name"`
		Author   string   `toml:"author"`
		Packages []string `toml:"packages"`
		Title    *Regexp  `toml:"title"`
		MaxFiles *int     `toml:"max_files"`
		Bot      bool     `toml:"bot"`
		Checks   *string  `toml:"checks"`
	}
)

// set puts *p in *dst when the file has it.
func set[T any](dst *T, p *T) {
	if p != nil {
		*dst = *p
	}
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
	d := doc{Claude: cfg.Claude, Theme: cfg.Theme, Log: cfg.Log, Cache: cfg.Cache}
	if err := toml.NewDecoder(bytes.NewReader(src)).DisallowUnknownFields().Decode(&d); err != nil {
		return File{}, Report(path, decodeProblems(err))
	}
	var ps Problems
	if d.Tabs != nil {
		var tps Problems
		cfg.Tabs, tps = tabsFrom(d.Tabs)
		ps = append(ps, tps...)
	}

	set(&cfg.List.Refresh, d.List.Refresh)
	set(&cfg.List.StaleDays, d.List.StaleDays)
	set(&cfg.List.HomeOwners, d.List.HomeOwners)
	set(&cfg.Preview.Height, d.Preview.Height)
	set(&cfg.Preview.Tab, d.Preview.Tab)
	set(&cfg.Description.Style, d.Description.Style)
	set(&cfg.Diff.Collapse, d.Diff.Collapse)
	set(&cfg.Checks.Approval, d.Checks.Approval)
	set(&cfg.Review.MarkDone, d.Review.MarkDone)
	set(&cfg.Review.Editor, d.Review.Editor)
	set(&cfg.Merge.Methods, d.Merge.Methods)
	set(&cfg.Merge.AutoMerge, d.Merge.AutoMerge)
	set(&cfg.Merge.ChecksTimeout, d.Merge.ChecksTimeout)
	set(&cfg.Merge.BlockedTimeout, d.Merge.BlockedTimeout)
	set(&cfg.Merge.UpdateBranch, d.Merge.UpdateBranch)
	set(&cfg.Bulk.AttentionPaths, d.Bulk.AttentionPaths)
	cfg.Claude, cfg.Theme, cfg.Log, cfg.Cache = d.Claude, d.Theme, d.Log, d.Cache
	cfg.Keys = keysFrom(d.Keys)
	var sps Problems
	cfg.Select, sps = selectFrom(d.Select)
	ps = append(ps, sps...)

	if p, bad := cfg.List.Refresh.problem("list.refresh", true); bad {
		ps = append(ps, p)
	}
	if p, bad := cfg.List.StaleDays.problem("list.stale_days"); bad {
		ps = append(ps, p)
	}
	for _, t := range []struct {
		key string
		d   Duration
	}{{"merge.checks_timeout", cfg.Merge.ChecksTimeout}, {"merge.blocked_timeout", cfg.Merge.BlockedTimeout}} {
		if p, bad := t.d.problem(t.key, false); bad {
			ps = append(ps, p)
		}
	}
	ps = append(ps, globProblems("diff.collapse", cfg.Diff.Collapse)...)
	ps = append(ps, globProblems("checks.approval", cfg.Checks.Approval)...)
	ps = append(ps, globProblems("bulk.attention_paths", cfg.Bulk.AttentionPaths)...)
	ps = append(ps, oneOf("theme.name", cfg.Theme.Name, Themes)...)
	ps = append(ps, oneOf("preview.tab", cfg.Preview.Tab, PaneTabs)...)
	ps = append(ps, oneOf("description.style", cfg.Description.Style, DescriptionStyles)...)
	ps = append(ps, oneOf("merge.update_branch", cfg.Merge.UpdateBranch, UpdateBranchValues)...)
	if cfg.Preview.Height < 0 {
		ps = append(ps, problem("preview.height", "preview.height must be 0 or more, got %d", cfg.Preview.Height))
	}
	if d.Review.Editor != nil && strings.TrimSpace(*d.Review.Editor) == "" {
		ps = append(ps, problem("review.editor", "review.editor must not be empty; leave it out to take $VISUAL, then $EDITOR"))
	}
	if len(cfg.Merge.Methods) == 0 {
		ps = append(ps, problem("merge.methods", "merge.methods must name at least one of %s", quoted(MergeMethods)))
	}
	for i, m := range cfg.Merge.Methods {
		if !slices.Contains(MergeMethods, m) {
			ps = append(ps, Problem{Key: []string{"merge", "methods"}, Index: i, Message: fmt.Sprintf("merge.methods[%d] must be one of %s, got %q", i, quoted(MergeMethods), m)})
		}
	}
	if strings.TrimSpace(cfg.Claude.Model) == "" {
		ps = append(ps, problem("claude.model", "claude.model must not be empty"))
	}
	if strings.TrimSpace(cfg.Claude.Language) == "" {
		ps = append(ps, problem("claude.language", "claude.language must not be empty"))
	}
	if strings.TrimSpace(cfg.Cache.Dir) == "" {
		ps = append(ps, problem("cache.dir", "cache.dir must not be empty"))
	}
	if len(ps) > 0 {
		return File{}, Report(path, ps)
	}
	return cfg, nil
}

// oneOf reports setting's value when it is not among values.
func oneOf(setting, v string, values []string) Problems {
	if slices.Contains(values, v) {
		return nil
	}
	return Problems{problem(setting, "%s must be one of %s, got %q", setting, quoted(values), v)}
}

// tabsFrom reads the tabs as written, filling in what each kind leaves
// out, and reports what is wrong with them.
func tabsFrom(ds []tabDoc) ([]Tab, Problems) {
	var ps Problems
	// at is a mistake in tab i: at key, or at the tab when key is "".
	at := func(i int, key, format string, args ...any) {
		k := []string{"tabs"}
		if key != "" {
			k = append(k, key)
		}
		ps = append(ps, Problem{Key: k, Element: i + 1, Index: -1, Message: fmt.Sprintf("tabs[%d]", i) + dotted(key) + " " + fmt.Sprintf(format, args...)})
	}
	if len(ds) > MaxTabs {
		ps = append(ps, Problem{Key: []string{"tabs"}, Element: MaxTabs + 1, Index: -1,
			Message: fmt.Sprintf("tabs: at most %d tabs, one for each of the keys 1 to 9; got %d", MaxTabs, len(ds))})
	}
	var tabs []Tab
	inboxes := 0
	seen := map[string]bool{}
	for i, d := range ds {
		t := Tab{Kind: "search", Sort: "updated", Limit: 100}
		set(&t.Kind, d.Kind)
		if !slices.Contains(TabKinds, t.Kind) {
			at(i, "kind", "must be one of %s, got %q", quoted(TabKinds), t.Kind)
		}
		if t.Inbox() {
			inboxes++
			t.Name, t.Sort = "inbox", ""
			for key, given := range map[string]bool{"query": d.Query != nil, "groups": d.Groups != nil, "folded": d.Folded != nil, "sort": d.Sort != nil} {
				if given {
					at(i, key, "is not read by the inbox")
				}
			}
		} else if d.ShowSettled != nil {
			at(i, "show_settled", `is read only by the inbox (kind = "inbox")`)
		}
		set(&t.Name, d.Name)
		set(&t.Query, d.Query)
		set(&t.Groups, d.Groups)
		set(&t.Folded, d.Folded)
		set(&t.Sort, d.Sort)
		set(&t.Limit, d.Limit)
		set(&t.ShowSettled, d.ShowSettled)
		switch {
		case strings.TrimSpace(t.Name) == "":
			at(i, "name", "must not be empty")
		case seen[t.Name]:
			at(i, "name", "%q is used twice", t.Name)
		}
		seen[t.Name] = true
		if !t.Inbox() {
			if d.Query == nil {
				at(i, "", "needs a query")
			} else if strings.TrimSpace(t.Query) == "" {
				at(i, "query", "must not be empty")
			}
			if !slices.Contains(TabSorts, t.Sort) {
				at(i, "sort", "must be one of %s, got %q", quoted(TabSorts), t.Sort)
			}
		}
		if d.Groups != nil && len(t.Groups) == 0 {
			at(i, "groups", "must name at least one group; leave it out to show the tab unsplit")
		}
		for j, g := range t.Groups {
			switch {
			case !slices.Contains(GroupNames, g):
				ps = append(ps, Problem{Key: []string{"tabs", "groups"}, Element: i + 1, Index: j,
					Message: fmt.Sprintf("tabs[%d].groups must name groups among %s, got %q", i, quoted(GroupNames), g)})
			case slices.Contains(t.Groups[:j], g):
				ps = append(ps, Problem{Key: []string{"tabs", "groups"}, Element: i + 1, Index: j,
					Message: fmt.Sprintf("tabs[%d].groups names %q twice", i, g)})
			}
		}
		for j, g := range t.Folded {
			if !slices.Contains(t.Groups, g) {
				ps = append(ps, Problem{Key: []string{"tabs", "folded"}, Element: i + 1, Index: j,
					Message: fmt.Sprintf("tabs[%d].folded must name groups among its groups, got %q", i, g)})
			}
		}
		if t.Limit < 1 || t.Limit > 1000 {
			at(i, "limit", "must be between 1 and 1000, got %d", t.Limit)
		}
		tabs = append(tabs, t)
	}
	if inboxes != 1 {
		ps = append(ps, Problem{Key: []string{"tabs"}, Element: 1, Index: -1,
			Message: fmt.Sprintf(`tabs: exactly one tab must be the inbox (kind = "inbox"), got %d`, inboxes)})
	}
	return tabs, ps
}

// selectFrom reads the rules S offers and reports what is wrong with
// them.
func selectFrom(ds []selectDoc) ([]Select, Problems) {
	var ps Problems
	at := func(i int, key, format string, args ...any) {
		k := []string{"select"}
		if key != "" {
			k = append(k, key)
		}
		ps = append(ps, Problem{Key: k, Element: i + 1, Index: -1, Message: fmt.Sprintf("select[%d]", i) + dotted(key) + " " + fmt.Sprintf(format, args...)})
	}
	var out []Select
	seen := map[string]bool{"approved by me": true}
	for i, d := range ds {
		s := Select{Name: d.Name, Author: d.Author, Packages: d.Packages, Bot: d.Bot}
		switch {
		case strings.TrimSpace(s.Name) == "":
			at(i, "name", "must not be empty")
		case seen[s.Name]:
			at(i, "name", "%q is used twice (approved by me is taken)", s.Name)
		}
		seen[s.Name] = true
		if d.Title != nil {
			if d.Title.err != nil {
				at(i, "title", "is not a regular expression: %v", d.Title.err)
			}
			s.Title = d.Title.Re
		}
		if d.MaxFiles != nil {
			if *d.MaxFiles < 1 {
				at(i, "max_files", "must be 1 or more, got %d", *d.MaxFiles)
			}
			s.MaxFiles = *d.MaxFiles
		}
		if d.Checks != nil {
			if *d.Checks != "passing" {
				at(i, "checks", `must be "passing", got %q`, *d.Checks)
			}
			s.Checks = *d.Checks
		}
		if s.Author == "" && len(s.Packages) == 0 && d.Title == nil && d.MaxFiles == nil && !s.Bot && d.Checks == nil {
			at(i, "", "%q matches every pull request; give it at least one condition", s.Name)
		}
		out = append(out, s)
	}
	return out, ps
}

// dotted is ".key", or "" for no key.
func dotted(key string) string {
	if key == "" {
		return ":"
	}
	return "." + key
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
	if strict, ok := errors.AsType[*toml.StrictMissingError](err); ok {
		var ps Problems
		told := map[string]bool{}
		for _, e := range strict.Errors {
			key := []string(e.Key())
			name := strings.Join(key, ".")
			msg := fmt.Sprintf("%q is not a setting", name)
			if to, ok := moved[name]; ok {
				// A table repeated, as [[sections]], is told once.
				if told[name] {
					continue
				}
				told[name] = true
				msg = fmt.Sprintf("%q %s", name, to)
			}
			ps = append(ps, Problem{Key: key, Index: -1, AtKey: true, Message: msg})
		}
		return ps
	}
	if de, ok := errors.AsType[*toml.DecodeError](err); ok {
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

// moved are settings and tables that were moved or renamed, by old name,
// with where they went, so a file that still has one is told at its line.
// The old name is not read. The decoder names a table it does not know,
// not each key in it.
var moved = map[string]string{
	"claude.cache_dir":    `moved to "cache.dir"`,
	"sections":            `moved to "tabs"; group = "requested" is groups = ["you", "team", "other", "bots"]`,
	"inbox":               `moved: the inbox is a [[tabs]] element with kind = "inbox", holding limit, and show_settled for hide_closed with the opposite value; done_after_review is review.mark_done`,
	"ui":                  `moved: theme is theme.name; stale_days and home_owners are under [list]; editor is review.editor; approval_checks is checks.approval`,
	"refresh":             `moved: refresh.interval is list.refresh, and "0" is "off"`,
	"preview.description": `moved to "description.style"`,
	"merge.watch_timeout": `is no longer a setting: gh-triage watches a pull request on auto-merge for 24h, and GitHub's auto-merge goes on after`,
}

// wrongType is how the decoder says a value has the wrong type.
var wrongType = regexp.MustCompile(`^cannot decode TOML (\w+) into struct field \S+ of type (\S+)$`)

// goTypeWords says what a setting's Go type takes.
func goTypeWords(t string) string {
	t = strings.TrimPrefix(t, "*")
	switch {
	case t == "bool":
		return "true or false"
	case t == "string":
		return "a string"
	case strings.HasPrefix(t, "int"), strings.HasPrefix(t, "uint"), strings.HasPrefix(t, "float"):
		return "a number"
	case strings.HasPrefix(t, "[]"):
		return "a list"
	}
	return t
}
