package app

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"os/exec"
	"slices"
	"strings"

	"github.com/babarot/gh-triage/internal/config"
	"github.com/babarot/gh-triage/internal/tui/keymap"
)

const configUsage = `Usage: gh triage config <command>

Commands:
  check [path]    read the config file and print its mistakes and warnings,
                  without starting; exits 1 when it has mistakes
  show [--diff]   print the settings in effect, or with --diff only those that
                  differ from the defaults
  init            write the config file with every setting at its default,
                  unless one is there
  edit            open the config file in review.editor, $VISUAL or $EDITOR,
                  then check it
  schema          print the JSON Schema of the config file

The config file is the one --config names: by default
$XDG_CONFIG_HOME/gh-triage/config.toml, or ~/.config/gh-triage/config.toml.
`

// configCommand runs gh triage config with args, on the config file at
// path. None of it reaches GitHub.
func configCommand(args []string, path string, debug bool, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, configUsage)
		return 2
	}
	switch args[0] {
	case "help", "-h", "--help":
		fmt.Fprint(stdout, configUsage)
		return 0
	case "check":
		if len(args) > 2 {
			fmt.Fprint(stderr, configUsage)
			return 2
		}
		if len(args) == 2 {
			path = args[1]
		}
		return checkConfig(path, debug, stdout)
	case "show":
		fl := flag.NewFlagSet("gh triage config show", flag.ContinueOnError)
		fl.SetOutput(stderr)
		diff := fl.Bool("diff", false, "only the settings that differ from the defaults")
		if err := fl.Parse(args[1:]); err != nil || fl.NArg() > 0 {
			return 2
		}
		cfg, km, _, err := load(path, debug, new(slog.LevelVar))
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		fmt.Fprint(stdout, strings.TrimRight(config.Show(cfg, *diff)+keysShown(km, *diff), "\n")+"\n")
		return 0
	case "init":
		if len(args) > 1 {
			fmt.Fprint(stderr, configUsage)
			return 2
		}
		switch err := config.WriteTemplate(path, KeysTemplate()); {
		case errors.Is(err, fs.ErrExist):
			fmt.Fprintf(stdout, "%s is there already; nothing was written\n", config.TildePath(path))
		case err != nil:
			fmt.Fprintf(stderr, "gh-triage: %v\n", err)
			return 1
		default:
			fmt.Fprintf(stdout, "wrote %s\n", config.TildePath(path))
		}
		return 0
	case "edit":
		if len(args) > 1 {
			fmt.Fprint(stderr, configUsage)
			return 2
		}
		return editConfig(path, debug, stdout, stderr)
	case "schema":
		if len(args) > 1 {
			fmt.Fprint(stderr, configUsage)
			return 2
		}
		stdout.Write(config.Schema())
		return 0
	}
	fmt.Fprintf(stderr, "gh-triage: unknown config command %q\n", args[0])
	fmt.Fprint(stderr, configUsage)
	return 2
}

// checkConfig prints the mistakes and warnings of the config file at
// path, as gh-triage would find them at start, and exits 1 on a mistake.
func checkConfig(path string, debug bool, stdout io.Writer) int {
	shown := config.TildePath(path)
	if _, err := os.Stat(path); errors.Is(err, fs.ErrNotExist) {
		fmt.Fprintf(stdout, "%s is not there; the defaults are used\n", shown)
		return 0
	}
	_, _, warns, err := load(path, debug, new(slog.LevelVar))
	if err != nil {
		fmt.Fprintln(stdout, err)
		return 1
	}
	if len(warns) > 0 {
		fmt.Fprintln(stdout, config.Report(path, warns))
		fmt.Fprintf(stdout, "\n%s: no mistakes, %d %s\n", shown, len(warns), plural(len(warns), "warning", "warnings"))
		return 0
	}
	fmt.Fprintf(stdout, "%s: no mistakes\n", shown)
	return 0
}

// editConfig opens the config file in the editor a review is written in,
// writing it first when it is not there, and checks it when the editor is
// closed.
func editConfig(path string, debug bool, stdout, stderr io.Writer) int {
	if err := config.WriteTemplate(path, KeysTemplate()); err != nil && !errors.Is(err, fs.ErrExist) {
		fmt.Fprintf(stderr, "gh-triage: %v\n", err)
		return 1
	}
	// The file may not read as it is; then only the environment names the
	// editor.
	set := ""
	if cfg, _, err := config.Load(path); err == nil {
		set = cfg.Review.Editor
	}
	args := strings.Fields(editor(set, os.Getenv))
	if len(args) == 0 {
		fmt.Fprintln(stderr, "gh-triage: set review.editor, $VISUAL or $EDITOR to edit the config file")
		return 1
	}
	cmd := exec.Command(args[0], append(args[1:], path)...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, stdout, stderr
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(stderr, "gh-triage: %s: %v\n", args[0], err)
		return 1
	}
	return checkConfig(path, debug, stdout)
}

// keysShown is [keys] with every action's keys in effect, or with onlyDiff
// those that differ from the defaults; "" when there are none to show.
func keysShown(km keymap.Map, onlyDiff bool) string {
	var lines []string
	for _, d := range keymap.Defaults {
		ks := km.Keys(d.Action)
		if onlyDiff && slices.Equal(ks, d.Keys) {
			continue
		}
		lines = append(lines, fmt.Sprintf("%s = %s", d.Action, keysValue(ks)))
	}
	if len(lines) == 0 {
		return ""
	}
	return "[keys]\n" + strings.Join(lines, "\n") + "\n"
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}
