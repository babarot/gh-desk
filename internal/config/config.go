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
	Log     Log     `toml:"log"`
	UI      UI      `toml:"ui"`
	Refresh Refresh `toml:"refresh"`
	// Keys is [keys] as written; the keymap package checks it.
	Keys Keys `toml:"-"`
}

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
		Log:     Log{Level: "info"},
		UI:      UI{Theme: "auto"},
		Refresh: Refresh{Interval: "3m"},
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

[log]
# How much goes to the log file: "debug", "info" (default), "warn" or
# "error". --debug and GH_TRIAGE_LOG_LEVEL take precedence. gh triage log
# prints where the file is.
# level = "info"

[ui]
# Color scheme: "auto" (default) picks mocha on a dark terminal and latte on a
# light one; or "mocha", "latte".
# theme = "auto"

[refresh]
# Refresh the shown tab this often, as a Go duration such as "3m" or "90s";
# "0" refreshes only when R is pressed.
# interval = "3m"

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
		Log     Log            `toml:"log"`
		UI      UI             `toml:"ui"`
		Refresh Refresh        `toml:"refresh"`
		Keys    map[string]any `toml:"keys"`
	}{Log: cfg.Log, UI: cfg.UI, Refresh: cfg.Refresh}
	if err := toml.NewDecoder(bytes.NewReader(src)).DisallowUnknownFields().Decode(&doc); err != nil {
		return File{}, Report(path, decodeProblems(err))
	}
	cfg = File{Log: doc.Log, UI: doc.UI, Refresh: doc.Refresh, Keys: keysFrom(doc.Keys)}
	var ps Problems
	if !slices.Contains(Themes, cfg.UI.Theme) {
		ps = append(ps, problem("ui.theme", "ui.theme must be one of %s, got %q", quoted(Themes), cfg.UI.Theme))
	}
	if d, err := time.ParseDuration(cfg.Refresh.Interval); err != nil || d < 0 {
		ps = append(ps, problem("refresh.interval", `refresh.interval must be a duration such as "3m", or "0", got %q`, cfg.Refresh.Interval))
	}
	if len(ps) > 0 {
		return File{}, Report(path, ps)
	}
	return cfg, nil
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
