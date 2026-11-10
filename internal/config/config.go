// Package config reads gh-desk's config file, writes it out with every
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
	"maps"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/pelletier/go-toml/v2"

	"github.com/babarot/gh-desk/internal/near"
)

// The values of the settings that take one of a few.
var (
	// Themes are the themes internal/tui draws; a test in internal/app
	// keeps the two in step.
	Themes = []string{"default", "latte", "github-dark", "github-light", "tokyonight", "tokyonight-day", "gruvbox-dark", "gruvbox-light", "nord"}
	// Spinners are the spinners internal/tui draws; a test in
	// internal/app keeps the two in step.
	Spinners = []string{"dots", "dense", "bounce", "snake", "jump", "fill", "sand", "line", "arc", "quarter", "block", "star", "off"}
	TabKinds = []string{"search", "inbox"}
	TabSorts = []string{"updated"}
	// Shows are how a tab shows the pull requests of a section.
	Shows             = []string{"open", "folded", "excluded"}
	PaneTabs          = []string{"overview", "diff"}
	DescriptionStyles = []string{"rendered", "plain"}
	MergeMethods      = []string{"merge", "squash", "rebase"}
	// Columns are the list's columns, in their default order.
	Columns = []string{"pr", "title", "author", "ci", "merge", "review", "why", "size", "age"}
	// MenuActions are the actions the menu of enter offers, in their
	// default order; a test in internal/app keeps them the TUI's.
	MenuActions        = []string{"ask_claude", "browse", "review", "comment", "approve", "merge", "draft", "close", "hide"}
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
	Menu        Menu
	Preview     Preview
	Description Description
	Diff        Diff
	Checks      Checks
	Review      Review
	Merge       Merge
	Bulk        Bulk
	// Select are the rules S offers besides "approved by me".
	Select []Select
	// Exclude are the rules that leave pull requests out of every tab, one
	// of which a pull request must match to be left out.
	Exclude []Exclude
	// AutoApprove are the rules that approve the pull requests they match
	// without a confirm (ADR-093).
	AutoApprove []AutoApprove
	Claude      Claude
	Theme       Theme
	Release     Release
	Log         Log
	Cache       Cache
	// Keys is [keys] as written; the keymap package checks it.
	Keys Keys
}

// Tab is a tab: a GitHub search, or the inbox.
type Tab struct {
	// Kind is "search" or "inbox".
	Kind  string
	Name  string
	Query string
	// Sections are the parts the tab's rows are split into, in the order
	// shown; a row goes in the first whose conditions it meets. None
	// leaves it unsplit.
	Sections []Section
	// Sort is "updated".
	Sort string
	// Limit is how many pull requests, or notifications, to read.
	Limit int
	// ShowSettled has the inbox show the notifications whose pull request
	// is merged or closed, rather than leave them out with the excluded.
	ShowSettled bool
}

// Section is a part of a tab: the pull requests its conditions match that
// no section above it took. A section with no condition takes the rest.
type Section struct {
	// Name is the head the section's rows are shown under; empty for none,
	// and then it has no head.
	Name string
	// Show is "open", "folded" (its head alone, until unfolded) or
	// "excluded" (left out, as an exclusion rule leaves them).
	Show string
	Conditions
}

// Inbox reports whether t is the inbox.
func (t Tab) Inbox() bool { return t.Kind == "inbox" }

// List configures the list of pull requests.
type List struct {
	// Refresh is how often the shown tab is read again; Off reads it only
	// on a refresh (r).
	Refresh Duration
	// StaleDays is the age past which AGE is marked; Off marks none.
	StaleDays Days
	// HomeOwners are owners whose name is left out of the PR column.
	HomeOwners []string
	// Columns are the columns shown, in their order; pr and title among
	// them.
	Columns []string
	// Bell rings the terminal's bell when a notification in the inbox is
	// newly unread.
	Bell bool
}

// Menu configures the menu of enter.
type Menu struct {
	// Actions are the actions offered, in their order, among MenuActions.
	Actions []string
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

// Description configures how a pull request's description and comments
// are drawn in the pane.
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
	// WaitTimeout is how long an approval or a merge waits for the jobs
	// still at work on the head, and an admin merge also for the required
	// checks. Only the time gh-desk is running counts.
	WaitTimeout Duration
}

// Review configures sending a review.
type Review struct {
	// MarkDone marks a pull request's notification done after a review
	// is sent from gh-desk.
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
	// BlockedTimeout is how long a pull request may stay blocked after its
	// jobs finished. It counts only time gh-desk is running.
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

// MaxWithinDays is the most days the conditions on days take
// (updated_within_days and the like): a hundred years, past any pull
// request, and short of what a time.Duration holds.
const MaxWithinDays = 36500

// Select is a rule S offers: a pull request matches when it meets every
// condition given.
type Select struct {
	Name string
	Conditions
}

// Exclude is a rule that leaves the pull requests meeting every condition
// given out of every tab, until they are shown with a key.
type Exclude struct {
	// Name is the rule's name, "" when it has none and is named by its
	// conditions.
	Name string
	Conditions
}

// AutoApprove is a rule that approves the pull requests its search finds
// and that meet every condition given, without a confirm, while gh-desk
// runs (ADR-093).
type AutoApprove struct {
	// Name is the rule's name, "" when it has none and is named by its
	// conditions.
	Name string
	// Query is the GitHub search the pull requests are found by; "" for
	// the one gh-desk builds from the conditions.
	Query string
	// DryRun only tells what the rule would approve.
	DryRun bool
	Conditions
}

// Conditions are the conditions of a rule, every one of which must hold.
// A zero field sets no condition.
type Conditions struct {
	// Repos are owner/repo globs the repository must match one of.
	Repos []string
	// Authors are the logins the author must be one of; bots without app/.
	Authors []string
	// Packages are the dependencies a bot's pull request may update, read
	// from its body.
	Packages []string
	// Title is a regular expression the title must match; nil for none.
	Title *regexp.Regexp
	// MaxFiles and MaxLines are the most files it may change and lines it
	// may add and delete together; MinFiles and MinLines the least.
	MaxFiles int
	MaxLines int
	MinFiles int
	MinLines int
	// UpdatedWithinDays and CreatedWithinDays are the most days since it
	// was last updated, or created; UpdatedBeforeDays and CreatedBeforeDays
	// the least.
	UpdatedWithinDays int
	CreatedWithinDays int
	UpdatedBeforeDays int
	CreatedBeforeDays int
	// Bot asks for a bot's pull request, Mine for yours and Draft for a
	// draft; each set to false asks for the opposite, and nil does not
	// ask.
	Bot   *bool
	Mine  *bool
	Draft *bool
	// States are "open", "merged" or "closed", one of which it must be in.
	States []string
	// Checks are "passing", "failing" or "pending", one of which its
	// checks must be in.
	Checks []string
	// Decision are "approved", "changes_requested", "review_required" or
	// "none", one of which its review decision must be.
	Decision []string
	// Merge are the states of merging, among MergeStates, one of which it
	// must be in.
	Merge []string
	// Labels are labels one of which it must have.
	Labels []string
	// ApprovedByMe requires the viewer's latest review to approve it, a
	// person's pull request or a bot's; ReviewedByMe a latest review of
	// any state.
	ApprovedByMe *bool
	ReviewedByMe *bool
	// Requested are whom a review must be asked of: "me", a team as
	// "@slug", or "@*" for any team.
	Requested []string
	// Reasons are reasons of a notification, one of which the row's must
	// have.
	Reasons []string
}

// States are the values of state.
var States = []string{"open", "merged", "closed"}

// CheckStates are the values of checks.
var CheckStates = []string{"passing", "failing", "pending"}

// Decisions are the values of decision; "none" is no review decision, as
// in a repository that requires no review.
var Decisions = []string{"approved", "changes_requested", "review_required", "none"}

// MergeStates are the values of merge: whether GitHub would let a pull
// request be merged now, and if not, why.
var MergeStates = []string{"ready", "behind", "conflict", "blocked", "unknown"}

// Reasons are the reasons GitHub gives a notification, the values of
// reasons.
var Reasons = []string{"approval_requested", "assign", "author", "ci_activity", "comment", "invitation", "manual", "member_feature_requested",
	"mention", "review_requested", "security_advisory_credit", "security_alert", "state_change", "subscribed", "team_mention"}

// Claude configures asking Claude Code.
type Claude struct {
	// Command is the claude command: a name looked up on PATH, or a path;
	// ~ is the home directory.
	Command string `toml:"command"`
	Model   string `toml:"model"`
	// AllowedTools are the tools Claude may use; the app package checks
	// them against claude.Tools.
	AllowedTools []string `toml:"allowed_tools"`
	// Language is what Claude answers in, such as "Japanese".
	Language string `toml:"language"`
	// Instructions are appended to the system prompt of every review and
	// triage.
	Instructions string `toml:"instructions"`
	// ReleaseNotesCheck lets Claude read the release notes of the bot pull
	// requests in a merge popup.
	ReleaseNotesCheck bool `toml:"release_notes_check"`
	// MCP are the MCP servers of the conversation about a pull request.
	MCP MCP `toml:"mcp"`
	// Plugins are the directories of plugins whose skills the
	// conversation takes (ADR-037); ~ is the home directory.
	Plugins []string `toml:"plugins"`
}

// MCP configures the MCP servers of the conversation about a pull request.
type MCP struct {
	// Builtin is whether the conversation takes gh-desk's own server
	// (ADR-080); nil when the file does not say, which BuiltinMCP decides.
	Builtin *bool `toml:"builtin"`
	// Servers are the user's servers, by name, as Claude Code configures a
	// server, with Tools (`tools`) added (ADR-037).
	Servers map[string]map[string]any `toml:"servers"`
}

// BuiltinMCP is whether the conversation takes gh-desk's own MCP server:
// as the file says, or else when nothing could send what its tools read
// out of the conversation (ADR-080). Load reports a file that leaves it
// out where something could.
func (f File) BuiltinMCP() bool {
	if b := f.Claude.MCP.Builtin; b != nil {
		return *b
	}
	return !builtinNeedsSaying(f)
}

// builtinNeedsSaying is whether claude.mcp.builtin must be written: when
// WebFetch or a server of the user's could send what gh-desk's tools read
// out of the conversation.
func builtinNeedsSaying(f File) bool {
	return slices.Contains(f.Claude.AllowedTools, "WebFetch") || len(f.Claude.MCP.Servers) > 0
}

// PluginDirs are Claude.Plugins with ~ expanded.
func (f File) PluginDirs() []string {
	out := make([]string, len(f.Claude.Plugins))
	for i, p := range f.Claude.Plugins {
		out[i] = expandHome(p)
	}
	return out
}

// MCPTools are the tools of server that may run without a prompt, as its
// `tools` names them. Load has checked them.
func MCPTools(server map[string]any) []string {
	var out []string
	if ts, ok := server["tools"].([]any); ok {
		for _, t := range ts {
			if s, ok := t.(string); ok {
				out = append(out, s)
			}
		}
	}
	return out
}

// Theme configures the colors.
type Theme struct {
	// Dark and Light are the themes used on a dark terminal and on a
	// light one, among Themes; the same theme in both is used on any.
	Dark  string `toml:"dark"`
	Light string `toml:"light"`
	// Spinner is the spinner shown while something is read, among
	// Spinners.
	Spinner string `toml:"spinner"`
	// Shimmer runs a light over the text that says what is going on.
	Shimmer bool `toml:"shimmer"`
}

// Release configures what gh-desk tells of its own releases.
type Release struct {
	// Check reads gh-desk's releases now and then, and tells of a newer
	// one and of an update. Only a release build reads them.
	Check bool `toml:"check"`
}

// Log configures the log file.
type Log struct {
	// Level is debug, info, warn or error; --debug and GH_DESK_LOG_LEVEL
	// take precedence.
	Level string `toml:"level"`
}

// Cache configures where gh-desk keeps what it can read again.
type Cache struct {
	// Dir holds the clones and worktrees Claude reads, and the lists last
	// read; ~ is the home directory.
	Dir string `toml:"dir"`
}

// CacheDir is Cache.Dir with ~ expanded.
func (f File) CacheDir() string { return expandHome(f.Cache.Dir) }

// ClaudeCommand is Claude.Command with ~ expanded.
func (f File) ClaudeCommand() string { return expandHome(f.Claude.Command) }

// expandHome replaces a leading ~ in p with the home directory.
func expandHome(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		p = filepath.Join(homeDir(), p[1:])
	}
	return p
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
	return filepath.Join(dir, "gh-desk")
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

// defaultRules are the rules of S that a file with no [[select]] has, read
// from what the manual shows of them.
func defaultRules() []Select {
	var d struct {
		Select []selectDoc `toml:"select"`
	}
	if err := toml.Unmarshal([]byte(defaultSelect), &d); err != nil {
		panic(err)
	}
	rules, ps := selectFrom(d.Select)
	if len(ps) > 0 {
		panic(ps)
	}
	return rules
}

// reviewSections are the sections of the default review tab: whom the
// review is asked of, a bot's pull request last, as it is judged first.
func reviewSections() []Section {
	return []Section{
		{Name: "requested from you", Show: "open", Conditions: Conditions{Requested: []string{"me"}, Bot: new(false)}},
		{Name: "requested from a team", Show: "folded", Conditions: Conditions{Requested: []string{"@*"}, Bot: new(false)}},
		{Name: "other", Show: "open", Conditions: Conditions{Bot: new(false)}},
		{Name: "bots", Show: "folded", Conditions: Conditions{Bot: new(true)}},
	}
}

// mineSections are the sections of the default tab of one's own pull
// requests: the drafts after the rest, with no heads.
func mineSections() []Section {
	return []Section{
		{Show: "open", Conditions: Conditions{Draft: new(false)}},
		{Show: "open", Conditions: Conditions{Draft: new(true)}},
	}
}

// Default is the config with every setting at its default.
func Default() File {
	return File{
		Tabs: []Tab{
			{Kind: "search", Name: "review", Query: "is:open is:pr review-requested:@me archived:false", Sections: reviewSections(),
				Sort: "updated", Limit: 100},
			{Kind: "search", Name: "mine", Query: "is:open is:pr author:@me archived:false", Sections: mineSections(), Sort: "updated", Limit: 100},
			{Kind: "search", Name: "involved", Query: "is:open is:pr involves:@me -author:@me -review-requested:@me archived:false", Sort: "updated", Limit: 100},
			{Kind: "inbox", Name: "inbox", Limit: 100},
		},
		List:        List{Refresh: dur("3m"), StaleDays: days(30), Columns: slices.Clone(Columns)},
		Menu:        Menu{Actions: slices.Clone(MenuActions)},
		Preview:     Preview{Height: DefaultPreviewHeight, Tab: "overview"},
		Description: Description{Style: "rendered"},
		Diff:        Diff{Collapse: globs("*.lock", "go.sum", "pnpm-lock.yaml", "package-lock.json", "yarn.lock")},
		Checks:      Checks{WaitTimeout: dur("6h")},
		Review:      Review{MarkDone: true},
		Merge: Merge{Methods: []string{"merge", "squash", "rebase"}, AutoMerge: true,
			BlockedTimeout: dur("30m"), UpdateBranch: "humans"},
		Select:  defaultRules(),
		Bulk:    Bulk{AttentionPaths: globs(".github/workflows/*", "CODEOWNERS", ".github/CODEOWNERS", "docs/CODEOWNERS")},
		Claude:  Claude{Command: "claude", Model: "opus", AllowedTools: []string{"Read", "Grep", "Glob"}, Language: "English", ReleaseNotesCheck: true},
		Theme:   Theme{Dark: "default", Light: "latte", Spinner: "dots", Shimmer: true},
		Release: Release{Check: true},
		Log:     Log{Level: "info"},
		Cache:   Cache{Dir: defaultCacheDir()},
	}
}

func homeDir() string {
	if h, err := os.UserHomeDir(); err == nil {
		return h
	}
	return "."
}

// FilePath is where the config file is: $XDG_CONFIG_HOME/gh-desk, or
// ~/.config/gh-desk.
func FilePath() string {
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		dir = filepath.Join(homeDir(), ".config")
	}
	return filepath.Join(dir, "gh-desk", "config.toml")
}

// WriteTemplate writes Template to path unless a file is already there.
func WriteTemplate(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.WriteString(Template()); err != nil {
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
		Tabs        []tabDoc         `toml:"tabs"`
		List        listDoc          `toml:"list"`
		Menu        menuDoc          `toml:"menu"`
		Preview     previewDoc       `toml:"preview"`
		Description descriptionDoc   `toml:"description"`
		Diff        diffDoc          `toml:"diff"`
		Checks      checksDoc        `toml:"checks"`
		Review      reviewDoc        `toml:"review"`
		Merge       mergeDoc         `toml:"merge"`
		Bulk        bulkDoc          `toml:"bulk"`
		Select      []selectDoc      `toml:"select"`
		Exclude     []excludeDoc     `toml:"exclude"`
		AutoApprove []autoApproveDoc `toml:"auto_approve"`
		Claude      Claude           `toml:"claude"`
		Theme       Theme            `toml:"theme"`
		Release     Release          `toml:"release"`
		Log         Log              `toml:"log"`
		Cache       Cache            `toml:"cache"`
		Keys        map[string]any   `toml:"keys"`
	}
	tabDoc struct {
		Kind     *string       `toml:"kind"`
		Name     *string       `toml:"name"`
		Query    *string       `toml:"query"`
		Sections *[]sectionDoc `toml:"sections"`
		// Groups and Folded are read only to tell how to write them as
		// sections; they are no setting.
		Groups      *[]string `toml:"groups" removed:"true"`
		Folded      *[]string `toml:"folded" removed:"true"`
		Sort        *string   `toml:"sort"`
		Limit       *int      `toml:"limit"`
		ShowSettled *bool     `toml:"show_settled"`
	}
	sectionDoc struct {
		Name *string `toml:"name"`
		Show *string `toml:"show"`
		// Packages is read only to tell it is not a condition of a section:
		// the decoder would name a key it does not know in the tab.
		Packages *[]string `toml:"packages" removed:"true"`
		conditionDoc
	}
	listDoc struct {
		Refresh    *Duration `toml:"refresh"`
		StaleDays  *Days     `toml:"stale_days"`
		HomeOwners *[]string `toml:"home_owners"`
		Columns    *[]string `toml:"columns"`
		Bell       *bool     `toml:"bell"`
	}
	menuDoc struct {
		Actions *[]string `toml:"actions"`
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
		Approval    *[]Glob   `toml:"approval"`
		WaitTimeout *Duration `toml:"wait_timeout"`
	}
	reviewDoc struct {
		MarkDone *bool   `toml:"mark_done"`
		Editor   *string `toml:"editor"`
	}
	mergeDoc struct {
		Methods        *[]string `toml:"methods"`
		AutoMerge      *bool     `toml:"auto_merge"`
		BlockedTimeout *Duration `toml:"blocked_timeout"`
		UpdateBranch   *string   `toml:"update_branch"`
	}
	bulkDoc struct {
		AttentionPaths *[]Glob `toml:"attention_paths"`
	}
	selectDoc struct {
		Name     string   `toml:"name"`
		Packages []string `toml:"packages"`
		conditionDoc
	}
	excludeDoc struct {
		Name string `toml:"name"`
		conditionDoc
	}
	autoApproveDoc struct {
		Name     string   `toml:"name"`
		Query    *string  `toml:"query"`
		DryRun   bool     `toml:"dry_run"`
		Packages []string `toml:"packages"`
		conditionDoc
	}
	conditionDoc struct {
		Repos         []string `toml:"repos"`
		Authors       []string `toml:"authors"`
		Title         *Regexp  `toml:"title"`
		MaxFiles      *int     `toml:"max_files"`
		MaxLines      *int     `toml:"max_lines"`
		MinFiles      *int     `toml:"min_files"`
		MinLines      *int     `toml:"min_lines"`
		UpdatedIn     *int     `toml:"updated_within_days"`
		CreatedIn     *int     `toml:"created_within_days"`
		UpdatedBefore *int     `toml:"updated_before_days"`
		CreatedBefore *int     `toml:"created_before_days"`
		Bot           *bool    `toml:"bot"`
		Mine          *bool    `toml:"mine"`
		Draft         *bool    `toml:"draft"`
		States        []string `toml:"state"`
		Checks        []string `toml:"checks"`
		Decision      []string `toml:"decision"`
		Merge         []string `toml:"merge"`
		Labels        []string `toml:"labels"`
		ApprovedByMe  *bool    `toml:"approved_by_me"`
		ReviewedByMe  *bool    `toml:"reviewed_by_me"`
		Requested     []string `toml:"requested"`
		Reasons       []string `toml:"reasons"`
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
// an error, since it would otherwise be ignored without a word. The
// problems returned with a File are warnings: what is likely not what was
// meant, which does not stop gh-desk.
func Load(path string) (File, Problems, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return Default(), nil, nil
		}
		return File{}, nil, fmt.Errorf("read %s: %w", path, err)
	}
	return Parse(path, src)
}

// Parse reads src, the config file at path, as Load does. path only names
// the file in the problems reported.
func Parse(path string, src []byte) (File, Problems, error) {
	cfg := Default()
	d := doc{Claude: cfg.Claude, Theme: cfg.Theme, Release: cfg.Release, Log: cfg.Log, Cache: cfg.Cache}
	if err := toml.NewDecoder(bytes.NewReader(src)).DisallowUnknownFields().Decode(&d); err != nil {
		return File{}, nil, Report(path, decodeProblems(err))
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
	set(&cfg.List.Columns, d.List.Columns)
	set(&cfg.List.Bell, d.List.Bell)
	set(&cfg.Menu.Actions, d.Menu.Actions)
	set(&cfg.Preview.Height, d.Preview.Height)
	set(&cfg.Preview.Tab, d.Preview.Tab)
	set(&cfg.Description.Style, d.Description.Style)
	set(&cfg.Diff.Collapse, d.Diff.Collapse)
	set(&cfg.Checks.Approval, d.Checks.Approval)
	set(&cfg.Checks.WaitTimeout, d.Checks.WaitTimeout)
	set(&cfg.Review.MarkDone, d.Review.MarkDone)
	set(&cfg.Review.Editor, d.Review.Editor)
	set(&cfg.Merge.Methods, d.Merge.Methods)
	set(&cfg.Merge.AutoMerge, d.Merge.AutoMerge)
	set(&cfg.Merge.BlockedTimeout, d.Merge.BlockedTimeout)
	set(&cfg.Merge.UpdateBranch, d.Merge.UpdateBranch)
	set(&cfg.Bulk.AttentionPaths, d.Bulk.AttentionPaths)
	cfg.Claude, cfg.Theme, cfg.Release, cfg.Log, cfg.Cache = d.Claude, d.Theme, d.Release, d.Log, d.Cache
	cfg.Keys = keysFrom(d.Keys)
	var sps Problems
	// Rules written replace the default ones, as tabs do, and select = []
	// leaves none, which a file with none cannot say.
	if d.Select != nil {
		cfg.Select, sps = selectFrom(d.Select)
		ps = append(ps, sps...)
	}
	cfg.Exclude, sps = excludeFrom(d.Exclude)
	ps = append(ps, sps...)
	cfg.AutoApprove, sps = autoApproveFrom(d.AutoApprove)
	ps = append(ps, sps...)

	ps = append(ps, checkSettings(cfg)...)
	if cfg.Claude.MCP.Builtin == nil && builtinNeedsSaying(cfg) {
		// Shown at what calls for it, since the key is not in the file.
		at := Problem{Key: []string{"claude", "allowed_tools"}, Index: slices.Index(cfg.Claude.AllowedTools, "WebFetch")}
		if at.Index < 0 {
			at = Problem{Key: []string{"claude", "mcp", "servers", slices.Min(slices.Collect(maps.Keys(cfg.Claude.MCP.Servers)))}, Index: -1, AtKey: true}
		}
		ps = append(ps, Problem{Key: at.Key, Index: at.Index, AtKey: at.AtKey,
			Message: "claude.mcp.builtin must be written as true or false, since allowed_tools has WebFetch or mcp.servers has a server: what gh-desk's tools read of the repository (issues, pull requests, CI logs) could be sent out through them"})
	}
	ps = append(ps, checkColumns(cfg.List.Columns)...)
	ps = append(ps, namedTwice("menu.actions", cfg.Menu.Actions)...)
	if d.Review.Editor != nil && strings.TrimSpace(*d.Review.Editor) == "" {
		ps = append(ps, problem("review.editor", "review.editor must not be empty; leave it out to take $VISUAL, then $EDITOR"))
	}
	warns := warnings(cfg)
	if len(ps) > 0 {
		return File{}, nil, Report(path, append(ps, warns...))
	}
	return cfg, warns, nil
}

// warnings are what is likely not what was meant in cfg.
func warnings(cfg File) Problems {
	var ps Problems
	warn := func(key []string, elem int, format string, args ...any) {
		ps = append(ps, Problem{Key: key, Element: elem, Index: -1, Warn: true, Message: fmt.Sprintf(format, args...)})
	}
	queries := map[string]int{}
	for i, t := range cfg.Tabs {
		if t.Inbox() {
			continue
		}
		if !searchesPRs(t.Query) {
			warn([]string{"tabs", "query"}, i+1, "tabs[%d].query has no is:pr: GitHub's search returns issues too, which are left out but take up the limit and the count", i)
		}
		q := strings.Join(strings.Fields(t.Query), " ")
		if j, ok := queries[q]; ok {
			warn([]string{"tabs", "query"}, i+1, "tabs[%d].query is the same as tabs[%d]'s", i, j)
		} else {
			queries[q] = i
		}
	}
	for i, s := range cfg.Select {
		for j, a := range s.Authors {
			if login, ok := strings.CutPrefix(a, "app/"); ok {
				warn([]string{"select", "authors"}, i+1, "select[%d].authors[%d] %q: a bot's login is written without app/, as %q; app/ is dropped when it is compared", i, j, a, login)
			}
		}
	}
	for i, g := range cfg.Checks.Approval {
		if strings.Trim(g.Pattern, "*") == "" {
			ps = append(ps, Problem{Key: []string{"checks", "approval"}, Index: i, Warn: true,
				Message: fmt.Sprintf("checks.approval[%d] %q matches every check, so pending checks always count as passing", i, g.Pattern)})
		}
	}
	return ps
}

// searchesPRs reports whether a GitHub search query asks for pull requests
// only.
func searchesPRs(q string) bool {
	for f := range strings.FieldsSeq(strings.ToLower(q)) {
		if f == "is:pr" || f == "type:pr" || f == "is:pull-request" {
			return true
		}
	}
	return false
}

// oneOf reports setting's value when it is not among values.
func oneOf(setting, v string, values []string) Problems {
	if slices.Contains(values, v) {
		return nil
	}
	return Problems{problem(setting, "%s must be one of %s, got %q%s", setting, quoted(values), v, didYouMean(v, values))}
}

// didYouMean is "; did you mean ...?" naming the value among values that
// v is likely a typo of, or "".
func didYouMean(v string, values []string) string {
	if c, ok := near.Closest(v, values); ok {
		return fmt.Sprintf("; did you mean %q?", c)
	}
	return ""
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
			at(i, "kind", "must be one of %s, got %q%s", quoted(TabKinds), t.Kind, didYouMean(t.Kind, TabKinds))
		}
		if t.Inbox() {
			inboxes++
			t.Name, t.Sort = "inbox", ""
			for key, given := range map[string]bool{"query": d.Query != nil, "sort": d.Sort != nil} {
				if given {
					at(i, key, "is not read by the inbox")
				}
			}
		} else if d.ShowSettled != nil {
			at(i, "show_settled", `is read only by the inbox (kind = "inbox")`)
		}
		set(&t.Name, d.Name)
		set(&t.Query, d.Query)
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
			switch {
			case t.Sort == "drafts_last" && d.Groups == nil:
				at(i, "sort", `"drafts_last" was replaced by sections; write the drafts as a section after the rest:%s`, sectionsWords(groupSections(nil, nil, true)))
			case t.Sort == "drafts_last":
				at(i, "sort", `"drafts_last" was replaced by sections, which cannot also put the drafts last in each of them; see groups`)
			case !slices.Contains(TabSorts, t.Sort):
				at(i, "sort", "must be one of %s, got %q%s", quoted(TabSorts), t.Sort, didYouMean(t.Sort, TabSorts))
			}
			t.Sort = "updated"
		}
		if d.Groups != nil {
			var folded []string
			if d.Folded != nil {
				folded = *d.Folded
			}
			ss := groupSections(*d.Groups, folded, false)
			msg := "groups was replaced by sections, which say what each group was; write" + sectionsWords(ss)
			if note := groupsNote(*d.Groups); note != "" {
				msg += "\n" + note
			}
			at(i, "groups", "%s", msg)
		}
		if d.Folded != nil {
			at(i, "folded", `was replaced by show = "folded" on a section; see groups`)
		}
		if d.Sections != nil {
			t.Sections = sectionsFrom(&ps, i, t.Inbox(), *d.Sections)
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

// sectionsFrom reads the sections of the tab i and reports what is wrong
// with them.
func sectionsFrom(ps *Problems, i int, inbox bool, ds []sectionDoc) []Section {
	var out []Section
	names := map[string]bool{}
	for j, d := range ds {
		r := elemReporter{ps: ps, table: "tabs", i: i, sub: "sections", j: j}
		s := Section{Show: "open"}
		set(&s.Name, d.Name)
		set(&s.Show, d.Show)
		if !slices.Contains(Shows, s.Show) {
			r.at("show", "must be one of %s, got %q%s", quoted(Shows), s.Show, didYouMean(s.Show, Shows))
		}
		// A section with no condition takes the rest, which is what one
		// written last is for.
		s.Conditions = conditionsFrom(d.conditionDoc, nil, s.Name, r)
		switch {
		case d.Name != nil && strings.TrimSpace(s.Name) == "":
			r.at("name", "must not be empty; leave it out for a section with no head")
		case names[s.Name]:
			r.at("name", "%q is used twice in the tab", s.Name)
		case s.Name == "" && s.Show == "folded":
			r.at("show", "a section with no name has no head to unfold it by, so it cannot be folded")
		case s.Name == "" && s.Show == "excluded" && noCondition(d.conditionDoc):
			r.at("", "excludes every pull request left, with no name to say so by; give it a name or a condition")
		}
		if s.Name != "" {
			names[s.Name] = true
		}
		if d.Packages != nil {
			r.at("packages", "is not a condition of a section: it reads the body of every bot pull request in the tab")
		}
		if !inbox && d.Reasons != nil {
			r.at("reasons", `is read only in the inbox (kind = "inbox"): the rows of a search have no notification`)
		}
		out = append(out, s)
	}
	return out
}

// groupNames are what each group's head said, which its section is named.
var groupNames = map[string]string{"you": "requested from you", "team": "requested from a team", "other": "other", "bots": "bots"}

// groupSections are the sections that show what the groups did: each group
// a section, you, team and other in that order, since other was what a
// review is asked of neither of, which a condition can say only by coming
// after them; bots where it was written among them, since no other takes a
// bot's pull request, and last when it was not written. A group not
// written is excluded where it is, so that it is not shown as it was not.
// draftsLast with no groups puts the drafts after the rest.
func groupSections(groups, folded []string, draftsLast bool) []Section {
	if groups == nil {
		if !draftsLast {
			return nil
		}
		return mineSections()
	}
	cond := map[string]Conditions{
		"you":   {Requested: []string{"me"}, Bot: new(false)},
		"team":  {Requested: []string{"@*"}, Bot: new(false)},
		"other": {Bot: new(false)},
		"bots":  {Bot: new(true)},
	}
	order := []string{"you", "team", "other"}
	at := len(order)
	if i := slices.Index(groups, "bots"); i >= 0 {
		at = 0
		for _, g := range groups[:i] {
			if g != "bots" {
				at++
			}
		}
	}
	order = slices.Insert(order, at, "bots")
	var out []Section
	for _, g := range order {
		s := Section{Name: groupNames[g], Show: "open", Conditions: cond[g]}
		switch {
		case !slices.Contains(groups, g):
			s.Show = "excluded"
		case slices.Contains(folded, g):
			s.Show = "folded"
		}
		out = append(out, s)
	}
	return out
}

// groupsNote says what the sections of groups do otherwise than the
// groups did: the order of you, team and other, and the groups not shown.
func groupsNote(groups []string) string {
	var notes []string
	var people []string
	for _, g := range groups {
		if g != "bots" {
			people = append(people, g)
		}
	}
	if !slices.IsSortedFunc(people, func(a, b string) int {
		return slices.Index([]string{"you", "team", "other"}, a) - slices.Index([]string{"you", "team", "other"}, b)
	}) {
		notes = append(notes, "you, team and other come in that order: other is what neither of the others takes")
	}
	for _, g := range []string{"you", "team", "other", "bots"} {
		if !slices.Contains(groups, g) {
			notes = append(notes, `a group not written is excluded: its pull requests are counted as excluded, and a shows them`)
			break
		}
	}
	return strings.Join(notes, "; ")
}

// sectionsWords are ss as the file writes them, indented under a message.
func sectionsWords(ss []Section) string {
	var b strings.Builder
	b.WriteString("\n    sections = [\n")
	for _, s := range ss {
		b.WriteString("      " + sectionTOML(s) + ",\n")
	}
	b.WriteString("    ]")
	return b.String()
}

// selectFrom reads the rules S offers and reports what is wrong with
// them.
func selectFrom(ds []selectDoc) ([]Select, Problems) {
	var ps Problems
	var out []Select
	seen := map[string]bool{"approved by me": true}
	for i, d := range ds {
		r := elemReporter{ps: &ps, table: "select", i: i, j: -1}
		at := r.at
		switch {
		case strings.TrimSpace(d.Name) == "":
			at("name", "must not be empty")
		case seen[d.Name]:
			at("name", "%q is used twice (approved by me is taken)", d.Name)
		}
		seen[d.Name] = true
		c := conditionsFrom(d.conditionDoc, d.Packages, d.Name, r)
		reportNoCondition(d.conditionDoc, d.Packages, d.Name, r)
		// S selects open pull requests that are not drafts, and matches a
		// pull request alone, without a notification.
		if c.Draft != nil && *c.Draft {
			at("draft", "S never selects a draft, so the rule would select nothing")
		}
		if d.Reasons != nil {
			at("reasons", "is read only by [[exclude]]: S selects pull requests, which have no notification")
		}
		for j, st := range c.States {
			// A state that is none is reported once, above.
			if st != "open" && slices.Contains(States, st) {
				r.item("state", j, "%q: S selects open pull requests only, so the rule would select nothing", st)
			}
		}
		out = append(out, Select{Name: d.Name, Conditions: c})
	}
	return out, ps
}

// excludeFrom reads the rules that leave pull requests out and reports
// what is wrong with them.
func excludeFrom(ds []excludeDoc) ([]Exclude, Problems) {
	var ps Problems
	var out []Exclude
	seen := map[string]bool{}
	for i, d := range ds {
		r := elemReporter{ps: &ps, table: "exclude", i: i, j: -1}
		// A rule with no name is named by its conditions where it is
		// shown; many rules are easier to write without one. A name of
		// spaces is none.
		d.Name = strings.TrimSpace(d.Name)
		if d.Name != "" && seen[d.Name] {
			r.at("name", "%q is used twice", d.Name)
		}
		if d.Name != "" {
			seen[d.Name] = true
		}
		// packages is not a key here: the bodies are read only for S, of
		// the bot pull requests of the tab shown, and leaving pull
		// requests out by them would read every body in every tab.
		c := conditionsFrom(d.conditionDoc, nil, d.Name, r)
		reportNoCondition(d.conditionDoc, nil, d.Name, r)
		out = append(out, Exclude{Name: d.Name, Conditions: c})
	}
	return out, ps
}

// autoApproveFrom reads the rules that approve without a confirm and
// reports what is wrong with them.
func autoApproveFrom(ds []autoApproveDoc) ([]AutoApprove, Problems) {
	var ps Problems
	var out []AutoApprove
	seen := map[string]bool{}
	for i, d := range ds {
		r := elemReporter{ps: &ps, table: "auto_approve", i: i, j: -1}
		at := r.at
		d.Name = strings.TrimSpace(d.Name)
		if d.Name != "" && seen[d.Name] {
			at("name", "%q is used twice", d.Name)
		}
		if d.Name != "" {
			seen[d.Name] = true
		}
		c := conditionsFrom(d.conditionDoc, d.Packages, d.Name, r)
		// A search given says which pull requests the rule is for; without
		// one, a rule with no condition would approve every review asked
		// of the user.
		var query string
		if d.Query != nil {
			query = strings.TrimSpace(*d.Query)
			if query == "" {
				at("query", "must not be empty; leave it out for the search gh-desk builds")
			}
		} else {
			reportNoCondition(d.conditionDoc, d.Packages, d.Name, r)
		}
		// It approves open pull requests that are not drafts, and judges a
		// pull request alone, without a notification.
		if c.Draft != nil && *c.Draft {
			at("draft", "a draft is never approved by a rule, so the rule would approve nothing")
		}
		if d.Reasons != nil {
			at("reasons", "is read only by [[exclude]]: a rule approves pull requests, which have no notification")
		}
		for j, st := range c.States {
			if st != "open" && slices.Contains(States, st) {
				r.item("state", j, "%q: only open pull requests are approved, so the rule would approve nothing", st)
			}
		}
		out = append(out, AutoApprove{Name: d.Name, Query: query, DryRun: d.DryRun, Conditions: c})
	}
	return out, ps
}

// elemReporter reports problems in the i-th element of the array of
// tables table, or with sub, in the j-th inline table of its key sub, as
// the sections of a tab are.
type elemReporter struct {
	ps    *Problems
	table string
	i     int
	sub   string
	j     int
}

// at reports a problem at key, or at the element when key is "".
func (r elemReporter) at(key, format string, args ...any) {
	r.item(key, -1, format, args...)
}

// item reports a problem at the j-th value of the list at key, or at the
// whole value when j is -1.
func (r elemReporter) item(key string, j int, format string, args ...any) {
	k := []string{r.table}
	where := fmt.Sprintf("%s[%d]", r.table, r.i)
	inline := 0
	if r.sub != "" {
		k = append(k, r.sub)
		where += fmt.Sprintf(".%s[%d]", r.sub, r.j)
		inline = r.j + 1
	}
	if key != "" {
		k = append(k, key)
	}
	where += dotted(key)
	if j >= 0 {
		where += fmt.Sprintf("[%d]", j)
	}
	*r.ps = append(*r.ps, Problem{Key: k, Element: r.i + 1, Index: j, Inline: inline, Message: where + " " + fmt.Sprintf(format, args...)})
}

// conditionsFrom reads the conditions of a rule named name, reporting what
// is wrong at at.
func conditionsFrom(d conditionDoc, packages []string, name string, r elemReporter) Conditions {
	at := r.at
	c := Conditions{Repos: d.Repos, Authors: d.Authors, Packages: packages, Labels: d.Labels, Requested: d.Requested, Reasons: d.Reasons, States: d.States, Checks: d.Checks, Decision: d.Decision, Merge: d.Merge,
		Bot: d.Bot, Mine: d.Mine, Draft: d.Draft, ApprovedByMe: d.ApprovedByMe, ReviewedByMe: d.ReviewedByMe}
	for _, l := range []struct {
		key  string
		v    []string
		noun string
	}{{"repos", d.Repos, "repository"}, {"authors", d.Authors, "login"}, {"labels", d.Labels, "label"}, {"requested", d.Requested, "reviewer"}, {"reasons", d.Reasons, "reason"}, {"state", d.States, "state"},
		{"checks", d.Checks, "state of checks"}, {"decision", d.Decision, "review decision"}, {"merge", d.Merge, "state of merging"}} {
		if l.v != nil && len(l.v) == 0 {
			at(l.key, "must name at least one %s", l.noun)
		}
	}
	for j, g := range d.Repos {
		owner, repo, ok := strings.Cut(g, "/")
		if _, err := path.Match(g, ""); err != nil || !ok || owner == "" || repo == "" || strings.Contains(repo, "/") {
			r.item("repos", j, "%q must be owner/repo, where * matches any part of either", g)
		}
	}
	for j, who := range d.Requested {
		if who != "me" && (!strings.HasPrefix(who, "@") || len(who) == 1) {
			r.item("requested", j, `%q must be "me", a team as "@slug", or "@*" for any team`, who)
		}
	}
	for j, st := range d.States {
		if !slices.Contains(States, st) {
			r.item("state", j, "must be one of %s, got %q", quoted(States), st)
		}
	}
	for j, reason := range d.Reasons {
		if !slices.Contains(Reasons, reason) {
			r.item("reasons", j, "%q is not a reason GitHub gives: %s", reason, strings.Join(Reasons, ", "))
		}
	}
	// A title left empty sets no condition, as a list left empty does: it
	// would match every title.
	if d.Title != nil && d.Title.raw != "" {
		if d.Title.err != nil {
			at("title", "is not a regular expression: %v", d.Title.err)
		}
		c.Title = d.Title.Re
	}
	for _, n := range []struct {
		key      string
		v        *int
		dst      *int
		min, max int
	}{
		{"max_files", d.MaxFiles, &c.MaxFiles, 1, 0}, {"max_lines", d.MaxLines, &c.MaxLines, 1, 0},
		{"min_files", d.MinFiles, &c.MinFiles, 1, 0}, {"min_lines", d.MinLines, &c.MinLines, 1, 0},
		{"updated_within_days", d.UpdatedIn, &c.UpdatedWithinDays, 1, MaxWithinDays}, {"created_within_days", d.CreatedIn, &c.CreatedWithinDays, 1, MaxWithinDays},
		{"updated_before_days", d.UpdatedBefore, &c.UpdatedBeforeDays, 1, MaxWithinDays}, {"created_before_days", d.CreatedBefore, &c.CreatedBeforeDays, 1, MaxWithinDays},
	} {
		if n.v == nil {
			continue
		}
		switch {
		case n.max > 0 && (*n.v < n.min || *n.v > n.max):
			at(n.key, "must be between %d and %d, got %d", n.min, n.max, *n.v)
		case *n.v < n.min:
			at(n.key, "must be %d or more, got %d", n.min, *n.v)
		}
		*n.dst = *n.v
	}
	for _, e := range []struct {
		key    string
		v      []string
		values []string
	}{{"checks", d.Checks, CheckStates}, {"decision", d.Decision, Decisions}, {"merge", d.Merge, MergeStates}} {
		for j, v := range e.v {
			if !slices.Contains(e.values, v) {
				r.item(e.key, j, "must be one of %s, got %q", quoted(e.values), v)
			}
		}
	}
	return c
}

// noCondition reports whether d sets no condition. A list or a title left
// empty sets none: the rule reads it as none.
func noCondition(d conditionDoc) bool {
	return len(d.Repos) == 0 && len(d.Authors) == 0 && len(d.Labels) == 0 && len(d.Requested) == 0 && len(d.Reasons) == 0 && len(d.States) == 0 &&
		len(d.Checks) == 0 && len(d.Decision) == 0 && len(d.Merge) == 0 && (d.Title == nil || d.Title.raw == "") &&
		d.MaxFiles == nil && d.MaxLines == nil && d.MinFiles == nil && d.MinLines == nil &&
		d.UpdatedIn == nil && d.CreatedIn == nil && d.UpdatedBefore == nil && d.CreatedBefore == nil &&
		d.Bot == nil && d.Mine == nil && d.Draft == nil && d.ApprovedByMe == nil && d.ReviewedByMe == nil
}

// reportNoCondition reports a rule named name that sets no condition, and
// so matches every pull request.
func reportNoCondition(d conditionDoc, packages []string, name string, r elemReporter) {
	if len(packages) == 0 && noCondition(d) {
		what := "the rule"
		if name != "" {
			what = fmt.Sprintf("%q", name)
		}
		r.at("", "%s matches every pull request; give it at least one condition", what)
	}
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
			if c, ok := near.Closest(key[len(key)-1], settingsIn(key[:len(key)-1])); ok {
				msg += fmt.Sprintf("; did you mean %q?", strings.Join(append(slices.Clone(key[:len(key)-1]), c), "."))
			}
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
	"claude.cache_dir":     `moved to "cache.dir"`,
	"theme.name":           `replaced by theme.dark and theme.light, the themes used on a dark terminal and on a light one; write the same theme in both to use it on any`,
	"sections":             `moved: each [[tabs]] element has its own sections = [...], which split it by rules, and [[exclude]] leaves pull requests out of every tab`,
	"inbox":                `moved: the inbox is a [[tabs]] element with kind = "inbox", holding limit, and show_settled for hide_closed with the opposite value; done_after_review is review.mark_done`,
	"ui":                   `moved: theme is theme.dark and theme.light; stale_days and home_owners are under [list]; editor is review.editor; approval_checks is checks.approval`,
	"refresh":              `moved: refresh.interval is list.refresh, and "0" is "off"`,
	"preview.description":  `moved to "description.style"`,
	"merge.checks_timeout": `moved to "checks.wait_timeout"`,
	"claude.mcp_servers":   `moved to "claude.mcp.servers", beside claude.mcp.builtin`,
	"merge.watch_timeout":  `is no longer a setting: gh-desk watches a pull request on auto-merge for 24h, and GitHub's auto-merge goes on after`,
}

// settingsIn are the keys the file may have in table, a path of table
// names from the top ([] for the top, [tabs] for an element of [[tabs]]),
// read from the toml tags of doc.
func settingsIn(table []string) []string {
	t := reflect.TypeFor[doc]()
	for _, name := range table {
		f, ok := fieldTagged(t, name)
		if !ok {
			return nil
		}
		t = f.Type
		for t.Kind() == reflect.Pointer || t.Kind() == reflect.Slice {
			t = t.Elem()
		}
		if t.Kind() != reflect.Struct {
			return nil
		}
	}
	return tagsOf(t)
}

// tagsOf are the keys of the struct type t, with those of the structs it
// embeds, which the decoder reads as its own.
func tagsOf(t reflect.Type) []string {
	var names []string
	for f := range t.Fields() {
		if f.Anonymous && f.Type.Kind() == reflect.Struct {
			names = append(names, tagsOf(f.Type)...)
			continue
		}
		if tag, _, _ := strings.Cut(f.Tag.Get("toml"), ","); tag != "" && tag != "-" && f.Tag.Get("removed") == "" {
			names = append(names, tag)
		}
	}
	return names
}

func fieldTagged(t reflect.Type, name string) (reflect.StructField, bool) {
	for f := range t.Fields() {
		if tag, _, _ := strings.Cut(f.Tag.Get("toml"), ","); tag == name {
			return f, true
		}
	}
	return reflect.StructField{}, false
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

// namedTwice reports the values of the list at key named twice.
func namedTwice(key string, ss []string) Problems {
	var ps Problems
	for i, s := range ss {
		if slices.Contains(ss[:i], s) {
			ps = append(ps, Problem{Key: strings.Split(key, "."), Index: i, Message: fmt.Sprintf("%s names %q twice", key, s)})
		}
	}
	return ps
}

// checkColumns reports the columns named twice, and pr or title left out:
// without them a row does not say which pull request it is.
func checkColumns(cols []string) Problems {
	ps := namedTwice("list.columns", cols)
	for _, c := range []string{"pr", "title"} {
		if !slices.Contains(cols, c) {
			ps = append(ps, problem("list.columns", "list.columns must have %q", c))
		}
	}
	return ps
}
