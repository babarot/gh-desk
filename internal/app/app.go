// Package app puts gh-triage together: it reads the flags and the config
// file, opens the log, builds the keymap, and starts the TUI. It is the only
// package that knows every other one; the rest receive what they need from
// here.
package app

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/babarot/gh-triage/internal/config"
	"github.com/babarot/gh-triage/internal/keymap"
	"github.com/babarot/gh-triage/internal/logging"
	"github.com/babarot/gh-triage/internal/tui"
)

// Version is set at build time.
var Version = "dev"

// EnvLogLevel overrides [log] level in the config file.
const EnvLogLevel = "GH_TRIAGE_LOG_LEVEL"

const usage = `Usage: gh triage [flags] [command]

Commands:
  log        print where the log file is
  version    print the version

Flags:
`

// Run runs gh-triage with args (without the program name) and returns the
// exit code.
func Run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("gh-triage", flag.ContinueOnError)
	flags.SetOutput(stderr)
	configPath := flags.String("config", config.FilePath(), "the config file")
	debug := flags.Bool("debug", false, "log at debug level")
	flags.Usage = func() {
		fmt.Fprint(stderr, usage)
		flags.PrintDefaults()
	}
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	switch flags.Arg(0) {
	case "":
	case "log":
		fmt.Fprintln(stdout, logging.DefaultPath())
		return 0
	case "version":
		fmt.Fprintln(stdout, Version)
		return 0
	default:
		fmt.Fprintf(stderr, "gh-triage: unknown command %q\n", flags.Arg(0))
		flags.Usage()
		return 2
	}

	level := new(slog.LevelVar)
	log, closeLog, err := logging.Open(logging.Options{Level: level})
	if err != nil {
		fmt.Fprintf(stderr, "gh-triage: open the log file: %v\n", err)
		return 1
	}
	defer closeLog.Close()
	run := logging.NewID()
	log = log.With(logging.KeyRun, run)

	cfg, km, err := setup(*configPath, *debug, level, log)
	if err != nil {
		log.Error("setup failed", "err", err.Error())
		fmt.Fprintln(stderr, err)
		return 1
	}
	log.Info("start", logging.KeyVersion, Version, "config", *configPath, "log", logging.DefaultPath(),
		"level", level.Level().String(), "refresh", cfg.RefreshInterval().String())

	start := time.Now()
	err = tui.Run(km, log)
	log.Info("exit", logging.KeyDurationMS, time.Since(start).Milliseconds(), "err", errString(err))
	if err != nil {
		fmt.Fprintf(stderr, "gh-triage: %v (run %s)\n", err, run)
		return 1
	}
	return 0
}

// setup writes the config file on first run, reads it, sets the log level,
// and builds the keymap. A mistake anywhere in the file is reported where
// it is, all at once.
func setup(path string, debug bool, level *slog.LevelVar, log *slog.Logger) (config.File, keymap.Map, error) {
	if err := config.WriteTemplate(path, KeysTemplate()); err == nil {
		log.Info("wrote the config file", "config", path)
	} else if !errors.Is(err, fs.ErrExist) {
		log.Warn("could not write the config file", "config", path, "err", err.Error())
	}
	cfg, err := config.Load(path)
	if err != nil {
		return config.File{}, keymap.Map{}, err
	}
	var ps config.Problems
	want := cfg.Log.Level
	if env := os.Getenv(EnvLogLevel); env != "" {
		want = env
	}
	if debug {
		want = "debug"
	}
	if l, err := logging.ParseLevel(want); err != nil {
		if want == cfg.Log.Level {
			ps = append(ps, config.SettingProblem("log.level", "log.level: %v", err))
		} else {
			return config.File{}, keymap.Map{}, fmt.Errorf("%s: %w", EnvLogLevel, err)
		}
	} else {
		level.Set(l)
	}
	km, kps := applyKeys(cfg.Keys)
	ps = append(ps, kps...)
	if len(ps) > 0 {
		return config.File{}, keymap.Map{}, config.Report(path, ps)
	}
	return cfg, km, nil
}

// applyKeys gives the actions under [keys] their keys, turning what the
// keymap finds wrong into problems placed in the config file.
func applyKeys(set config.Keys) (keymap.Map, config.Problems) {
	var ps config.Problems
	keys := map[string][]string{}
	for name, kl := range set {
		if kl.Err != nil {
			ps = append(ps, config.Problem{Key: []string{"keys", name}, Index: -1, Message: fmt.Sprintf("keys.%s %v", name, kl.Err)})
			continue
		}
		keys[name] = kl.Keys
	}
	km, kps := keymap.Apply(keymap.Default(), keys)
	for _, p := range kps {
		ps = append(ps, config.Problem{Key: []string{"keys", p.Action}, Index: p.Index, AtKey: p.AtKey, Message: p.Message})
	}
	if len(ps) > 0 {
		return keymap.Map{}, ps
	}
	return km, nil
}

// KeysTemplate is the [keys] part of the config file written on first run:
// every action at its default keys, commented out, scope by scope.
func KeysTemplate() string {
	var b strings.Builder
	scope := keymap.Scope("")
	for _, d := range keymap.Defaults {
		if d.Scope != scope {
			if scope != "" {
				b.WriteString("#\n")
			}
			scope = d.Scope
			fmt.Fprintf(&b, "# %s:\n", scopeWords[scope])
		}
		fmt.Fprintf(&b, "# %s = %s\n", d.Action, keysValue(d.Keys))
	}
	return b.String()
}

var scopeWords = map[keymap.Scope]string{
	keymap.ScopeGlobal:  "Anywhere",
	keymap.ScopeNav:     "Moving, in every list and pane",
	keymap.ScopeList:    "The list and its preview pane",
	keymap.ScopePR:      "The pull request screen",
	keymap.ScopePopup:   "A confirm popup",
	keymap.ScopeChat:    "The Claude screen",
	keymap.ScopeCompose: "Writing a review",
}

// keysValue writes keys as TOML: a string for one key, an array otherwise.
func keysValue(keys []string) string {
	if len(keys) == 1 {
		return fmt.Sprintf("%q", keys[0])
	}
	q := make([]string, len(keys))
	for i, k := range keys {
		q[i] = fmt.Sprintf("%q", k)
	}
	return "[" + strings.Join(q, ", ") + "]"
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
