package app

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"slices"
	"strings"

	"github.com/cli/go-gh/v2/pkg/browser"
	"github.com/spf13/cobra"

	"github.com/babarot/gh-desk/internal/config"
	"github.com/babarot/gh-desk/internal/config/ui"
	"github.com/babarot/gh-desk/internal/release"
	"github.com/babarot/gh-desk/internal/state"
	"github.com/babarot/gh-desk/internal/tui"
	"github.com/babarot/gh-desk/internal/tui/keymap"
)

// newConfigCommand is gh desk config. None of it reaches GitHub.
func newConfigCommand(o *options, stdout, stderr io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "check, show or edit the config file, and print what each setting does",
		Long: `Check, show or edit the config file, and print what each setting does.

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

	var onPage bool
	edit := &cobra.Command{
		Use:   "edit",
		Short: "open the config file in review.editor, $VISUAL or $EDITOR, then check it; with --ui, on a page in the browser",
		Long: `Open the config file in review.editor, $VISUAL or $EDITOR, then check it.

With --ui, open a page in the browser instead, with the settings of the
file and gh-desk drawn as they change. It is served on this machine for as
long as this runs. Saving on the page checks the file as gh-desk checks it
at start and replaces it, keeping the old one beside it, with .bak added to
its name. Comments do not carry over. A change to anything but how things
are drawn is listed here first, and saved only if it is answered with y.

Closing the page, or ctrl+c, leaves without saving, and writes nothing.`,
		Args: cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			if !onPage {
				return exited(editConfig(o.config, o.debug, stdout, stderr))
			}
			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
			defer stop()
			return exited(uiConfig(ctx, o.config, o.debug, terminalConfirmer(os.Stdin, stderr), openPage, stdout, stderr))
		},
	}
	edit.Flags().BoolVar(&onPage, "ui", false, "edit the settings on a page in the browser")

	apply := &cobra.Command{
		Use:   "apply",
		Short: "replace the config file with the one read from standard input, if it has no mistakes",
		Long: `Replace the config file with the one read from standard input.

The file is checked as gh-desk checks it at start, and nothing is written
when it has mistakes. The file it replaces is kept beside it, with .bak
added to its name.`,
		Args: cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			if st, err := os.Stdin.Stat(); err == nil && st.Mode()&os.ModeCharDevice != 0 {
				fmt.Fprintln(stderr, "gh-desk: config apply reads the file from standard input, as in: pbpaste | gh desk config apply")
				return exitCode(1)
			}
			return exited(applyConfig(o.config, o.debug, os.Stdin, stdout, stderr))
		},
	}

	cmd.AddCommand(
		apply,
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
			Short: "write the config file, which holds no settings yet, unless one is there",
			Args:  cobra.NoArgs,
			RunE: func(*cobra.Command, []string) error {
				switch err := config.WriteTemplate(o.config); {
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
		edit,
		&cobra.Command{
			Use:   "reference [table or key]",
			Short: "print every setting with what it does and its default, or those of a table or a key",
			Long: `Print every setting with what it does and its default, as comments, from
this version of gh-desk. The config file is not read.

A name narrows it to a table (merge), an array of tables (select), keys, or
one key of any of them (list.refresh, select.max_lines, keys.approve).`,
			Args: cobra.MaximumNArgs(1),
			RunE: func(_ *cobra.Command, args []string) error {
				if len(args) == 0 {
					fmt.Fprint(stdout, config.Manual(KeysManual(), newSettings(state.DefaultDir())))
					return nil
				}
				text, err := config.ManualOf(args[0], KeysManual(), newSettings(state.DefaultDir()))
				if err != nil {
					fmt.Fprintf(stderr, "gh-desk: %v\n", err)
					return exitCode(1)
				}
				fmt.Fprint(stdout, text)
				return nil
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

// newSettings are the settings to mark as new, kept under dir by the
// update that found them. The manual reads as well without them, so a
// record that cannot be read marks none.
func newSettings(dir string) config.Mark {
	rec, err := release.Open(dir).Load()
	if err != nil || len(rec.New) == 0 {
		return config.Mark{}
	}
	return config.Mark{Names: rec.New, Since: "v" + strings.TrimPrefix(rec.NewSince, "v")}
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

// confirmer asks a question and says whether it was answered yes.
type confirmer func(ctx context.Context, question string) (bool, error)

// errNoTerminal is why a change that must be confirmed cannot be.
var errNoTerminal = errors.New("this change must be confirmed in a terminal, and gh desk has none: run gh desk config edit --ui in one, or use gh desk config apply")

// terminalConfirmer asks on the terminal standard input is. Only someone at
// that terminal can answer, which a request to the page cannot do.
func terminalConfirmer(in *os.File, stderr io.Writer) confirmer {
	return func(ctx context.Context, question string) (bool, error) {
		if st, err := in.Stat(); err != nil || st.Mode()&os.ModeCharDevice == 0 {
			return false, errNoTerminal
		}
		return askLine(ctx, in, question, stderr)
	}
}

// askLine reads one answer. Only y or yes, with its newline, is yes: no
// answer, the end of the input or anything else leaves the file alone.
func askLine(ctx context.Context, in io.Reader, question string, stderr io.Writer) (bool, error) {
	fmt.Fprintf(stderr, "%s [y/N] ", question)
	line := make(chan string, 1)
	go func() {
		s, err := bufio.NewReader(in).ReadString('\n')
		if err != nil {
			s = ""
		}
		line <- s
	}()
	select {
	case s := <-line:
		s = strings.ToLower(strings.TrimSpace(s))
		return s == "y" || s == "yes", nil
	case <-ctx.Done():
		fmt.Fprintln(stderr)
		return false, ctx.Err()
	}
}

// confirmation is what is asked before changes are saved, or "" when they
// change only how things are drawn.
func confirmation(path string, changes []config.Change) string {
	var b strings.Builder
	display := 0
	for _, c := range changes {
		if c.Display {
			display++
			continue
		}
		fmt.Fprintf(&b, "  %s\n", c)
	}
	if b.Len() == 0 {
		return ""
	}
	if display > 0 {
		fmt.Fprintf(&b, "  and %d that change only how things are drawn\n", display)
	}
	return fmt.Sprintf("\nthe page asks to save:\n\n%s\nsave them to %s?", b.String(), config.TildePath(path))
}

// uiConfig edits the settings on a page. Saving there replaces the file
// with what the terminal confirmed, byte for byte, as config apply does.
func uiConfig(ctx context.Context, path string, debug bool, ask confirmer, open func(string) error, stdout, stderr io.Writer) int {
	src, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		fmt.Fprintf(stderr, "gh-desk: %v\n", err)
		return 1
	}
	exists := err == nil
	if exists {
		if _, _, _, err := load(path, debug, new(slog.LevelVar)); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
	}
	page, err := ui.Page(ui.Input{File: ui.File{Path: config.TildePath(path), Src: src}, Keys: uiKeys(),
		Themes: tui.Palettes(), Spinners: tui.Spinners()})
	if err != nil {
		fmt.Fprintf(stderr, "gh-desk: %v\n", err)
		return 1
	}
	// What the file was read as is what the page changed.
	unchanged := func(now []byte, there bool) error {
		if !bytes.Equal(now, src) || there != exists {
			return fmt.Errorf("%s changed while this page was open. Run gh desk config edit --ui again, or your change would be lost", config.TildePath(path))
		}
		return nil
	}
	srv := &ui.Server{Page: page, Save: func(ctx context.Context, made []byte) error {
		now, err := os.ReadFile(path)
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		if err := unchanged(now, err == nil); err != nil {
			return err
		}
		if _, err := checkedFile(path, made, "", debug); err != nil {
			return err
		}
		changes, err := config.Changes(src, made)
		if err != nil {
			return err
		}
		if question := confirmation(path, changes); question != "" {
			yes, err := ask(ctx, question)
			if err != nil {
				return err
			}
			if !yes {
				fmt.Fprintln(stderr, "not saved; the page is still open")
				return errors.New("nothing was saved: this was not confirmed in your terminal")
			}
		}
		warned, kept, err := replaceConfig(path, made, debug, unchanged)
		if errors.As(err, new(writeError)) {
			return fmt.Errorf("%w\n\nnothing was saved; the new file is under New file, to put where this one is kept", err)
		}
		if err != nil {
			return err
		}
		reportReplaced(path, warned, kept, stdout)
		return nil
	}}
	saved, err := srv.Serve(ctx, func(url string) {
		fmt.Fprintf(stderr, "editing the settings at %s\nsave on the page to finish; close it to leave them as they are\n", url)
		if err := open(url); err != nil {
			fmt.Fprintln(stderr, "open it in your browser")
		}
	})
	if err != nil {
		fmt.Fprintln(stderr, "nothing was saved")
		return 1
	}
	if !saved {
		fmt.Fprintln(stderr, "closed; nothing was changed")
	}
	return 0
}

// openPage opens a page in the browser.
func openPage(url string) error { return browser.New("", io.Discard, io.Discard).Browse(url) }

// uiKeys are the key actions with the words of their scope.
func uiKeys() []ui.Key {
	out := make([]ui.Key, len(keymap.Defaults))
	for i, d := range keymap.Defaults {
		out[i] = ui.Key{Action: string(d.Action), Keys: d.Keys, Scope: string(d.Scope), ScopeWords: scopeWords[d.Scope]}
	}
	return out
}

// checkedFile writes src to the file tmp, or to a new temporary one, and
// checks it as gh-desk checks a config file at start, telling of it as the
// config file at path. It leaves tmp holding src. What comes back are the
// warnings, and the mistakes as an error.
func checkedFile(path string, src []byte, tmp string, debug bool) (string, error) {
	if tmp == "" {
		f, err := os.CreateTemp("", "gh-desk-config-*.toml")
		if err != nil {
			return "", err
		}
		tmp = f.Name()
		defer os.Remove(tmp)
		if err := f.Close(); err != nil {
			return "", err
		}
	}
	if err := writeSynced(tmp, src); err != nil {
		return "", fmt.Errorf("cannot write %s: %w", config.TildePath(path), err)
	}
	said := strings.NewReplacer(config.TildePath(tmp), config.TildePath(path), tmp, config.TildePath(path))
	_, _, warns, err := load(tmp, debug, new(slog.LevelVar))
	if err != nil {
		return "", errors.New(said.Replace(err.Error()))
	}
	if len(warns) > 0 {
		return said.Replace(config.Report(tmp, warns).Error()), nil
	}
	return "", nil
}

func writeSynced(path string, src []byte) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_TRUNC, 0)
	if err != nil {
		return err
	}
	_, werr := f.Write(src)
	return errors.Join(werr, f.Sync(), f.Close())
}

// applyConfig replaces the config file at path with what in holds.
func applyConfig(path string, debug bool, in io.Reader, stdout, stderr io.Writer) int {
	src, err := io.ReadAll(in)
	if err != nil {
		fmt.Fprintf(stderr, "gh-desk: %v\n", err)
		return 1
	}
	// An empty clipboard, or a tool that ended without saving: no file was
	// given, so there is nothing to replace this one with.
	if strings.TrimSpace(string(src)) == "" {
		fmt.Fprintf(stderr, "nothing to apply; %s was not changed\n", config.TildePath(path))
		return 0
	}
	warned, kept, err := replaceConfig(path, src, debug, nil)
	if err != nil {
		fmt.Fprintln(stderr, err)
		fmt.Fprintf(stderr, "\n%s was not changed\n", config.TildePath(path))
		return 1
	}
	reportReplaced(path, warned, kept, stdout)
	return 0
}

func reportReplaced(path, warned string, kept bool, stdout io.Writer) {
	if warned != "" {
		fmt.Fprintln(stdout, warned)
	}
	fmt.Fprintf(stdout, "wrote %s\n", config.TildePath(path))
	if kept {
		fmt.Fprintf(stdout, "the old one is %s\n", config.TildePath(path+".bak"))
	}
}

// writeError is a failure to write beside the config file, rather than a
// mistake in what was to be written.
type writeError struct{ error }

// replaceConfig replaces the config file at path with src. It is checked
// first, as a file of its own beside the config file, so that the mistakes
// are found as gh-desk would find them, and the config file is there, the
// old one or the new, at every moment. unchanged, if given, sees the file
// right before that and can refuse. It returns the warnings, and whether an
// old file was kept.
func replaceConfig(path string, src []byte, debug bool, unchanged func(now []byte, there bool) error) (string, bool, error) {
	cannotWrite := func(err error) error {
		return writeError{fmt.Errorf("cannot write %s: %w", config.TildePath(path), err)}
	}
	// A link stays one: what it points to is replaced.
	real := path
	if r, err := filepath.EvalSymlinks(path); err == nil {
		real = r
	}
	// A file gh-desk writes for the first time is the user's alone: it can
	// hold what an MCP server signs in with.
	mode := fs.FileMode(0o600)
	if st, err := os.Stat(real); err == nil {
		mode = st.Mode().Perm()
	}
	if err := os.MkdirAll(filepath.Dir(real), 0o755); err != nil {
		return "", false, cannotWrite(err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(real), ".config-*.toml")
	if err != nil {
		return "", false, cannotWrite(err)
	}
	defer os.Remove(tmp.Name())
	if err := errors.Join(tmp.Close(), os.Chmod(tmp.Name(), mode)); err != nil {
		return "", false, cannotWrite(err)
	}
	warned, err := checkedFile(path, src, tmp.Name(), debug)
	if err != nil {
		return "", false, err
	}
	old, err := os.ReadFile(real)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return "", false, err
	}
	exists := err == nil
	if unchanged != nil {
		if err := unchanged(old, exists); err != nil {
			return "", false, err
		}
	}
	if exists {
		bak := real + ".bak"
		if err := errors.Join(os.WriteFile(bak, old, 0o600), os.Chmod(bak, 0o600)); err != nil {
			return "", false, writeError{fmt.Errorf("cannot keep the old file: %w", err)}
		}
	}
	if err := os.Rename(tmp.Name(), real); err != nil {
		return "", false, cannotWrite(err)
	}
	return warned, exists, nil
}

// editConfig opens the config file in the editor a review is written in,
// writing it first when it is not there, and checks it when the editor is
// closed.
func editConfig(path string, debug bool, stdout, stderr io.Writer) int {
	if err := config.WriteTemplate(path); err != nil && !errors.Is(err, fs.ErrExist) {
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
