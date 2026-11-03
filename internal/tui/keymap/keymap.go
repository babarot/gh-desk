// Package keymap binds keys to actions by name. The TUI never writes a key
// in its code: it asks for an action ("approve", "merge") and draws the key
// hints and the ? list from here, so every key can be changed under [keys]
// in the config file.
//
// Keys are written as Bubble Tea reports key presses: "a", "A", "enter",
// "ctrl+s", "shift+tab".
package keymap

import (
	"fmt"
	"slices"
	"strings"

	"github.com/babarot/gh-desk/internal/near"
)

// Action is something a key does, named as it is under [keys].
type Action string

// Scope is where an action's keys are read. An action in ScopeGlobal or
// ScopeNav is read in every other scope too, so its keys may not be used by
// any other action; an action in another scope only shares its scope.
type Scope string

const (
	ScopeGlobal  Scope = "global"
	ScopeNav     Scope = "nav"
	ScopeList    Scope = "list"
	ScopePopup   Scope = "popup"
	ScopeConfirm Scope = "confirm"
	ScopeTriage  Scope = "triage"
	ScopeChat    Scope = "chat"
	ScopeCompose Scope = "compose"
	ScopeInput   Scope = "input"
)

// Def is an action with its default keys and the words the key hints show.
type Def struct {
	Action Action
	Scope  Scope
	Keys   []string
	Help   string
}

// Defaults are every action with its keys, in the order the ? list shows
// them.
var Defaults = []Def{
	{"quit", ScopeGlobal, []string{"q"}, "quit"},
	{"help", ScopeGlobal, []string{"?"}, "keys"},
	{"refresh", ScopeGlobal, []string{"r"}, "refresh"},
	{"events", ScopeGlobal, []string{"n"}, "notifications"},

	{"up", ScopeNav, []string{"up", "k"}, "up"},
	{"down", ScopeNav, []string{"down", "j"}, "down"},
	{"top", ScopeNav, []string{"home", "g"}, "top"},
	{"bottom", ScopeNav, []string{"end", "G"}, "bottom"},
	{"page_up", ScopeNav, []string{"pgup", "ctrl+u"}, "page up"},
	{"page_down", ScopeNav, []string{"pgdown", "ctrl+d"}, "page down"},

	{"next_tab", ScopeList, []string{"tab"}, "next tab"},
	{"prev_tab", ScopeList, []string{"shift+tab"}, "previous tab"},
	{"tab_1", ScopeList, []string{"1"}, "tab 1"},
	{"tab_2", ScopeList, []string{"2"}, "tab 2"},
	{"tab_3", ScopeList, []string{"3"}, "tab 3"},
	{"tab_4", ScopeList, []string{"4"}, "tab 4"},
	{"tab_5", ScopeList, []string{"5"}, "tab 5"},
	{"tab_6", ScopeList, []string{"6"}, "tab 6"},
	{"tab_7", ScopeList, []string{"7"}, "tab 7"},
	{"tab_8", ScopeList, []string{"8"}, "tab 8"},
	{"tab_9", ScopeList, []string{"9"}, "tab 9"},
	{"menu", ScopeList, []string{"enter"}, "actions"},
	{"expand", ScopeList, []string{"l", "right"}, "unfold group / next pane tab"},
	{"collapse", ScopeList, []string{"h", "left"}, "fold group / previous pane tab"},
	{"select", ScopeList, []string{"space"}, "select"},
	{"select_shown", ScopeList, []string{"V"}, "select shown"},
	{"select_rule", ScopeList, []string{"S"}, "select by rule"},
	{"clear_selection", ScopeList, []string{"u"}, "clear selection"},
	{"approve", ScopeList, []string{"A"}, "approve"},
	{"merge", ScopeList, []string{"M"}, "merge"},
	{"ready_for_review", ScopeList, []string{"D"}, "ready for review"},
	{"comment", ScopeList, []string{"c"}, "comment"},
	{"review", ScopeList, []string{"R"}, "review"},
	{"triage", ScopeList, []string{"C"}, "triage with claude"},
	{"ask_claude", ScopeList, []string{"i"}, "ask claude about it"},
	{"browse", ScopeList, []string{"o"}, "browser"},
	{"hide", ScopeList, []string{"d"}, "hide / done"},
	{"fold_bots", ScopeList, []string{"b"}, "fold bots"},
	{"show_settled", ScopeList, []string{"a"}, "settled"},
	{"filter", ScopeList, []string{"/"}, "filter"},
	{"preview_down", ScopeList, []string{"J"}, "scroll pane down"},
	{"preview_up", ScopeList, []string{"K"}, "scroll pane up"},
	{"next_file", ScopeList, []string{"]"}, "next file tab / part"},
	{"prev_file", ScopeList, []string{"["}, "previous file tab / part"},
	{"preview_tab", ScopeList, []string{"v"}, "overview / diff"},
	{"fold", ScopeList, []string{"z"}, "fold file / part"},
	{"preview", ScopeList, []string{"p"}, "preview pane"},
	{"preview_grow", ScopeList, []string{"+"}, "taller pane"},
	{"preview_shrink", ScopeList, []string{"-"}, "shorter pane"},

	{"choose", ScopePopup, []string{"enter"}, "choose"},
	{"toggle_rule", ScopePopup, []string{"space"}, "check a rule"},
	{"add_to_selection", ScopePopup, []string{"a"}, "add to the selection"},
	{"filter_by_rule", ScopePopup, []string{"f"}, "show only these"},

	// A confirm popup reads no choose, so enter answers it too.
	{"confirm", ScopeConfirm, []string{"y", "enter"}, "yes"},
	{"include_failing", ScopeConfirm, []string{"f"}, "include failing"},

	{"triage_apply", ScopeTriage, []string{"enter"}, "apply"},
	{"triage_more", ScopeTriage, []string{"i"}, "ask more"},

	// The text box has the focus while nothing is on its way, so these
	// keys are not ones it types.
	{"fill_suggestion", ScopeChat, []string{"tab"}, "fill in the suggestion"},
	{"draft_review", ScopeChat, []string{"ctrl+r"}, "draft review"},
	{"open_claude", ScopeChat, []string{"ctrl+o"}, "open in claude code"},
	{"chat_restart", ScopeChat, []string{"ctrl+l"}, "start over"},
	// Read while Claude answers, when there is no text box.
	{"chat_close", ScopeChat, []string{"backspace"}, "back, claude goes on"},

	{"next_kind", ScopeCompose, []string{"tab"}, "kind"},
	{"post", ScopeCompose, []string{"ctrl+s"}, "post"},
	{"editor", ScopeCompose, []string{"ctrl+e"}, "$EDITOR"},

	{"accept", ScopeInput, []string{"enter"}, "accept"},
	{"erase", ScopeInput, []string{"backspace"}, "erase"},
	{"erase_word", ScopeInput, []string{"ctrl+w"}, "erase word"},
}

// renamed are the actions that were renamed, by their old name. A [keys]
// entry under an old name is a mistake that names the new one; the old name
// is not read (ADR-023).
var renamed = map[string]Action{
	"open":          "menu",
	"claude":        "triage",
	"pr_claude":     "ask_claude",
	"unfold":        "fold",
	"toggle_bots":   "fold_bots",
	"toggle_pane":   "preview",
	"pane_tab":      "preview_tab",
	"pane_grow":     "preview_grow",
	"pane_shrink":   "preview_shrink",
	"pane_down":     "preview_down",
	"pane_up":       "preview_up",
	"notifications": "events",
}

// The fixed keys are read before any action and cannot be bound.
const (
	// Interrupt quits from anywhere.
	Interrupt = "ctrl+c"
	// Back steps back one level everywhere; at the top it only says how to quit.
	Back = "esc"
)

// Fixed are the fixed keys and what each does.
var Fixed = map[string]string{
	Interrupt: "quits from anywhere",
	Back:      "steps back one level everywhere",
}

// Map is the keys of every action, after the config file.
type Map struct {
	defs map[Action]Def
	keys map[Action][]string
}

// Default is the map with every action at its default keys.
func Default() Map {
	m := Map{defs: map[Action]Def{}, keys: map[Action][]string{}}
	for _, d := range Defaults {
		m.defs[d.Action] = d
		m.keys[d.Action] = slices.Clone(d.Keys)
	}
	return m
}

// Keys are action's keys; none for an action that does not exist or was
// turned off.
func (m Map) Keys(a Action) []string { return slices.Clone(m.keys[a]) }

// Help is the words the key hints show for action.
func (m Map) Help(a Action) string { return m.defs[a].Help }

// Matches reports whether a key press, as Bubble Tea writes it, is one of
// action's keys.
func (m Map) Matches(a Action, pressed string) bool {
	return slices.Contains(m.keys[a], pressed)
}

// Actions are the actions of scope, in the order of Defaults.
func (m Map) Actions(s Scope) []Action {
	var as []Action
	for _, d := range Defaults {
		if d.Scope == s {
			as = append(as, d.Action)
		}
	}
	return as
}

// Problem is a mistake in what [keys] sets: Action is the setting, Index the
// key in its list (or -1 for the whole value), AtKey whether the action's
// name itself is the mistake.
type Problem struct {
	Action  string
	Index   int
	AtKey   bool
	Message string
}

// Apply returns base with the actions in set given those keys, replacing
// their own; [] turns an action off. A key set for an action is taken away
// from any action that has it by default and would meet it, so A = "x"
// does not leave x also on its default owner. Every mistake is returned
// rather than ignored, since a key that never matches looks like a bug: an
// unknown action, a key no key press is read as, a fixed key, and two
// actions set to the same key where they meet.
func Apply(base Map, set map[string][]string) (Map, []Problem) {
	m := Map{defs: base.defs, keys: map[Action][]string{}}
	for a, ks := range base.keys {
		m.keys[a] = slices.Clone(ks)
	}
	names := make([]string, 0, len(set))
	for n := range set {
		names = append(names, n)
	}
	slices.Sort(names)
	var ps []Problem
	for _, n := range names {
		a := Action(n)
		if to, ok := renamed[n]; ok {
			ps = append(ps, Problem{Action: n, Index: -1, AtKey: true, Message: fmt.Sprintf("keys.%s: %q was renamed to %q", n, n, to)})
			continue
		}
		if _, ok := m.defs[a]; !ok {
			ps = append(ps, Problem{Action: n, Index: -1, AtKey: true, Message: fmt.Sprintf("keys.%s: %s", n, unknownAction(n))})
			continue
		}
		bad := false
		for i, k := range set[n] {
			if err := CheckKey(k); err != nil {
				ps = append(ps, Problem{Action: n, Index: i, Message: fmt.Sprintf("keys.%s: %v", n, err)})
				bad = true
			}
		}
		if !bad {
			m.keys[a] = slices.Clone(set[n])
		}
	}
	if len(ps) > 0 {
		return Map{}, ps
	}
	// A key set in the file wins over a default it meets.
	for _, c := range m.conflicts() {
		_, firstSet := set[string(c.first)]
		_, secondSet := set[string(c.second)]
		if firstSet == secondSet {
			continue
		}
		loser := c.second
		if secondSet {
			loser = c.first
		}
		m.keys[loser] = slices.DeleteFunc(m.keys[loser], func(k string) bool { return k == c.key })
	}
	for _, c := range m.conflicts() {
		ps = append(ps, Problem{Action: string(c.second), Index: slices.Index(m.keys[c.second], c.key),
			Message: fmt.Sprintf("keys.%s: %q is also %s's key", c.second, c.key, c.first)})
	}
	if len(ps) > 0 {
		return Map{}, ps
	}
	return m, nil
}

type conflict struct {
	key           string
	first, second Action
}

// meets reports whether keys of actions in scopes a and b can be read at the
// same time.
func meets(a, b Scope) bool {
	wide := func(s Scope) bool { return s == ScopeGlobal || s == ScopeNav }
	return a == b || wide(a) || wide(b)
}

// conflicts are the keys two actions share where they meet, in the order of
// Defaults.
func (m Map) conflicts() []conflict {
	var cs []conflict
	for i, a := range Defaults {
		for _, b := range Defaults[i+1:] {
			if !meets(a.Scope, b.Scope) {
				continue
			}
			for _, k := range m.keys[a.Action] {
				if slices.Contains(m.keys[b.Action], k) {
					cs = append(cs, conflict{key: k, first: a.Action, second: b.Action})
				}
			}
		}
	}
	return cs
}

// unknownAction says an action does not exist, naming a close one when
// there is.
func unknownAction(name string) string {
	names := make([]string, len(Defaults))
	for i, d := range Defaults {
		names[i] = string(d.Action)
	}
	if a, ok := near.Closest(name, names); ok {
		return fmt.Sprintf("%q is not an action; did you mean %q?", name, a)
	}
	for _, d := range Defaults {
		if a := string(d.Action); strings.Contains(a, name) || strings.Contains(name, a) {
			return fmt.Sprintf("%q is not an action; did you mean %q?", name, a)
		}
	}
	return fmt.Sprintf("%q is not an action", name)
}
