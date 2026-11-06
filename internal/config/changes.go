package config

import (
	"fmt"
	"maps"
	"net/url"
	"reflect"
	"slices"
	"strconv"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// Change is a key that differs between two versions of the config file. Was
// and Now are safe to print to a terminal, and empty where the key is not
// in the file.
type Change struct {
	Key      string
	Was, Now string
	// Display is true of a setting that changes only what is drawn. All
	// else, a key nobody marked included, is confirmed before saving.
	Display bool
}

// String is the change on one line.
func (c Change) String() string {
	was, now := c.Was, c.Now
	if was == "" {
		was = "(not set)"
	}
	if now == "" {
		now = "(removed)"
	}
	return c.Key + ": " + was + " -> " + now
}

// Changes lists what differs between two config files, as read and not as
// written, so that comments and order do not count. A rule of an array of
// tables is compared whole: one edited is one removed and one added.
func Changes(old, new []byte) ([]Change, error) {
	var a, b map[string]any
	if err := toml.Unmarshal(old, &a); err != nil {
		return nil, err
	}
	if err := toml.Unmarshal(new, &b); err != nil {
		return nil, err
	}
	var out []Change
	diffValues(nil, a, b, &out)
	return out, nil
}

func diffValues(path []string, a, b any, out *[]Change) {
	ma, tableA := a.(map[string]any)
	mb, tableB := b.(map[string]any)
	if (tableA || a == nil) && (tableB || b == nil) {
		keys := slices.Collect(maps.Keys(ma))
		keys = append(keys, slices.Collect(maps.Keys(mb))...)
		slices.Sort(keys)
		for _, k := range slices.Compact(keys) {
			diffValues(append(slices.Clone(path), k), ma[k], mb[k], out)
		}
		return
	}
	switch {
	case reflect.DeepEqual(a, b):
	case isTables(a) && isTables(b):
		diffTables(path, a, b, out)
	default:
		*out = append(*out, change(path, shownValue(path, a), shownValue(path, b)))
	}
}

func isTables(v any) bool {
	list, ok := v.([]any)
	if !ok || len(list) == 0 {
		return ok
	}
	_, ok = list[0].(map[string]any)
	return ok
}

// diffTables tells which elements went and which came. Only a change of
// order is left when each is still there.
func diffTables(path []string, a, b any, out *[]Change) {
	shown := func(v any) []string {
		list, _ := v.([]any)
		s := make([]string, len(list))
		for i, e := range list {
			s[i] = shownValue(path, e)
		}
		return s
	}
	was, now := shown(a), shown(b)
	gone := without(was, now)
	came := without(now, was)
	for _, s := range gone {
		*out = append(*out, change(path, s, ""))
	}
	for _, s := range came {
		*out = append(*out, change(path, "", s))
	}
	if len(gone) == 0 && len(came) == 0 {
		*out = append(*out, change(path, "(in another order)", "(in another order)"))
	}
}

// without is a less what b has, counting each element once.
func without(a, b []string) []string {
	left := slices.Clone(b)
	var out []string
	for _, s := range a {
		if i := slices.Index(left, s); i >= 0 {
			left = slices.Delete(left, i, i+1)
			continue
		}
		out = append(out, s)
	}
	return out
}

func change(path []string, was, now string) Change {
	key := shownPath(path)
	return Change{Key: key, Was: was, Now: now, Display: slices.ContainsFunc(settings, func(s setting) bool {
		return s.Display && s.Key == key
	})}
}

// shownPath is the key as the file spells it. A dotted name that is one key
// is quoted, so that it never reads as a setting of a table.
func shownPath(path []string) string {
	s := make([]string, len(path))
	for i, k := range path {
		s[i] = k
		if !mcpName.MatchString(k) {
			s[i] = strconv.Quote(k)
		}
	}
	return strings.Join(s, ".")
}

// shownValue is v for a terminal: strconv.Quote leaves no control
// character or reordering mark to rewrite what the screen shows, and what
// may be a secret stays out of the scrollback.
func shownValue(path []string, v any) string {
	if v == nil {
		return ""
	}
	if hidden(path) {
		return "(hidden)"
	}
	switch v := v.(type) {
	case string:
		if isServerURL(path) {
			return shownURL(v)
		}
		return strconv.Quote(v)
	case []any:
		s := make([]string, len(v))
		for i, e := range v {
			s[i] = shownValue(path, e)
		}
		return "[" + strings.Join(s, ", ") + "]"
	case map[string]any:
		s := make([]string, 0, len(v))
		for _, k := range slices.Sorted(maps.Keys(v)) {
			s = append(s, shownPath([]string{k})+" = "+shownValue(append(slices.Clone(path), k), v[k]))
		}
		return "{" + strings.Join(s, ", ") + "}"
	}
	return fmt.Sprint(v)
}

// inServer says that path is under a user's MCP server, and which key of
// it.
func inServer(path []string) (string, bool) {
	if len(path) < 5 || path[0] != "claude" || path[1] != "mcp" || path[2] != "servers" {
		return "", false
	}
	return path[4], true
}

// hidden says that path holds what a server is given to sign in with: the
// name of the key is shown, as it says what changes.
func hidden(path []string) bool {
	k, ok := inServer(path)
	return ok && (k == "env" || k == "headers")
}

func isServerURL(path []string) bool {
	k, ok := inServer(path)
	return ok && k == "url"
}

// shownURL is a URL without what can sign in with it. One that cannot be
// parsed is hidden whole.
func shownURL(s string) string {
	u, err := url.Parse(s)
	if err != nil {
		return "(hidden)"
	}
	if u.User == nil && u.RawQuery == "" && u.Fragment == "" {
		return strconv.Quote(s)
	}
	u.User, u.RawQuery, u.Fragment = nil, "", ""
	return strconv.Quote(u.String()) + " (its credentials hidden)"
}
