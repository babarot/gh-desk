package app

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/babarot/gh-desk/internal/config"
	"github.com/babarot/gh-desk/internal/logging"
	"github.com/babarot/gh-desk/internal/version"
)

// options are the flags of gh desk that its commands read.
type options struct {
	config  string
	debug   bool
	noCache bool
}

// exitCode is what a command returns once it has said why it failed, so
// that Run exits with the code and prints nothing more. Any other error
// comes from a wrong call.
type exitCode int

func (c exitCode) Error() string { return fmt.Sprintf("exit status %d", int(c)) }

// exited is code as what a command returns.
func exited(code int) error {
	if code == 0 {
		return nil
	}
	return exitCode(code)
}

// newRootCommand is gh desk with its commands, writing to stdout and
// stderr. Installed as a gh extension it is called gh desk, and the help
// says so; its name stays that of the binary, which shell completion runs.
func newRootCommand(stdout, stderr io.Writer) *cobra.Command {
	o := &options{}
	root := &cobra.Command{
		Use:           "gh-desk",
		Short:         "review and merge the pull requests that reach you, from the terminal",
		Annotations:   map[string]string{cobra.CommandDisplayNameAnnotation: "gh desk"},
		Version:       version.String(),
		SilenceErrors: true,
		SilenceUsage:  true,
		Args: func(_ *cobra.Command, args []string) error {
			if len(args) > 0 {
				return fmt.Errorf("unknown command %q", args[0])
			}
			return nil
		},
		RunE: func(*cobra.Command, []string) error {
			return exited(start(o, false, stdout, stderr))
		},
	}
	root.SetOut(stdout)
	root.SetErr(stderr)
	root.SetVersionTemplate("{{.Version}}\n")

	root.PersistentFlags().StringVar(&o.config, "config", config.FilePath(), "the config file")
	root.PersistentFlags().BoolVar(&o.debug, "debug", false, "log at debug level")
	root.Flags().BoolVar(&o.noCache, "no-cache", false, "neither show nor keep the lists last read")
	root.Flags().Bool("version", false, "print the version")

	root.AddCommand(
		newConfigCommand(o, stdout, stderr),
		&cobra.Command{
			Use:   "gc",
			Short: "remove the worktrees of pull requests no longer open",
			Args:  cobra.NoArgs,
			RunE: func(*cobra.Command, []string) error {
				return exited(start(o, true, stdout, stderr))
			},
		},
		&cobra.Command{
			Use:    "mcp",
			Short:  "serve gh-desk's own MCP server to Claude Code, which starts it",
			Hidden: true,
			Args:   cobra.NoArgs,
			RunE: func(*cobra.Command, []string) error {
				return exited(serveMCP(o, stderr))
			},
		},
		&cobra.Command{
			Use:   "log",
			Short: "print where the log file is",
			Args:  cobra.NoArgs,
			RunE: func(*cobra.Command, []string) error {
				fmt.Fprintln(stdout, logging.DefaultPath())
				return nil
			},
		},
	)
	return root
}
