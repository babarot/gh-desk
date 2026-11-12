package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
	"regexp"
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
	// Display is true of a setting that changes only how things are drawn:
	// it cannot make gh-desk run anything, write to GitHub, read something
	// else, or put another action under a key. The settings page saves
	// these without asking in the terminal (ADR-101).
	Display bool
}

// table is a table of the file, in the order the template writes them.
type table struct {
	Name string
	// Doc is what the template says under the table's header.
	Doc string
	// Rules writes the arrays of tables of the rules before this table.
	Rules bool
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

// kindName is what values a kind takes, in words.
func kindName(k kind) string {
	switch k := k.(type) {
	case boolKind:
		return "boolean"
	case optBoolKind:
		return "boolean, or left out"
	case intKind:
		if k.Max != 0 {
			return fmt.Sprintf("integer, %d to %d", k.Min, k.Max)
		}
		return fmt.Sprintf("integer, %d or more", k.Min)
	case stringKind:
		return "string"
	case enumKind:
		return "one of " + codeList(k.Values)
	case optEnumKind:
		return "one of " + codeList(k.Values) + ", or left out"
	case listKind:
		if len(k.Values) > 0 {
			return "list of " + codeList(k.Values)
		}
		return "list of strings"
	case durationKind:
		if k.Off {
			return `duration or "off"`
		}
		return "duration"
	case daysKind:
		return `days or "off"`
	case globsKind:
		return "list of glob patterns"
	case mcpServersKind:
		return "table of MCP servers"
	case sectionsKind:
		return "list of sections"
	}
	panic(fmt.Sprintf("config: no name for %T", k))
}

func codeList(vs []string) string {
	q := make([]string, len(vs))
	for i, v := range vs {
		q[i] = "`" + v + "`"
	}
	return strings.Join(q, ", ")
}

// splitKey is a setting's key as its table and its name in the table: the
// table is all but the last part, as claude.mcp of claude.mcp.builtin.
func splitKey(k string) (table, key string) {
	i := strings.LastIndex(k, ".")
	if i < 0 {
		return "", k
	}
	return k[:i], k[i+1:]
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
	// optBoolKind is a bool the file may leave out, which then has no
	// value: the default is worked out from other settings.
	optBoolKind struct{}
	// intKind is a whole number from Min.
	// intKind is an int of Min or more, and of Max or less when Max is not 0.
	intKind struct{ Min, Max int }
	// stringKind is any string, or one with something in it.
	stringKind struct{ NonEmpty bool }
	// enumKind is one of Values.
	enumKind struct{ Values []string }
	// optEnumKind is one of Values, or left out, as "".
	optEnumKind struct{ Values []string }
	// listKind is a list of strings, each among Values when there are
	// some, and at least one when NonEmpty. Renamed are values that were
	// renamed, by their old name, with the new one: not read, but told.
	listKind struct {
		Values   []string
		NonEmpty bool
		Renamed  map[string]string
	}
	// durationKind is a Duration, or "off" when Off.
	durationKind struct{ Off bool }
	daysKind     struct{}
	globsKind    struct{}
	// mcpServersKind is a table of MCP servers by name, each as Claude
	// Code configures a server, with the tools that may run without a
	// prompt in tools.
	mcpServersKind struct{}
)

func (boolKind) check(string, any) Problems { return nil }

func (boolKind) toml(v any) string      { return fmt.Sprint(v) }
func (boolKind) schema() map[string]any { return map[string]any{"type": "boolean"} }

func (optBoolKind) check(string, any) Problems { return nil }

// toml is "" when the file leaves the setting out, as Show leaves it out.
func (optBoolKind) toml(v any) string {
	if b := as[*bool](v); b != nil {
		return fmt.Sprint(*b)
	}
	return `""`
}
func (optBoolKind) schema() map[string]any { return map[string]any{"type": "boolean"} }

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

// sectionsKind is a tab's sections: a list of inline tables, each a name,
// how it is shown and the conditions of a rule. They are read by hand, as
// the rest of a tab is.
type sectionsKind struct{}

func (sectionsKind) check(string, any) Problems { return nil }
func (sectionsKind) toml(v any) string {
	ss := as[[]Section](v)
	if len(ss) == 0 {
		return "[]"
	}
	var b strings.Builder
	b.WriteString("[\n")
	for _, s := range ss {
		b.WriteString("  " + sectionTOML(s) + ",\n")
	}
	b.WriteString("]")
	return b.String()
}
func (sectionsKind) schema() map[string]any {
	props := map[string]any{
		"name": map[string]any{"type": "string", "pattern": `\S`, "description": "The head the section's rows are shown under; left out, the section has no head and cannot be folded."},
		"show": map[string]any{"type": "string", "enum": Shows, "description": `"open" (default), "folded" (its head alone, until unfolded) or "excluded" (left out as an exclusion rule leaves them, until a shows them).`},
	}
	for _, e := range sectionElems {
		props[e.Key] = described(e.Kind.schema(), e.Doc)
	}
	return map[string]any{"type": "array", "items": map[string]any{"type": "object", "additionalProperties": false, "properties": props}}
}

// sectionElems are the conditions a section takes: those of [[exclude]],
// which does not read the description either.
var sectionElems = slices.DeleteFunc(slices.Clone(conditionElems), func(e elem) bool { return e.Key == "packages" })

func (k enumKind) check(key string, v any) Problems { return oneOf(key, as[string](v), k.Values) }
func (enumKind) toml(v any) string                  { return fmt.Sprintf("%q", v) }
func (k enumKind) schema() map[string]any {
	return map[string]any{"type": "string", "enum": k.Values}
}

func (k optEnumKind) check(key string, v any) Problems {
	if as[string](v) == "" {
		return nil
	}
	return enumKind(k).check(key, v)
}
func (optEnumKind) toml(v any) string        { return fmt.Sprintf("%q", v) }
func (k optEnumKind) schema() map[string]any { return enumKind(k).schema() }

func (k listKind) check(key string, v any) Problems {
	ss := as[[]string](v)
	var ps Problems
	if k.NonEmpty && len(ss) == 0 {
		ps = append(ps, problem(key, "%s must name at least one of %s", key, quoted(k.Values)))
	}
	if len(k.Values) > 0 {
		for i, s := range ss {
			if to, ok := k.Renamed[s]; ok {
				ps = append(ps, Problem{Key: strings.Split(key, "."), Index: i,
					Message: fmt.Sprintf("%s[%d]: %q was renamed to %q", key, i, s, to)})
				continue
			}
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

// mcpName is what a server's name and its tools are made of: nothing that
// a permission rule of Claude Code reads as a pattern.
var mcpName = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// BuiltinServer is the name of gh-desk's own MCP server, which the user's
// servers may not take; internal/app checks it is the server's.
const BuiltinServer = "gh-desk"

func (mcpServersKind) check(key string, v any) Problems {
	servers := as[map[string]map[string]any](v)
	var ps Problems
	at := func(name string, sub ...string) []string {
		return append(append(strings.Split(key, "."), name), sub...)
	}
	for _, name := range slices.Sorted(maps.Keys(servers)) {
		srv := servers[name]
		if !mcpName.MatchString(name) {
			ps = append(ps, Problem{Key: at(name), Index: -1, AtKey: true,
				Message: fmt.Sprintf("%s.%s: a server's name is made of letters, digits, _ and -", key, name)})
		}
		if name == BuiltinServer {
			ps = append(ps, Problem{Key: at(name), Index: -1, AtKey: true,
				Message: fmt.Sprintf("%s.%s: %q is the name of gh-desk's own server, which mcp.builtin turns on", key, name, BuiltinServer)})
		}
		ts, ok := srv["tools"].([]any)
		if !ok || len(ts) == 0 {
			ps = append(ps, Problem{Key: at(name), Index: -1, AtKey: true,
				Message: fmt.Sprintf("%s.%s needs tools: the tools of the server that may run without a prompt", key, name)})
		}
		for i, t := range ts {
			if s, ok := t.(string); !ok || !mcpName.MatchString(s) {
				ps = append(ps, Problem{Key: at(name, "tools"), Index: i,
					Message: fmt.Sprintf("%s.%s.tools[%d] must name one tool, made of letters, digits, _ and -, got %v", key, name, i, t)})
			}
		}
		if c, ok := srv["command"]; ok {
			if s, ok := c.(string); !ok || strings.TrimSpace(s) == "" {
				ps = append(ps, problemAt(at(name, "command"), "%s.%s.command must be a command", key, name))
			}
		}
		if a, ok := srv["args"]; ok {
			as, ok := a.([]any)
			for _, x := range as {
				if _, s := x.(string); !s {
					ok = false
				}
			}
			if !ok {
				ps = append(ps, problemAt(at(name, "args"), "%s.%s.args must be a list of strings", key, name))
			}
		}
	}
	return ps
}

func problemAt(key []string, format string, args ...any) Problem {
	return Problem{Key: key, Index: -1, Message: fmt.Sprintf(format, args...)}
}

// toml writes the servers as an inline table, the values of env and
// headers hidden, since they may hold secrets.
func (mcpServersKind) toml(v any) string {
	servers := as[map[string]map[string]any](v)
	m := make(map[string]any, len(servers))
	for name, srv := range servers {
		c := make(map[string]any, len(srv))
		for k, x := range srv {
			if h, ok := x.(map[string]any); ok && (k == "env" || k == "headers") {
				hidden := make(map[string]any, len(h))
				for hk := range h {
					hidden[hk] = "(hidden)"
				}
				x = hidden
			}
			c[k] = x
		}
		m[name] = c
	}
	return tomlValue(m)
}

func (mcpServersKind) schema() map[string]any {
	strs := map[string]any{"type": "array", "items": map[string]any{"type": "string"}}
	return map[string]any{"type": "object", "additionalProperties": map[string]any{
		"type":     "object",
		"required": []string{"tools"},
		"properties": map[string]any{
			"tools":   map[string]any{"type": "array", "minItems": 1, "items": map[string]any{"type": "string", "pattern": mcpName.String()}},
			"command": map[string]any{"type": "string"},
			"args":    strs,
		},
	}}
}

// tomlValue is v, as decoded from TOML, written as an inline TOML value.
func tomlValue(v any) string {
	switch x := v.(type) {
	case string:
		return fmt.Sprintf("%q", x)
	case []any:
		q := make([]string, len(x))
		for i, e := range x {
			q[i] = tomlValue(e)
		}
		return "[" + strings.Join(q, ", ") + "]"
	case map[string]any:
		if len(x) == 0 {
			return "{}"
		}
		var q []string
		for _, k := range slices.Sorted(maps.Keys(x)) {
			q = append(q, tomlKey(k)+" = "+tomlValue(x[k]))
		}
		return "{ " + strings.Join(q, ", ") + " }"
	default:
		return fmt.Sprint(x)
	}
}

func tomlKey(k string) string {
	if mcpName.MatchString(k) {
		return k
	}
	return fmt.Sprintf("%q", k)
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
	{Name: "bulk", Rules: true},
	{Name: "claude", Doc: "C and i ask Claude Code (claude -p, with the account you are signed in with). i, or ask claude about it in the menu of enter, reviews the pull request under the cursor in a worktree of its head. C plans what to show and select in the tab shown, with no tools, and answers in the language you ask in; enter applies the plan, and an approve or a merge in it opens the confirm popup. Claude's own tools only read: no settings files are loaded, its file tools see only the worktree and the skills of plugins, and it has none that run commands. The conversation also takes the MCP servers of [claude.mcp] and the skills of plugins, which can do whatever they are made to."},
	{Name: "claude.mcp", Doc: "The MCP servers of the conversation about a pull request: gh-desk's own, and yours."},
	{Name: "theme"},
	{Name: "release"},
	{Name: "log"},
	{Name: "cache"},
}

// settings are every key of the tables, table by table in their order.
var settings = []setting{
	{Key: "list.refresh", Kind: durationKind{Off: true}, Value: func(f File) any { return f.List.Refresh },
		Doc: `Refresh every tab this often, as a Go duration such as "3m" or "90s"; "off" refreshes only when r is pressed.`},
	{Key: "list.stale_days", Display: true, Kind: daysKind{}, Value: func(f File) any { return f.List.StaleDays },
		Doc: `Mark AGE when a pull request has not been updated for this many days, or "off".`},
	{Key: "list.home_owners", Kind: listKind{}, Value: func(f File) any { return f.List.HomeOwners },
		Doc: "Owners whose name is left out of the PR column, such as your company."},
	{Key: "list.columns", Display: true, Kind: listKind{Values: Columns, NonEmpty: true}, Value: func(f File) any { return f.List.Columns },
		Doc: `The columns shown, in the order given; "pr" and "title" are needed. WHY shows when a row has something in it (whom a review is asked of, the reason of a notification), and AUTHOR not on a tab of your own pull requests. On a narrow terminal columns go before TITLE gets narrow: SIZE first, then AUTHOR, MERGE, WHY, AGE, REVIEW and CI.`},
	{Key: "list.scope", Kind: optEnumKind{Scopes}, Value: func(f File) any { return f.List.Scope }, Example: `"owner"`,
		Doc: `Narrow every tab to one owner, repository ("repo") or author at a time, chosen on the header with < and >, or from a list with O; left out, the tabs are not narrowed. The one chosen is kept for the next launch.`},
	{Key: "list.bell", Display: true, Kind: boolKind{}, Value: func(f File) any { return f.List.Bell },
		Doc: "Ring the terminal's bell when a reading of the inbox finds a notification newly unread: new, read before, or updated since. The first reading after start does not ring it, nor one of the excluded. The terminal rings, flashes or stays quiet as it is set to."},

	{Key: "menu.actions", Kind: listKind{Values: MenuActions, NonEmpty: true, Renamed: map[string]string{"ready_for_review": "draft"}}, Value: func(f File) any { return f.Menu.Actions },
		Doc: `The actions the menu of enter offers, in the order given. One left out is not in the menu; its key still runs it from the list.`},

	{Key: "preview.height", Display: true, Kind: intKind{Min: 0}, Value: func(f File) any { return f.Preview.Height },
		Doc: "Lines of the preview pane under the list; 0 hides it until it is shown with a key. Dragging the pane's top edge, or + and -, changes them, and the lines set that way are kept over this one."},
	{Key: "preview.tab", Display: true, Kind: enumKind{PaneTabs}, Value: func(f File) any { return f.Preview.Tab },
		Doc: `The tab the pane opens on: "overview" (default), the description beside a column of facts, or "diff".`},

	{Key: "description.style", Display: true, Kind: enumKind{DescriptionStyles}, Value: func(f File) any { return f.Description.Style },
		Doc: `How a description and its comments are drawn in the pane: "rendered" (default) draws headings, lists, code, quotes and tables; "plain" shows the Markdown as written.`},

	{Key: "diff.collapse", Display: true, Kind: globsKind{}, Value: func(f File) any { return f.Diff.Collapse },
		Doc: "Files shown folded, as their name and line counts, until unfolded: glob patterns, matched against the base name unless they have a slash."},

	{Key: "checks.approval", Kind: globsKind{}, Value: func(f File) any { return f.Checks.Approval },
		Doc: "Checks that pass only after an approval, such as a policy bot's: glob patterns of check names. While only they are pending, CI shows ◌, and the checks count as passing when deciding a merge."},
	{Key: "checks.wait_timeout", Kind: durationKind{}, Value: func(f File) any { return f.Checks.WaitTimeout },
		Doc: "How long a merge, or an approval asked to wait, waits for the jobs still at work on the pull request's head, required or not; an admin merge also waits for the required checks within it. Only the time gh-desk is running counts."},

	{Key: "review.mark_done", Kind: boolKind{}, Value: func(f File) any { return f.Review.MarkDone },
		Doc: "Mark a pull request's notification done after you review it here, from any tab."},
	{Key: "review.editor", Kind: stringKind{}, Value: func(f File) any { return f.Review.Editor }, Example: `"code --wait"`,
		Doc: `The editor ctrl+e opens to write a review, with its arguments, such as "code --wait". Left out, $VISUAL, then $EDITOR.`},

	{Key: "merge.methods", Kind: listKind{Values: MergeMethods, NonEmpty: true}, Value: func(f File) any { return f.Merge.Methods },
		Doc: `The order of preference among the methods a repository allows: "merge", "squash", "rebase". The first one the repository allows is used.`},
	{Key: "merge.auto_merge", Kind: boolKind{}, Value: func(f File) any { return f.Merge.AutoMerge },
		Doc: "Use GitHub's auto-merge where the repository allows it; elsewhere gh-desk waits for the checks and merges when they pass. Commits pushed after auto-merge is turned on are merged too, as GitHub does."},
	{Key: "merge.blocked_timeout", Kind: durationKind{}, Value: func(f File) any { return f.Merge.BlockedTimeout },
		Doc: "Where gh-desk waits, how long a pull request may stay blocked by another rule (a second approval, code owners) after its jobs finished. Only the time gh-desk is running counts."},
	{Key: "merge.update_branch", Kind: enumKind{UpdateBranchValues}, Value: func(f File) any { return f.Merge.UpdateBranch },
		Doc: `Whose branch is updated when it is behind its base: "humans" (default), "all" or "never". Bots rebase their own branches, and some (renovate) stop updating a branch someone else committed to.`},

	{Key: "bulk.attention_paths", Kind: globsKind{}, Value: func(f File) any { return f.Bulk.AttentionPaths },
		Doc: "Pull requests changing files that match these glob patterns get a mark in the confirm popup, which names the patterns matched; a pattern of one file is named by the file name, as CODEOWNERS."},

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
	{Key: "claude.mcp.builtin", Kind: optBoolKind{}, Value: func(f File) any { return f.Claude.MCP.Builtin }, Example: "true",
		Doc: "Give the conversation gh-desk's own MCP server, whose tools read the pull request's repository and nothing else: its checks and CI logs, its issues and pull requests. They only read, without a prompt, what anyone who sees the pull request can see. Left out, it is on, unless allowed_tools has WebFetch or servers has one: either could send what the tools read out of the conversation, so then write true or false."},
	{Key: "claude.mcp.servers", Kind: mcpServersKind{}, Value: func(f File) any { return f.Claude.MCP.Servers },
		Example: `{ recall = { command = "recall", args = ["mcp", "--repo", "${GH_DESK_URL}"], tools = ["recall_search", "recall_list", "recall_export"] } }`,
		Doc:     "Your MCP servers for the conversation about a pull request, by name, written as Claude Code configures a server (command, args, env; or type, url, headers), with tools: the server's tools that may run without a prompt. A server started with a command runs in the cache, not in the worktree. ${GH_DESK_OWNER}, ${GH_DESK_REPO}, ${GH_DESK_NUMBER} and ${GH_DESK_URL} (the repository's URL) are the pull request; pass secrets as ${VAR} from your environment. What the pull request says may call these tools: a server that reads private data, together with WebFetch, lets it send the data out."},

	{Key: "claude.plugins", Kind: listKind{}, Value: func(f File) any { return f.Claude.Plugins },
		Example: `["~/.claude/plugins/cache/<marketplace>/<plugin>/<version>"]`,
		Doc:     "Plugins whose skills the conversation about a pull request takes, as their directories (~ is your home). Only the skills are taken, copied when gh-desk starts, as gh-desk:<name>; the rest of a plugin (hooks, MCP servers, language servers, commands) is not. A skill whose allowed-tools grants a tool outside allowed_tools, Skill and the tools of the MCP servers is a mistake."},
	{Key: "theme.dark", Display: true, Kind: enumKind{Themes}, Value: func(f File) any { return f.Theme.Dark },
		Doc: `The color scheme on a dark terminal, and on one that does not tell its background.`},
	{Key: "theme.light", Display: true, Kind: enumKind{Themes}, Value: func(f File) any { return f.Theme.Light },
		Doc: `The color scheme on a light terminal. Write the same theme as theme.dark to use it on any terminal.`},
	{Key: "theme.spinner", Display: true, Kind: enumKind{Spinners}, Value: func(f File) any { return f.Theme.Spinner },
		Doc: `The spinner shown while something is read: "dots" (default) ⠋⠙⠹, "dense" ⣾⣽⣻, "bounce" ⠁⠂⠄, "snake" ⠉⠙⠚, "jump" ⢄⢂⢁, "fill" ⣀⣤⣶, "sand" ⡀⣀⣤, "line" |/-\, "arc" ◜◠◝, "quarter" ◴◷◶, "block" ▖▘▝, "star" ✶✸✹, or "off", a ⋯ that does not move and is left out before a line that says what is going on. The progress pane of the log viewer turns a spinner and marks of its own, which "off" stops too.`},
	{Key: "theme.shimmer", Display: true, Kind: boolKind{}, Value: func(f File) any { return f.Theme.Shimmer },
		Doc: "Run a light over the text that says what is going on, such as refreshing… or thinking…; the screen is drawn more often while something is read."},

	{Key: "release.check", Kind: boolKind{}, Value: func(f File) any { return f.Release.Check },
		Doc: "Read gh-desk's releases on GitHub once a day, and tell in the notifications of a newer release and, after an update, of the update. Only the builds of a release read them; GH_NO_EXTENSION_UPDATE_NOTIFIER turns it off too."},

	{Key: "log.level", Kind: stringKind{}, Value: func(f File) any { return f.Log.Level },
		Doc: `How much goes to the log file: "debug", "info" (default), "warn" or "error". --debug and GH_DESK_LOG_LEVEL take precedence. gh desk log prints where the file is.`},

	{Key: "cache.dir", Kind: stringKind{NonEmpty: true}, Value: func(f File) any { return f.Cache.Dir }, Example: `"~/.cache/gh-desk"`,
		Doc: "Where gh-desk keeps its clones and the worktrees Claude reads, and the lists last read (lists.json, shown at once at the next launch) (~ is your home); by default $XDG_CACHE_HOME/gh-desk, or ~/.cache/gh-desk."},
}

// arrayTable is an array of tables of the file. Everything that lists them
// reads them here; a test keeps this and elems in step. What reads the file
// (Parse, Show) has each by hand, since their checks depend on one another.
type arrayTable struct {
	Name string
	// First writes it first in the manual, as the tabs are; the others are
	// written before the table with Rules.
	First bool
	// Words is what the manual says of it, as comments.
	Words func() string
	// Doc is what the configuration page says of it.
	Doc string
}

func (a arrayTable) manual(m Mark) string { return a.Words() + m.arrayLine(a) }

var arrayTables = []arrayTable{
	{Name: "tabs", First: true, Words: func() string { return tabsWords },
		Doc: fmt.Sprintf("The tabs, in the order shown; the keys 1 to 9 go to them. Tabs given replace the default four, and one of them must be the inbox. At most %d tabs.", MaxTabs)},
	{Name: "select", Words: selectManual,
		Doc: `The rules S offers to select pull requests, besides the built-in "approved by me". A pull request matches a rule when it meets every key the rule gives. These rules are the default; rules written in the file replace all of them, and ` + "`select = []`" + ` at the top of the file leaves none:`},
	{Name: "exclude", Words: func() string { return excludeWords },
		Doc: "Rules that leave pull requests out of every tab, until a shows them. A pull request is left out when it meets every key of one of them. There are none by default."},
	{Name: "auto_approve", Words: func() string { return autoApproveWords },
		Doc: "Rules that approve, without a confirm, the pull requests they match while gh-desk runs. There are none by default: nothing is approved unless a rule is written. A pull request an `[[exclude]]` rule matches is never approved."},
}

// elems are the keys of the arrays of tables.
var elems = map[string][]elem{
	"tabs": {
		{Key: "kind", Kind: enumKind{TabKinds}, Doc: `"search" (default), a GitHub search, or "inbox", the pull requests in your notifications.`},
		{Key: "name", Kind: stringKind{NonEmpty: true}, Doc: `The tab's name; "inbox" for the inbox by default.`},
		{Key: "query", Kind: stringKind{NonEmpty: true}, Doc: "A search's GitHub search query."},
		{Key: "sections", Kind: sectionsKind{}, Doc: `The parts a tab's rows are split into, in the order shown, each an inline table: a name (its head; without one, no head), show ("open", "folded" or "excluded") and the conditions of a rule. A row goes in the first section whose conditions it meets; one with no condition takes the rest, and what none takes goes in other. A section whose conditions wait for what is not read yet is passed over, and never leaves a row out.`},
		{Key: "sort", Kind: enumKind{TabSorts}, Doc: `"updated" (default): the most recently updated first.`},
		{Key: "limit", Kind: intKind{Min: 1}, Doc: "How many pull requests, or notifications, to read (at most 1000; 100 by default)."},
		{Key: "show_settled", Kind: boolKind{}, Doc: "The inbox shows the notifications whose pull request is merged or closed, rather than leave them out with the excluded until a shows them."},
	},
	"select": append([]elem{
		{Key: "name", Kind: stringKind{NonEmpty: true}, Doc: "The rule's name, as S lists it."},
	}, conditionElems...),
	"exclude": append([]elem{
		{Key: "name", Kind: stringKind{}, Doc: "The rule's name, which a row it leaves out says; without one, the row says the rule's conditions."},
	}, slices.DeleteFunc(slices.Clone(conditionElems), func(e elem) bool { return e.Key == "packages" })...),
	"auto_approve": append([]elem{
		{Key: "name", Kind: stringKind{}, Doc: "The rule's name, which the notifications say of what it approved; without one, they say the rule's conditions."},
		{Key: "query", Kind: stringKind{NonEmpty: true}, Doc: "The GitHub search the pull requests are found by; without it, the open pull requests that are not drafts and whose review is asked of you, in the rule's repos when each can be searched. A rule with neither query nor a condition is a mistake."},
		{Key: "dry_run", Kind: boolKind{}, Doc: "true: only tell, in the notifications, what the rule would approve."},
	}, conditionElems...),
}

// conditionElems are the conditions of a rule, which [[select]] takes.
var conditionElems = []elem{
	{Key: "repos", Kind: listKind{NonEmpty: true}, Doc: "owner/repo globs the repository must match one of; * matches any part of the owner or the name."},
	{Key: "authors", Kind: listKind{NonEmpty: true}, Doc: "The logins the author must be one of; bots without app/."},
	{Key: "packages", Kind: listKind{}, Doc: "The dependencies a renovate or dependabot pull request updates, read from its body; every one it updates must be listed."},
	{Key: "title", Kind: stringKind{}, Doc: "A regular expression the title must match, as written: a shortcode such as `:arrow_up:` is matched as itself, not as its emoji. An empty one sets no condition."},
	{Key: "max_files", Kind: intKind{Min: 1}, Doc: "The most files it may change."},
	{Key: "max_lines", Kind: intKind{Min: 1}, Doc: "The most lines it may add and delete together: the sum of the two numbers SIZE shows."},
	{Key: "min_files", Kind: intKind{Min: 1}, Doc: "The least files it may change."},
	{Key: "min_lines", Kind: intKind{Min: 1}, Doc: "The least lines it may add and delete together."},
	{Key: "updated_within_days", Kind: intKind{Min: 1, Max: MaxWithinDays}, Doc: "The most days, of 24 hours, since it was last updated: a label or a bot's comment updates it too. An old pull request updated lately matches."},
	{Key: "created_within_days", Kind: intKind{Min: 1, Max: MaxWithinDays}, Doc: "The most days, of 24 hours, since it was created."},
	{Key: "updated_before_days", Kind: intKind{Min: 1, Max: MaxWithinDays}, Doc: "The least days, of 24 hours, since it was last updated, as AGE counts them in a search tab."},
	{Key: "created_before_days", Kind: intKind{Min: 1, Max: MaxWithinDays}, Doc: "The least days, of 24 hours, since it was created."},
	{Key: "bot", Kind: boolKind{}, Doc: "true: the author must be a bot; false: a person."},
	{Key: "mine", Kind: boolKind{}, Doc: "true: it must be your own pull request; false: someone else's."},
	{Key: "draft", Kind: boolKind{}, Doc: "true: it must be a draft, which S never selects, so [[select]] does not take true; false: ready for review."},
	{Key: "state", Kind: listKind{Values: States, NonEmpty: true}, Doc: `The states it must be in one of: "open", "merged" or "closed"; S selects open ones only, so [[select]] takes "open" alone.`},
	{Key: "checks", Kind: listKind{Values: CheckStates, NonEmpty: true}, Doc: `The states its checks must be in one of: "passing" (passed, none at all, or only checks.approval pending), "failing" or "pending" (still running).`},
	{Key: "decision", Kind: listKind{Values: Decisions, NonEmpty: true}, Doc: `The review decisions REVIEW shows, one of which it must have; "none" is none, as in a repository that requires no review.`},
	{Key: "merge", Kind: listKind{Values: MergeStates, NonEmpty: true}, Doc: `Whether GitHub would let it be merged now, one of: "ready" (the merge button can be pressed), "behind" (its branch must be brought up to date), "conflict" (it conflicts with its base), "blocked" (its reviews or required checks hold it, or it is a draft) or "unknown" (GitHub is still working it out).`},
	{Key: "labels", Kind: listKind{NonEmpty: true}, Doc: "Labels one of which it must have."},
	{Key: "approved_by_me", Kind: boolKind{}, Doc: "true: your latest review must approve it, a person's pull request too, unlike the built-in \"approved by me\"; false: none of yours does."},
	{Key: "reviewed_by_me", Kind: boolKind{}, Doc: "true: you must have reviewed it, whatever the review said; false: you have not."},
	{Key: "requested", Kind: listKind{NonEmpty: true}, Doc: `Whom a review must be asked of, one of them: "me", a team as "@slug", or "@*" for any team.`},
	{Key: "reasons", Kind: listKind{Values: Reasons, NonEmpty: true}, Doc: "Reasons of a notification, one of which the row's must have; S and a rule that approves judge a pull request without one, so [[select]] and [[auto_approve]] do not take it."},
}

// checkSettings checks every setting's value against its kind.
func checkSettings(f File) Problems {
	var ps Problems
	for _, s := range settings {
		ps = append(ps, s.Kind.check(s.Key, s.Value(f))...)
	}
	return ps
}

const templateHead = `# gh-desk settings. Only what you change goes here: a setting left out takes
# its default, and follows it when a later version changes it.
# gh desk config reference prints every setting with what it does.
`

const tabsWords = `# The tabs, in the order shown; the keys 1 to 9 go to them. Tabs given here
# replace all four below, and one of them must be the inbox. A tab is a
# GitHub search, or with kind = "inbox", the pull requests in your
# notifications. sections split a tab into parts by rules: each takes the
# keys of [[select]] but packages, a pull request goes in the first section
# it meets, and one with no condition takes the rest. name is the head the
# section is shown under; a section with no name has no head. show is "open"
# (default), "folded" (folded at start) or "excluded" (counted as excluded
# until a shows them). What no section takes goes in "other" at the end, or
# in the section named "other" where it is; a section with no rows is not
# shown. reasons is read only by the inbox. sort is "updated" (default).
# limit is how many pull requests, or notifications, to read (at most 1000;
# 100 by default). The inbox hides notifications whose pull request is merged
# or closed until they are shown with a key; show_settled = true shows them
# from the start.
#
# [[tabs]]
# name = "review"
# query = "is:open is:pr review-requested:@me archived:false"
# sections = [
#   { name = "requested from you", requested = ["me"], bot = false },
#   { name = "requested from a team", requested = ["@*"], bot = false, show = "folded" },
#   { name = "other", bot = false },
#   { name = "bots", bot = true, show = "folded" },
# ]
#
# [[tabs]]
# name = "mine"
# query = "is:open is:pr author:@me archived:false"
# sections = [
#   { draft = false },
#   { draft = true },
# ]
#
# [[tabs]]
# name = "involved"
# query = "is:open is:pr involves:@me -author:@me -review-requested:@me archived:false"
# sections = [
#   { bot = false },
#   { name = "bots", bot = true, show = "folded" },
# ]
#
# [[tabs]]
# kind = "inbox"
# name = "inbox"
# show_settled = false
# sections = [
#   { name = "review requests", reasons = ["review_requested"] },
#   { name = "mentions", reasons = ["mention"] },
# ]
`

const selectWords = `# Rules S offers to select pull requests, besides "approved by me" (bot pull
# requests you approved that are still open). A pull request matches when it
# meets every key given: authors (the logins it may be by; bots without
# app/), packages (the dependencies a renovate or dependabot pull request
# updates, read from its body; every one it updates must be listed), title
# (a regular expression), max_files (1 or more), max_lines (1 or more; the
# lines added and deleted together, the sum of the two numbers SIZE shows),
# updated_within_days and created_within_days (the most days, of 24 hours,
# since it was last updated or created; a label or a bot's comment updates
# it too, and the inbox's AGE is its notification's time, not this),
# updated_before_days and created_before_days (the least days), min_files and
# min_lines (the least), repos (owner/repo globs), labels (one of them),
# requested ("me", a team as "@slug", or "@*" for any team), state ("open",
# "merged" or "closed"), checks ("passing": passed, or only checks.approval
# pending; "failing"; "pending"), decision ("approved", "changes_requested",
# "review_required", or "none" in a repository that requires no review), and
# merge ("ready" when the merge button can be pressed, "behind", "conflict",
# "blocked" or "unknown").
# The keys that take a list match one of its values, which is how to say
# "not": the states of checks but "passing" are ["failing", "pending"].
# The keys bot, mine (your own), draft, approved_by_me (a review of yours
# approves it; a person's pull request matches too) and reviewed_by_me (any
# review of yours) are true to ask for that, and false for the opposite.
`

// defaultSelect are the rules a file with no [[select]] has.
const defaultSelect = `[[select]]
name = "small"
max_lines = 300

[[select]]
name = "dependency bots"
authors = ["renovate", "dependabot"]
checks = ["passing"]

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
`

const selectExample = `# A rule on the packages a bot's pull request updates:
#
# [[select]]
# name = "safe renovate"
# authors = ["renovate"]
# packages = ["actions/checkout", "songmu/tagpr"]
# checks = ["passing"]
`

const excludeWords = `# Rules that leave pull requests out of every tab, until a shows them. They
# take the keys of [[select]] but packages, and name may be left out; a pull
# request is left out when it meets every key of one of them. To leave them
# out of one tab only, give that tab a section with show = "excluded". The
# inbox leaves out its notifications on merged or closed pull requests too,
# unless its show_settled is true.
#
# [[exclude]]
# name = "old upgrades"
# title = "^Upgrade"
# updated_before_days = 30
#
# [[exclude]]
# labels = ["wip", "do-not-merge"]
`

const autoApproveWords = `# Rules that approve, without a confirm, the pull requests they match while
# gh-desk runs. They take the keys of [[select]] but reasons, a query to find
# the pull requests by (the open ones whose review is asked of you, without
# it), and dry_run to only tell what they would approve. A pull request an
# [[exclude]] rule matches is never approved; the jobs on its head are waited
# for first.
#
# [[auto_approve]]
# name = "renovate patches"
# authors = ["renovate"]
# title = '^chore\(deps\): update .* to v?\d+\.\d+\.\d+$'
# checks = ["passing"]
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
// file written on first run names it for editors that read it (taplo).
const SchemaURL = "https://raw.githubusercontent.com/babarot/gh-desk/main/docs/config.schema.json"

// Template is the config file written on first run. It holds no setting:
// what is left out takes its default, so a default changed later still
// applies, and what each setting does is in the manual (ADR-092).
func Template() string {
	return "#:schema " + SchemaURL + "\n" + templateHead
}

// Mark says which settings are new since a version.
type Mark struct {
	Names []string
	Since string
}

func (m Mark) line(name string) string {
	if !slices.Contains(m.Names, name) {
		return ""
	}
	return "# New since " + m.Since + ".\n"
}

// head is the line naming the new settings there still are.
func (m Mark) head() string {
	known := Names()
	var names []string
	for _, n := range m.Names {
		if slices.Contains(known, n) {
			names = append(names, n)
		}
	}
	if len(names) == 0 {
		return ""
	}
	return "# New since " + m.Since + ": " + strings.Join(Collapse(names), ", ") + "\n\n"
}

// arrayLine is the line naming the new keys of the array of tables a,
// whose words are written as one, so a key has no place of its own.
func (m Mark) arrayLine(a arrayTable) string {
	var names []string
	for _, e := range elems[a.Name] {
		if name := a.Name + "." + e.Key; slices.Contains(m.Names, name) {
			names = append(names, name)
		}
	}
	if len(names) == 0 {
		return ""
	}
	return "# New since " + m.Since + ": " + strings.Join(names, ", ") + "\n"
}

// Collapse gives a table or an array of tables by its name when names has
// every setting of it, and the others as they are: a new one is told of
// as one thing, not as each of its keys.
func Collapse(names []string) []string {
	var groups [][]string
	var titles []string
	for _, t := range tables {
		var g []string
		for _, s := range settings {
			if table, _ := splitKey(s.Key); table == t.Name {
				g = append(g, s.Key)
			}
		}
		groups, titles = append(groups, g), append(titles, t.Name)
	}
	for _, a := range arrayTables {
		var g []string
		for _, e := range elems[a.Name] {
			g = append(g, a.Name+"."+e.Key)
		}
		groups, titles = append(groups, g), append(titles, a.Name)
	}
	whole := map[string]string{}
	for i, g := range groups {
		if len(g) > 0 && !slices.ContainsFunc(g, func(n string) bool { return !slices.Contains(names, n) }) {
			for _, n := range g {
				whole[n] = titles[i]
			}
		}
	}
	var out []string
	for _, n := range names {
		if t, ok := whole[n]; ok {
			n = t
		}
		if !slices.Contains(out, n) {
			out = append(out, n)
		}
	}
	return out
}

// Names are the settings by the name config reference takes: table.key,
// and for an array of tables its name and the key. A new one is told of
// after an update.
func Names() []string {
	var names []string
	for _, s := range settings {
		names = append(names, s.Key)
	}
	for _, a := range arrayTables {
		for _, e := range elems[a.Name] {
			names = append(names, a.Name+"."+e.Key)
		}
	}
	return names
}

// Manual is every setting with what it does, all of it comments under the
// headers of the tables, so that read as a file it is the defaults. It is
// made from the table each time, so it is the running version's. keys is
// the [keys] part, which the caller writes from the keymap.
func Manual(keys string, m Mark) string {
	var b strings.Builder
	b.WriteString(m.head())
	for _, a := range arrayTables {
		if a.First {
			b.WriteString(a.manual(m))
		}
	}
	for _, t := range tables {
		b.WriteString("\n")
		if t.Rules {
			for _, a := range arrayTables {
				if !a.First {
					b.WriteString(a.manual(m) + "\n")
				}
			}
		}
		writeTable(&b, t, m)
	}
	b.WriteString("\n" + keysHead + keys)
	return b.String()
}

// ManualOf is the part of the manual of name: a table, an array of tables,
// keys, or one key of any of them. An unknown name is an error.
func ManualOf(name, keys string, m Mark) (string, error) {
	if name == "keys" {
		return keysHead + keys, nil
	}
	if i := slices.IndexFunc(arrayTables, func(a arrayTable) bool { return a.Name == name }); i >= 0 {
		return arrayTables[i].manual(m), nil
	}
	var b strings.Builder
	for _, t := range tables {
		if t.Name == name || strings.HasPrefix(t.Name, name+".") {
			if b.Len() > 0 {
				b.WriteString("\n")
			}
			writeTable(&b, t, m)
		}
	}
	if b.Len() > 0 {
		return b.String(), nil
	}
	if i := slices.IndexFunc(settings, func(s setting) bool { return s.Key == name }); i >= 0 {
		table, _ := splitKey(name)
		return "[" + table + "]\n" + settingText(settings[i], Default(), m), nil
	}
	table, key := splitKey(name)
	if i := slices.IndexFunc(elems[table], func(e elem) bool { return e.Key == key }); i >= 0 {
		e := elems[table][i]
		return "[[" + table + "]]\n" + comment(e.Doc) + m.line(name) + "# " + key + ": " + kindName(e.Kind) + "\n", nil
	}
	if table == "keys" {
		for line := range strings.SplitSeq(keys, "\n") {
			if strings.HasPrefix(line, "# "+key+" = ") {
				return "[keys]\n" + line + "\n", nil
			}
		}
	}
	return "", fmt.Errorf("no table or setting %q%s", name, didYouMean(name, manualNames(keys)))
}

// manualNames are the names ManualOf takes.
func manualNames(keys string) []string {
	names := append(Names(), "keys")
	for _, a := range arrayTables {
		names = append(names, a.Name)
	}
	for _, t := range tables {
		names = append(names, t.Name)
	}
	for line := range strings.SplitSeq(keys, "\n") {
		if key, _, ok := strings.Cut(strings.TrimPrefix(line, "# "), " = "); ok {
			names = append(names, "keys."+key)
		}
	}
	return names
}

// writeTable writes table t's header, words and settings.
func writeTable(b *strings.Builder, t table, m Mark) {
	def := Default()
	b.WriteString("[" + t.Name + "]\n")
	b.WriteString(comment(t.Doc))
	for _, s := range settings {
		if table, _ := splitKey(s.Key); table == t.Name {
			b.WriteString(settingText(s, def, m))
		}
	}
}

// settingText is a setting as the manual writes it.
func settingText(s setting, def File, m Mark) string {
	_, key := splitKey(s.Key)
	v := s.Example
	if v == "" {
		v = s.Kind.toml(s.Value(def))
	}
	return comment(s.Doc) + m.line(s.Key) + "# " + key + " = " + v + "\n"
}

// selectManual is the words of [[select]] with the default rules and an
// example, as comments.
func selectManual() string {
	var b strings.Builder
	b.WriteString(selectWords + "#\n# The rules below are the default. Rules written here replace all of them;\n# to have none, write select = [] at the top of the file.\n#\n")
	for line := range strings.SplitSeq(strings.TrimRight(defaultSelect, "\n"), "\n") {
		b.WriteString(strings.TrimRight("# "+line, " ") + "\n")
	}
	return b.String() + "#\n" + selectExample
}

// comment is text as comment lines of the manual, wrapped at 76
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
			table, key := splitKey(s.Key)
			if table == t.Name {
				tp[key] = described(s.Kind.schema(), s.Doc)
			}
		}
		ts := described(map[string]any{"type": "object", "additionalProperties": false, "properties": tp}, t.Doc)
		// A table in a table (claude.mcp) is a property of its parent,
		// which comes first in tables.
		if parent, name := splitKey(t.Name); parent != "" {
			pt, _ := props[parent].(map[string]any)
			pp, _ := pt["properties"].(map[string]any)
			pp[name] = ts
			continue
		}
		props[t.Name] = ts
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
		"title":                "gh-desk config",
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
	// Before any table, where it can only be the file's own key.
	if len(f.Select) == 0 {
		b.WriteString("select = []\n\n")
	}
	if !onlyDiff || !slices.EqualFunc(f.Tabs, def.Tabs, sameTab) {
		for _, t := range f.Tabs {
			b.WriteString("[[tabs]]\n")
			if t.Inbox() {
				b.WriteString("kind = \"inbox\"\n")
			}
			fmt.Fprintf(&b, "name = %q\n", t.Name)
			if !t.Inbox() {
				fmt.Fprintf(&b, "query = %q\n", t.Query)
				fmt.Fprintf(&b, "sort = %q\n", t.Sort)
			}
			if len(t.Sections) > 0 {
				b.WriteString("sections = " + sectionsKind{}.toml(t.Sections) + "\n")
			}
			fmt.Fprintf(&b, "limit = %d\n", t.Limit)
			if t.Inbox() {
				fmt.Fprintf(&b, "show_settled = %v\n", t.ShowSettled)
			}
			b.WriteString("\n")
		}
	}
	if !onlyDiff || rulesText(f.Select) != rulesText(def.Select) {
		b.WriteString(rulesText(f.Select))
	}
	for _, e := range f.Exclude {
		b.WriteString("[[exclude]]\n")
		fmt.Fprintf(&b, "name = %q\n", e.Name)
		writeConditions(&b, e.Conditions)
		b.WriteString("\n")
	}
	for _, a := range f.AutoApprove {
		b.WriteString("[[auto_approve]]\n")
		fmt.Fprintf(&b, "name = %q\n", a.Name)
		if a.Query != "" {
			fmt.Fprintf(&b, "query = %q\n", a.Query)
		}
		if a.DryRun {
			b.WriteString("dry_run = true\n")
		}
		writeConditions(&b, a.Conditions)
		b.WriteString("\n")
	}
	for _, t := range tables {
		var lines []string
		for _, s := range settings {
			table, key := splitKey(s.Key)
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

// rulesText is rules as the file takes them.
func rulesText(rules []Select) string {
	var b strings.Builder
	for _, s := range rules {
		b.WriteString("[[select]]\n")
		fmt.Fprintf(&b, "name = %q\n", s.Name)
		writeConditions(&b, s.Conditions)
		b.WriteString("\n")
	}
	return b.String()
}

func sameTab(a, b Tab) bool {
	return a.Kind == b.Kind && a.Name == b.Name && a.Query == b.Query &&
		slices.EqualFunc(a.Sections, b.Sections, func(x, y Section) bool { return sectionTOML(x) == sectionTOML(y) }) &&
		a.Sort == b.Sort && a.Limit == b.Limit && a.ShowSettled == b.ShowSettled
}

// sectionTOML is s as an inline table on one line, as a tab's sections are
// written: its name, its conditions, and how it is shown unless it is open.
func sectionTOML(s Section) string {
	var parts []string
	if s.Name != "" {
		parts = append(parts, fmt.Sprintf("name = %q", s.Name))
	}
	var b strings.Builder
	writeConditions(&b, s.Conditions)
	for l := range strings.Lines(b.String()) {
		parts = append(parts, strings.TrimSuffix(l, "\n"))
	}
	if s.Show != "" && s.Show != "open" {
		parts = append(parts, fmt.Sprintf("show = %q", s.Show))
	}
	if len(parts) == 0 {
		return "{}"
	}
	return "{ " + strings.Join(parts, ", ") + " }"
}

// writeConditions writes the conditions c sets, as the file takes them.
func writeConditions(b *strings.Builder, c Conditions) {
	for _, l := range []struct {
		key string
		v   []string
	}{{"repos", c.Repos}, {"authors", c.Authors}, {"packages", c.Packages}} {
		if len(l.v) > 0 {
			b.WriteString(l.key + " = " + tomlList(l.v) + "\n")
		}
	}
	if c.Title != nil {
		fmt.Fprintf(b, "title = %q\n", c.Title.String())
	}
	for _, n := range []struct {
		key string
		v   int
	}{{"max_files", c.MaxFiles}, {"max_lines", c.MaxLines}, {"min_files", c.MinFiles}, {"min_lines", c.MinLines},
		{"updated_within_days", c.UpdatedWithinDays}, {"created_within_days", c.CreatedWithinDays},
		{"updated_before_days", c.UpdatedBeforeDays}, {"created_before_days", c.CreatedBeforeDays}} {
		if n.v > 0 {
			fmt.Fprintf(b, "%s = %d\n", n.key, n.v)
		}
	}
	// A flag is written when it is set, true or false.
	type flag struct {
		key string
		v   *bool
	}
	flags := func(fs ...flag) {
		for _, f := range fs {
			if f.v != nil {
				fmt.Fprintf(b, "%s = %v\n", f.key, *f.v)
			}
		}
	}
	flags(flag{"bot", c.Bot}, flag{"mine", c.Mine}, flag{"draft", c.Draft})
	for _, l := range []struct {
		key string
		v   []string
	}{{"checks", c.Checks}, {"decision", c.Decision}, {"merge", c.Merge}} {
		if len(l.v) > 0 {
			b.WriteString(l.key + " = " + tomlList(l.v) + "\n")
		}
	}
	if len(c.Labels) > 0 {
		b.WriteString("labels = " + tomlList(c.Labels) + "\n")
	}
	if len(c.States) > 0 {
		b.WriteString("state = " + tomlList(c.States) + "\n")
	}
	flags(flag{"approved_by_me", c.ApprovedByMe}, flag{"reviewed_by_me", c.ReviewedByMe})
	for _, l := range []struct {
		key string
		v   []string
	}{{"requested", c.Requested}, {"reasons", c.Reasons}} {
		if len(l.v) > 0 {
			b.WriteString(l.key + " = " + tomlList(l.v) + "\n")
		}
	}
}
