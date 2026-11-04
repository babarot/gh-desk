package app

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"os/exec"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/babarot/gh-desk/internal/config"
	"github.com/babarot/gh-desk/internal/tui/keymap"
)

// newConfigCommand is gh desk config. None of it reaches GitHub.
func newConfigCommand(o *options, stdout, stderr io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "check, show or edit the config file",
		Long: `Check, show or edit the config file.

The config file is the one --config names: by default
$XDG_CONFIG_HOME/gh-desk/config.toml, or ~/.config/gh-desk/config.toml.`,
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			switch {
			case len(args) == 0:
				return errors.New("config needs a command")
			case len(args) == 1 && args[0] == "help":
				return cmd.Help()
			}
			return fmt.Errorf("unknown config command %q", args[0])
		},
	}

	var diff bool
	show := &cobra.Command{
		Use:   "show",
		Short: "print the settings in effect, or with --diff only those that differ from the defaults",
		Args:  cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			cfg, km, _, err := load(o.config, o.debug, new(slog.LevelVar))
			if err != nil {
				fmt.Fprintln(stderr, err)
				return exitCode(1)
			}
			fmt.Fprint(stdout, strings.TrimRight(config.Show(cfg, diff)+keysShown(km, diff), "\n")+"\n")
			return nil
		},
	}
	show.Flags().BoolVar(&diff, "diff", false, "only the settings that differ from the defaults")

	cmd.AddCommand(
		&cobra.Command{
			Use:   "check [path]",
			Short: "read the config file and print its mistakes and warnings, without starting; exits 1 when it has mistakes",
			Args:  cobra.MaximumNArgs(1),
			RunE: func(_ *cobra.Command, args []string) error {
				path := o.config
				if len(args) == 1 {
					path = args[0]
				}
				return exited(checkConfig(path, o.debug, stdout))
			},
		},
		show,
		&cobra.Command{
			Use:   "init",
			Short: "write the config file with every setting at its default, unless one is there",
			Args:  cobra.NoArgs,
			RunE: func(*cobra.Command, []string) error {
				switch err := config.WriteTemplate(o.config, KeysTemplate()); {
				case errors.Is(err, fs.ErrExist):
					fmt.Fprintf(stdout, "%s is there already; nothing was written\n", config.TildePath(o.config))
				case err != nil:
					fmt.Fprintf(stderr, "gh-desk: %v\n", err)
					return exitCode(1)
				default:
					fmt.Fprintf(stdout, "wrote %s\n", config.TildePath(o.config))
				}
				return nil
			},
		},
		&cobra.Command{
			Use:   "edit",
			Short: "open the config file in review.editor, $VISUAL or $EDITOR, then check it",
			Args:  cobra.NoArgs,
			RunE: func(*cobra.Command, []string) error {
				return exited(editConfig(o.config, o.debug, stdout, stderr))
			},
		},
		&cobra.Command{
			Use:   "schema",
			Short: "print the JSON Schema of the config file",
			Args:  cobra.NoArgs,
			RunE: func(*cobra.Command, []string) error {
				stdout.Write(config.Schema())
				return nil
			},
		},
	)
	return cmd
}

// checkConfig prints the mistakes and warnings of the config file at
// path, as gh-desk would find them at start, and exits 1 on a mistake.
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
		fmt.Fprintf(stderr, "gh-desk: %v\n", err)
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
		fmt.Fprintln(stderr, "gh-desk: set review.editor, $VISUAL or $EDITOR to edit the config file")
		return 1
	}
	cmd := exec.Command(args[0], append(args[1:], path)...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, stdout, stderr
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(stderr, "gh-desk: %s: %v\n", args[0], err)
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
