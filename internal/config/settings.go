package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
)

// The settings are defined once, here: the template written on first run,
// the JSON Schema an editor checks the file with, and the checks of each
// value's range and choices are made from this table. The file is decoded
// into the doc struct, and a test keeps the table and doc's toml tags one
// to one.

// setting is one key of a table.
type setting struct {
	// Key is table.key.
	Key string
	// Doc is what the template says above the key, and the schema's
	// description.
	Doc  string
	Kind kind
	// Value is the setting in f.
	Value func(f File) any
	// Example is written in the template in place of the default, when the
	// default is not a value to write (none, or worked out at run time).
	Example string
}

// table is a table of the file, in the order the template writes them.
type table struct {
	Name string
	// Doc is what the template says under the table's header.
	Doc string
	// Before is written before the table's header, as the examples of an
	// array of tables.
	Before string
}

// elem is a key of an element of an array of tables ([[tabs]],
// [[select]]). Their checks depend on one another, so Load makes them by
// hand; the table gives the schema.
type elem struct {
	Key  string
	Doc  string
	Kind kind
}

// kind is what values a setting takes.
type kind interface {
	// check reports what is wrong with v, the setting's value, at key.
	check(key string, v any) Problems
	// toml is v as the template writes it.
	toml(v any) string
	// schema is the setting's JSON Schema, without its description.
	schema() map[string]any
}

// as is v, a setting's value, as the type its kind takes. The table pairs
// every kind with a value of that type, so another type is a mistake in the
// table and panics.
func as[T any](v any) T {
	t, ok := v.(T)
	if !ok {
		panic(fmt.Sprintf("config: a setting's value is %T, not %T", v, t))
	}
	return t
}

type (
	boolKind struct{}
	// intKind is a whole number from Min.
	// intKind is an int of Min or more, and of Max or less when Max is not 0.
	intKind struct{ Min, Max int }
	// stringKind is any string, or one with something in it.
	stringKind struct{ NonEmpty bool }
	// enumKind is one of Values.
	enumKind struct{ Values []string }
	// listKind is a list of strings, each among Values when there are
	// some, and at least one when NonEmpty.
	listKind struct {
		Values   []string
		NonEmpty bool
	}
	// durationKind is a Duration, or "off" when Off.
	durationKind struct{ Off bool }
	daysKind     struct{}
	globsKind    struct{}
)

func (boolKind) check(string, any) Problems { return nil }
func (boolKind) toml(v any) string          { return fmt.Sprint(v) }
func (boolKind) schema() map[string]any     { return map[string]any{"type": "boolean"} }

func (k intKind) check(key string, v any) Problems {
	if n := as[int](v); n < k.Min || k.Max != 0 && n > k.Max {
		if k.Max != 0 {
			return Problems{problem(key, "%s must be between %d and %d, got %d", key, k.Min, k.Max, n)}
		}
		return Problems{problem(key, "%s must be %d or more, got %d", key, k.Min, n)}
	}
	return nil
}
func (intKind) toml(v any) string { return fmt.Sprint(v) }
func (k intKind) schema() map[string]any {
	s := map[string]any{"type": "integer", "minimum": k.Min}
	if k.Max != 0 {
		s["maximum"] = k.Max
	}
	return s
}

func (k stringKind) check(key string, v any) Problems {
	if k.NonEmpty && strings.TrimSpace(as[string](v)) == "" {
		return Problems{problem(key, "%s must not be empty", key)}
	}
	return nil
}
func (stringKind) toml(v any) string { return fmt.Sprintf("%q", v) }
func (k stringKind) schema() map[string]any {
	s := map[string]any{"type": "string"}
	if k.NonEmpty {
		s["pattern"] = `\S`
	}
	return s
}

func (k enumKind) check(key string, v any) Problems { return oneOf(key, as[string](v), k.Values) }
func (enumKind) toml(v any) string                  { return fmt.Sprintf("%q", v) }
func (k enumKind) schema() map[string]any {
	return map[string]any{"type": "string", "enum": k.Values}
}

func (k listKind) check(key string, v any) Problems {
	ss := as[[]string](v)
	var ps Problems
	if k.NonEmpty && len(ss) == 0 {
		ps = append(ps, problem(key, "%s must name at least one of %s", key, quoted(k.Values)))
	}
	if len(k.Values) > 0 {
		for i, s := range ss {
			if !slices.Contains(k.Values, s) {
				ps = append(ps, Problem{Key: strings.Split(key, "."), Index: i,
					Message: fmt.Sprintf("%s[%d] must be one of %s, got %q%s", key, i, quoted(k.Values), s, didYouMean(s, k.Values))})
			}
		}
	}
	return ps
}
func (listKind) toml(v any) string { return tomlList(as[[]string](v)) }
func (k listKind) schema() map[string]any {
	items := map[string]any{"type": "string"}
	if len(k.Values) > 0 {
		items["enum"] = k.Values
	}
	s := map[string]any{"type": "array", "items": items}
	if k.NonEmpty {
		s["minItems"] = 1
	}
	return s
}

func (k durationKind) check(key string, v any) Problems {
	if p, bad := as[Duration](v).problem(key, k.Off); bad {
		return Problems{p}
	}
	return nil
}
func (durationKind) toml(v any) string { return fmt.Sprintf("%q", as[Duration](v).String()) }
func (k durationKind) schema() map[string]any {
	d := map[string]any{"type": "string", "pattern": `^([0-9]+(\.[0-9]+)?(ns|us|µs|ms|s|m|h))+$`}
	if !k.Off {
		return d
	}
	return map[string]any{"anyOf": []any{d, map[string]any{"const": "off"}}}
}

func (daysKind) check(key string, v any) Problems {
	if p, bad := as[Days](v).problem(key); bad {
		return Problems{p}
	}
	return nil
}
func (daysKind) toml(v any) string {
	if d := as[Days](v); !d.Off {
		return fmt.Sprint(d.N)
	}
	return `"off"`
}
func (daysKind) schema() map[string]any {
	return map[string]any{"anyOf": []any{map[string]any{"type": "integer", "minimum": 1}, map[string]any{"const": "off"}}}
}

func (globsKind) check(key string, v any) Problems { return globProblems(key, as[[]Glob](v)) }
func (globsKind) toml(v any) string                { return tomlList(Patterns(as[[]Glob](v))) }
func (globsKind) schema() map[string]any {
	return map[string]any{"type": "array", "items": map[string]any{"type": "string"}}
}

func tomlList(ss []string) string {
	q := make([]string, len(ss))
	for i, s := range ss {
		q[i] = fmt.Sprintf("%q", s)
	}
	return "[" + strings.Join(q, ", ") + "]"
}

// tables are the tables of the file, in the order the template writes
// them.
var tables = []table{
	{Name: "list"},
	{Name: "menu"},
	{Name: "preview"},
	{Name: "description"},
	{Name: "diff"},
	{Name: "checks"},
	{Name: "review"},
	{Name: "merge"},
	{Name: "bulk", Before: selectExamples},
	{Name: "claude", Doc: "C and i ask Claude Code (claude -p, with the account you are signed in with). i, or ask claude about it in the menu of enter, reviews the pull request under the cursor in a worktree of its head. C plans what to show and select in the tab shown, with no tools, and answers in the language you ask in; enter applies the plan, and an approve or a merge in it opens the confirm popup. Claude only reads: no settings files or MCP servers are loaded, its file tools see only the worktree, and it has none that run commands."},
	{Name: "theme"},
	{Name: "log"},
	{Name: "cache"},
}

// settings are every key of the tables, table by table in their order.
var settings = []setting{
	{Key: "list.refresh", Kind: durationKind{Off: true}, Value: func(f File) any { return f.List.Refresh },
		Doc: `Refresh the shown tab this often, as a Go duration such as "3m" or "90s"; "off" refreshes only when r is pressed.`},
	{Key: "list.stale_days", Kind: daysKind{}, Value: func(f File) any { return f.List.StaleDays },
		Doc: `Mark AGE when a pull request has not been updated for this many days, or "off".`},
	{Key: "list.home_owners", Kind: listKind{}, Value: func(f File) any { return f.List.HomeOwners },
		Doc: "Owners whose name is left out of the PR column, such as your company."},
	{Key: "list.columns", Kind: listKind{Values: Columns, NonEmpty: true}, Value: func(f File) any { return f.List.Columns },
		Doc: `The columns shown, in the order given; "pr" and "title" are needed. WHY shows on grouped tabs and the inbox only, and AUTHOR not on a tab of your own pull requests. On a narrow terminal SIZE goes first, then AUTHOR.`},

	{Key: "menu.actions", Kind: listKind{Values: MenuActions, NonEmpty: true}, Value: func(f File) any { return f.Menu.Actions },
		Doc: `The actions the menu of enter offers, in the order given. One left out is not in the menu; its key still runs it from the list.`},

	{Key: "preview.height", Kind: intKind{Min: 0}, Value: func(f File) any { return f.Preview.Height },
		Doc: "Lines of the preview pane under the list; 0 hides it until it is shown with a key. Dragging the pane's top edge, or + and -, changes them, and the lines set that way are kept over this one."},
	{Key: "preview.tab", Kind: enumKind{PaneTabs}, Value: func(f File) any { return f.Preview.Tab },
		Doc: `The tab the pane opens on: "overview" (default), the description beside a column of facts, or "diff".`},

	{Key: "description.style", Kind: enumKind{DescriptionStyles}, Value: func(f File) any { return f.Description.Style },
		Doc: `How a description and its comments are drawn in the pane: "rendered" (default) draws headings, lists, code, quotes and tables; "plain" shows the Markdown as written.`},

	{Key: "diff.collapse", Kind: globsKind{}, Value: func(f File) any { return f.Diff.Collapse },
		Doc: "Files shown folded, as their name and line counts, until unfolded: glob patterns, matched against the base name unless they have a slash."},

	{Key: "checks.approval", Kind: globsKind{}, Value: func(f File) any { return f.Checks.Approval },
		Doc: "Checks that pass only after an approval, such as a policy bot's: glob patterns of check names. While only they are pending, CI shows ◌, and the checks count as passing when deciding a merge."},

	{Key: "review.mark_done", Kind: boolKind{}, Value: func(f File) any { return f.Review.MarkDone },
		Doc: "Mark a pull request's notification done after you review it here, from any tab."},
	{Key: "review.editor", Kind: stringKind{}, Value: func(f File) any { return f.Review.Editor }, Example: `"code --wait"`,
		Doc: `The editor ctrl+e opens to write a review, with its arguments, such as "code --wait". Left out, $VISUAL, then $EDITOR.`},

	{Key: "merge.methods", Kind: listKind{Values: MergeMethods, NonEmpty: true}, Value: func(f File) any { return f.Merge.Methods },
		Doc: `The order of preference among the methods a repository allows: "merge", "squash", "rebase". The first one the repository allows is used.`},
	{Key: "merge.auto_merge", Kind: boolKind{}, Value: func(f File) any { return f.Merge.AutoMerge },
		Doc: "Use GitHub's auto-merge where the repository allows it; elsewhere gh-triage waits for the checks and merges when they pass. Commits pushed after auto-merge is turned on are merged too, as GitHub does."},
	{Key: "merge.checks_timeout", Kind: durationKind{}, Value: func(f File) any { return f.Merge.ChecksTimeout },
		Doc: "Where gh-triage waits, how long to wait for required checks still running. Only the time gh-triage is running counts."},
	{Key: "merge.blocked_timeout", Kind: durationKind{}, Value: func(f File) any { return f.Merge.BlockedTimeout },
		Doc: "Where gh-triage waits, how long a pull request may stay blocked by another rule (a second approval, code owners) after its required checks finished. Only the time gh-triage is running counts."},
	{Key: "merge.update_branch", Kind: enumKind{UpdateBranchValues}, Value: func(f File) any { return f.Merge.UpdateBranch },
		Doc: `Whose branch is updated when it is behind its base: "humans" (default), "all" or "never". Bots rebase their own branches, and some (renovate) stop updating a branch someone else committed to.`},

	{Key: "bulk.attention_paths", Kind: globsKind{}, Value: func(f File) any { return f.Bulk.AttentionPaths },
		Doc: "Pull requests changing files that match these glob patterns get a mark in the confirm popup."},

	{Key: "claude.command", Kind: stringKind{NonEmpty: true}, Value: func(f File) any { return f.Claude.Command },
		Doc: "The claude command: a name looked up on PATH, or a path to it (~ is your home), without arguments."},
	{Key: "claude.model", Kind: stringKind{NonEmpty: true}, Value: func(f File) any { return f.Claude.Model },
		Doc: "The model Claude Code runs, as claude --model takes it."},
	{Key: "claude.allowed_tools", Kind: listKind{}, Value: func(f File) any { return f.Claude.AllowedTools },
		Doc: `The tools Claude may use, among "Read", "Grep", "Glob" and "WebFetch".`},
	{Key: "claude.language", Kind: stringKind{NonEmpty: true}, Value: func(f File) any { return f.Claude.Language },
		Doc: `The language Claude answers in about a pull request, such as "Japanese". A review drafted with D is written in the language of the pull request instead.`},
	{Key: "claude.instructions", Kind: stringKind{}, Value: func(f File) any { return f.Claude.Instructions },
		Doc: `Appended to the instructions of every review and triage, such as "Point out changes that have no tests."`},
	{Key: "claude.release_notes_check", Kind: boolKind{}, Value: func(f File) any { return f.Claude.ReleaseNotesCheck },
		Doc: "Let Claude read the release notes of the bot pull requests in the merge popup, and mark those that need care."},

	{Key: "theme.dark", Kind: enumKind{Themes}, Value: func(f File) any { return f.Theme.Dark },
		Doc: `The color scheme on a dark terminal, and on one that does not tell its background.`},
	{Key: "theme.light", Kind: enumKind{Themes}, Value: func(f File) any { return f.Theme.Light },
		Doc: `The color scheme on a light terminal. Write the same theme as theme.dark to use it on any terminal.`},
	{Key: "theme.spinner", Kind: enumKind{Spinners}, Value: func(f File) any { return f.Theme.Spinner },
		Doc: `The spinner shown while something is read: "dots" (default) ⠋⠙⠹, "dense" ⣾⣽⣻, "bounce" ⠁⠂⠄, "snake" ⠉⠙⠚, "jump" ⢄⢂⢁, "fill" ⣀⣤⣶, "sand" ⡀⣀⣤, "line" |/-\, "arc" ◜◠◝, "quarter" ◴◷◶, "block" ▖▘▝, "star" ✶✸✹, or "off", a ⋯ that does not move and is left out before a line that says what is going on.`},
	{Key: "theme.shimmer", Kind: boolKind{}, Value: func(f File) any { return f.Theme.Shimmer },
		Doc: "Run a light over the text that says what is going on, such as refreshing… or thinking…; the screen is drawn more often while something is read."},
	{Key: "theme.wave", Kind: boolKind{}, Value: func(f File) any { return f.Theme.Wave },
		Doc: "Put each line of a table a frame of the spinner ahead of the one above it, so that a wave runs down a column being read."},

	{Key: "log.level", Kind: stringKind{}, Value: func(f File) any { return f.Log.Level },
		Doc: `How much goes to the log file: "debug", "info" (default), "warn" or "error". --debug and GH_TRIAGE_LOG_LEVEL take precedence. gh triage log prints where the file is.`},

	{Key: "cache.dir", Kind: stringKind{NonEmpty: true}, Value: func(f File) any { return f.Cache.Dir }, Example: `"~/.cache/gh-triage"`,
		Doc: "Where gh-triage keeps its clones and the worktrees Claude reads, and the lists last read (lists.json, shown at once at the next launch) (~ is your home); by default $XDG_CACHE_HOME/gh-triage, or ~/.cache/gh-triage."},
}

// elems are the keys of the arrays of tables.
var elems = map[string][]elem{
	"tabs": {
		{Key: "kind", Kind: enumKind{TabKinds}, Doc: `"search" (default), a GitHub search, or "inbox", the pull requests in your notifications.`},
		{Key: "name", Kind: stringKind{NonEmpty: true}, Doc: `The tab's name; "inbox" for the inbox by default.`},
		{Key: "query", Kind: stringKind{NonEmpty: true}, Doc: "A search's GitHub search query."},
		{Key: "groups", Kind: listKind{Values: GroupNames, NonEmpty: true}, Doc: "The groups a search is split into, in the order shown."},
		{Key: "folded", Kind: listKind{Values: GroupNames}, Doc: "The groups folded at start, among groups."},
		{Key: "sort", Kind: enumKind{TabSorts}, Doc: `"updated" (default) or "drafts_last".`},
		{Key: "limit", Kind: intKind{Min: 1}, Doc: "How many pull requests, or notifications, to read (at most 1000; 100 by default)."},
		{Key: "show_settled", Kind: boolKind{}, Doc: "The inbox shows notifications whose pull request is merged or closed from the start."},
	},
	"select": {
		{Key: "name", Kind: stringKind{NonEmpty: true}, Doc: "The rule's name, as S lists it."},
		{Key: "authors", Kind: listKind{NonEmpty: true}, Doc: "The logins the author must be one of; bots without app/."},
		{Key: "packages", Kind: listKind{}, Doc: "The dependencies a renovate or dependabot pull request updates, read from its body; every one it updates must be listed."},
		{Key: "title", Kind: stringKind{}, Doc: "A regular expression the title must match."},
		{Key: "max_files", Kind: intKind{Min: 1}, Doc: "The most files it may change."},
		{Key: "max_lines", Kind: intKind{Min: 1}, Doc: "The most lines it may add and delete together: the sum of the two numbers SIZE shows."},
		{Key: "updated_within_days", Kind: intKind{Min: 1, Max: MaxWithinDays}, Doc: "The most days, of 24 hours, since it was last updated: a label or a bot's comment updates it too. An old pull request updated lately matches."},
		{Key: "created_within_days", Kind: intKind{Min: 1, Max: MaxWithinDays}, Doc: "The most days, of 24 hours, since it was created."},
		{Key: "bot", Kind: boolKind{}, Doc: "The author must be a bot."},
		{Key: "checks", Kind: enumKind{[]string{"passing"}}, Doc: `"passing": passed, or only checks.approval pending.`},
		{Key: "approved_by_me", Kind: boolKind{}, Doc: "Your latest review must approve it; a person's pull request matches too, unlike the built-in \"approved by me\"."},
	},
}

// checkSettings checks every setting's value against its kind.
func checkSettings(f File) Problems {
	var ps Problems
	for _, s := range settings {
		ps = append(ps, s.Kind.check(s.Key, s.Value(f))...)
	}
	return ps
}

const templateHead = `# gh-triage settings. Uncomment a line to change it.

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
`

const selectExamples = `# Rules S offers to select pull requests, besides "approved by me" (bot pull
# requests you approved that are still open). A pull request matches when it
# meets every key given: authors (the logins it may be by; bots without
# app/), packages (the dependencies a renovate or dependabot pull request
# updates, read from its body; every one it updates must be listed), title
# (a regular expression), max_files (1 or more), max_lines (1 or more; the
# lines added and deleted together, the sum of the two numbers SIZE shows),
# updated_within_days and created_within_days (the most days, of 24 hours,
# since it was last updated or created; a label or a bot's comment updates
# it too, and the inbox's AGE is its notification's time, not this), a bot as
# the author with bot = true, checks = "passing" (passed, or only
# checks.approval pending), and approved_by_me = true (your latest review
# approves it; a person's pull request matches too).
#
# The rules below are written to start with, not commented out: S offers
# only the rules in this file, so delete or comment out one you do not want.

[[select]]
name = "small"
max_lines = 300

[[select]]
name = "dependency bots"
authors = ["renovate", "dependabot"]
checks = "passing"

[[select]]
name = "approved"
approved_by_me = true

[[select]]
name = "recent"
updated_within_days = 90

[[select]]
name = "lock files"
authors = ["renovate"]
title = "(?i)lock file maintenance"

# A rule on the packages a bot's pull request updates:
#
# [[select]]
# name = "safe renovate"
# authors = ["renovate"]
# packages = ["actions/checkout", "songmu/tagpr"]
# checks = "passing"
`

const keysHead = `[keys]
# Which keys do what, by action: a key or a list of keys, replacing the
# action's own, or [] to turn it off. A key given to an action is taken from
# an action that has it by default where they meet. ctrl+c and esc are fixed.
# Keys are written as key presses are read: "a", "A", "enter", "space",
# "ctrl+s", "shift+tab".
#
`

// SchemaURL is where the JSON Schema of the file is published; the
// template names it for editors that read it (taplo).
const SchemaURL = "https://raw.githubusercontent.com/babarot/gh-triage/main/docs/config.schema.json"

// Template is the config file written on first run: every setting at its
// default, commented out, so a default changed later still applies until
// the user picks a value. The [[select]] rules alone are written live, as
// rules to start with that the user deletes or comments out (ADR-028).
// keys is the [keys] part, every action with its default keys, which the
// caller writes from the keymap.
func Template(keys string) string {
	def := Default()
	var b strings.Builder
	b.WriteString("#:schema " + SchemaURL + "\n")
	b.WriteString(templateHead)
	for _, t := range tables {
		b.WriteString("\n")
		if t.Before != "" {
			b.WriteString(t.Before + "\n")
		}
		b.WriteString("[" + t.Name + "]\n")
		b.WriteString(comment(t.Doc))
		for _, s := range settings {
			table, key, _ := strings.Cut(s.Key, ".")
			if table != t.Name {
				continue
			}
			v := s.Example
			if v == "" {
				v = s.Kind.toml(s.Value(def))
			}
			b.WriteString(comment(s.Doc) + "# " + key + " = " + v + "\n")
		}
	}
	b.WriteString("\n" + keysHead + keys)
	return b.String()
}

// comment is text as comment lines of the template, wrapped at 76
// columns.
func comment(text string) string {
	if text == "" {
		return ""
	}
	var b strings.Builder
	line := "#"
	for w := range strings.FieldsSeq(text) {
		if len(line)+1+len(w) > 76 && line != "#" {
			b.WriteString(line + "\n")
			line = "#"
		}
		line += " " + w
	}
	b.WriteString(line + "\n")
	return b.String()
}

// Schema is the JSON Schema of the file.
func Schema() []byte {
	props := map[string]any{}
	for _, t := range tables {
		tp := map[string]any{}
		for _, s := range settings {
			table, key, _ := strings.Cut(s.Key, ".")
			if table == t.Name {
				tp[key] = described(s.Kind.schema(), s.Doc)
			}
		}
		props[t.Name] = described(map[string]any{"type": "object", "additionalProperties": false, "properties": tp}, t.Doc)
	}
	for name, es := range elems {
		ep := map[string]any{}
		for _, e := range es {
			ep[e.Key] = described(e.Kind.schema(), e.Doc)
		}
		arr := map[string]any{"type": "array", "items": map[string]any{"type": "object", "additionalProperties": false, "properties": ep}}
		if name == "tabs" {
			arr["maxItems"] = MaxTabs
		}
		props[name] = arr
	}
	key := map[string]any{"anyOf": []any{map[string]any{"type": "string"}, map[string]any{"type": "array", "items": map[string]any{"type": "string"}}}}
	props["keys"] = map[string]any{"type": "object", "description": "Which keys do what, by action: a key or a list of keys, or [] to turn it off.",
		"additionalProperties": key}
	s := map[string]any{
		"$schema":              "http://json-schema.org/draft-07/schema#",
		"title":                "gh-triage config",
		"type":                 "object",
		"additionalProperties": false,
		"properties":           props,
	}
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	_ = enc.Encode(s)
	return b.Bytes()
}

func described(s map[string]any, doc string) map[string]any {
	if doc != "" {
		s["description"] = doc
	}
	return s
}

// Show is f as a config file, every setting with its value, or with
// onlyDiff only those that differ from the defaults. It reads back as f.
// The tabs are written all or none, since tabs given replace the defaults;
// [keys] is left to the caller, who has the keymap.
func Show(f File, onlyDiff bool) string {
	def := Default()
	var b strings.Builder
	if !onlyDiff || !slices.EqualFunc(f.Tabs, def.Tabs, sameTab) {
		for _, t := range f.Tabs {
			b.WriteString("[[tabs]]\n")
			if t.Inbox() {
				b.WriteString("kind = \"inbox\"\n")
			}
			fmt.Fprintf(&b, "name = %q\n", t.Name)
			if !t.Inbox() {
				fmt.Fprintf(&b, "query = %q\n", t.Query)
				if len(t.Groups) > 0 {
					b.WriteString("groups = " + tomlList(t.Groups) + "\n")
				}
				if len(t.Folded) > 0 {
					b.WriteString("folded = " + tomlList(t.Folded) + "\n")
				}
				fmt.Fprintf(&b, "sort = %q\n", t.Sort)
			}
			fmt.Fprintf(&b, "limit = %d\n", t.Limit)
			if t.Inbox() {
				fmt.Fprintf(&b, "show_settled = %v\n", t.ShowSettled)
			}
			b.WriteString("\n")
		}
	}
	for _, s := range f.Select {
		b.WriteString("[[select]]\n")
		fmt.Fprintf(&b, "name = %q\n", s.Name)
		if len(s.Authors) > 0 {
			b.WriteString("authors = " + tomlList(s.Authors) + "\n")
		}
		if len(s.Packages) > 0 {
			b.WriteString("packages = " + tomlList(s.Packages) + "\n")
		}
		if s.Title != nil {
			fmt.Fprintf(&b, "title = %q\n", s.Title.String())
		}
		if s.MaxFiles > 0 {
			fmt.Fprintf(&b, "max_files = %d\n", s.MaxFiles)
		}
		if s.MaxLines > 0 {
			fmt.Fprintf(&b, "max_lines = %d\n", s.MaxLines)
		}
		if s.UpdatedWithinDays > 0 {
			fmt.Fprintf(&b, "updated_within_days = %d\n", s.UpdatedWithinDays)
		}
		if s.CreatedWithinDays > 0 {
			fmt.Fprintf(&b, "created_within_days = %d\n", s.CreatedWithinDays)
		}
		if s.Bot {
			b.WriteString("bot = true\n")
		}
		if s.Checks != "" {
			fmt.Fprintf(&b, "checks = %q\n", s.Checks)
		}
		if s.ApprovedByMe {
			b.WriteString("approved_by_me = true\n")
		}
		b.WriteString("\n")
	}
	for _, t := range tables {
		var lines []string
		for _, s := range settings {
			table, key, _ := strings.Cut(s.Key, ".")
			if table != t.Name {
				continue
			}
			v := s.Kind.toml(s.Value(f))
			if onlyDiff && v == s.Kind.toml(s.Value(def)) {
				continue
			}
			// A setting with no value of its own (review.editor) is left out.
			if s.Example != "" && v == `""` {
				continue
			}
			lines = append(lines, key+" = "+v)
		}
		if len(lines) > 0 {
			b.WriteString("[" + t.Name + "]\n" + strings.Join(lines, "\n") + "\n\n")
		}
	}
	return b.String()
}

func sameTab(a, b Tab) bool {
	return a.Kind == b.Kind && a.Name == b.Name && a.Query == b.Query && slices.Equal(a.Groups, b.Groups) &&
		slices.Equal(a.Folded, b.Folded) && a.Sort == b.Sort && a.Limit == b.Limit && a.ShowSettled == b.ShowSettled
}
