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

type (
	boolKind struct{}
	// intKind is a whole number from Min.
	intKind struct{ Min int }
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
	if n := v.(int); n < k.Min {
		return Problems{problem(key, "%s must be %d or more, got %d", key, k.Min, n)}
	}
	return nil
}
func (intKind) toml(v any) string { return fmt.Sprint(v) }
func (k intKind) schema() map[string]any {
	return map[string]any{"type": "integer", "minimum": k.Min}
}

func (k stringKind) check(key string, v any) Problems {
	if k.NonEmpty && strings.TrimSpace(v.(string)) == "" {
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

func (k enumKind) check(key string, v any) Problems { return oneOf(key, v.(string), k.Values) }
func (enumKind) toml(v any) string                  { return fmt.Sprintf("%q", v) }
func (k enumKind) schema() map[string]any {
	return map[string]any{"type": "string", "enum": k.Values}
}

func (k listKind) check(key string, v any) Problems {
	ss := v.([]string)
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
func (listKind) toml(v any) string { return tomlList(v.([]string)) }
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
	if p, bad := v.(Duration).problem(key, k.Off); bad {
		return Problems{p}
	}
	return nil
}
func (durationKind) toml(v any) string { return fmt.Sprintf("%q", v.(Duration).String()) }
func (k durationKind) schema() map[string]any {
	d := map[string]any{"type": "string", "pattern": `^([0-9]+(\.[0-9]+)?(ns|us|µs|ms|s|m|h))+$`}
	if !k.Off {
		return d
	}
	return map[string]any{"anyOf": []any{d, map[string]any{"const": "off"}}}
}

func (daysKind) check(key string, v any) Problems {
	if p, bad := v.(Days).problem(key); bad {
		return Problems{p}
	}
	return nil
}
func (daysKind) toml(v any) string {
	if d := v.(Days); !d.Off {
		return fmt.Sprint(d.N)
	}
	return `"off"`
}
func (daysKind) schema() map[string]any {
	return map[string]any{"anyOf": []any{map[string]any{"type": "integer", "minimum": 1}, map[string]any{"const": "off"}}}
}

func (globsKind) check(key string, v any) Problems { return globProblems(key, v.([]Glob)) }
func (globsKind) toml(v any) string                { return tomlList(Patterns(v.([]Glob))) }
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
	{Name: "preview"},
	{Name: "description"},
	{Name: "diff"},
	{Name: "checks"},
	{Name: "review"},
	{Name: "merge"},
	{Name: "bulk", Before: selectExamples},
	{Name: "claude", Doc: "C asks Claude Code (claude -p, with the account you are signed in with). On a pull request's screen, it reviews that pull request in a worktree of its head. On the list, it plans what to show and select in the tab shown, with no tools, and answers in the language you ask in; enter applies the plan, and an approve or a merge in it opens the confirm popup. Claude only reads: no settings files or MCP servers are loaded, its file tools see only the worktree, and it has none that run commands."},
	{Name: "theme"},
	{Name: "log"},
	{Name: "cache"},
}

// settings are every key of the tables, table by table in their order.
var settings = []setting{
	{Key: "list.refresh", Kind: durationKind{Off: true}, Value: func(f File) any { return f.List.Refresh },
		Doc: `Refresh the shown tab this often, as a Go duration such as "3m" or "90s"; "off" refreshes only when R is pressed.`},
	{Key: "list.stale_days", Kind: daysKind{}, Value: func(f File) any { return f.List.StaleDays },
		Doc: `Mark AGE when a pull request has not been updated for this many days, or "off".`},
	{Key: "list.home_owners", Kind: listKind{}, Value: func(f File) any { return f.List.HomeOwners },
		Doc: "Owners whose name is left out of the PR column, such as your company."},

	{Key: "preview.height", Kind: intKind{Min: 0}, Value: func(f File) any { return f.Preview.Height },
		Doc: "Lines of the preview pane under the list; 0 hides it until it is shown with a key. Dragging the pane's top edge, or + and -, changes them, and the lines set that way are kept over this one."},
	{Key: "preview.tab", Kind: enumKind{PaneTabs}, Value: func(f File) any { return f.Preview.Tab },
		Doc: `The tab the pane opens on: "overview" (default), the description beside a column of facts, or "diff".`},

	{Key: "description.style", Kind: enumKind{DescriptionStyles}, Value: func(f File) any { return f.Description.Style },
		Doc: `How a description is drawn, in the pane and on the PR screen: "rendered" (default) draws headings, lists, code, quotes and tables; "plain" shows the Markdown as written.`},

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

	{Key: "theme.name", Kind: enumKind{Themes}, Value: func(f File) any { return f.Theme.Name },
		Doc: `Color scheme: "auto" (default) picks mocha on a dark terminal and latte on a light one; or "mocha", "latte".`},

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
		{Key: "author", Kind: stringKind{}, Doc: "A login; bots without app/."},
		{Key: "packages", Kind: listKind{}, Doc: "The dependencies a renovate or dependabot pull request updates, read from its body; every one it updates must be listed."},
		{Key: "title", Kind: stringKind{}, Doc: "A regular expression the title must match."},
		{Key: "max_files", Kind: intKind{Min: 1}, Doc: "The most files it may change."},
		{Key: "bot", Kind: boolKind{}, Doc: "The author must be a bot."},
		{Key: "checks", Kind: enumKind{[]string{"passing"}}, Doc: `"passing": passed, or only checks.approval pending.`},
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
// the user picks a value. keys is the [keys] part, every action with its
// default keys, which the caller writes from the keymap.
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
	for _, w := range strings.Fields(text) {
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
