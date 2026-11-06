package app

import (
	"bytes"
	"flag"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

var update = flag.Bool("update", false, "rewrite docs/reference/commands.md")

// The commands page of the documentation is the help gh-desk prints, so
// it says what -h and gh desk config help say.

const commandsReferencePath = "../../docs/reference/commands.md"

func commandsReference(t *testing.T) string {
	// The default of --config is written as a reader's own path would
	// start, not as the path of whoever runs the test.
	t.Setenv("XDG_CONFIG_HOME", "~/.config")
	var b strings.Builder
	b.WriteString("# Commands\n\n")
	b.WriteString("<!-- Generated from internal/app by TestCommandsReferenceIsUpToDate. Do not edit; run go test ./internal/app -run TestCommandsReferenceIsUpToDate -update. -->\n\n")
	b.WriteString("Installed as a gh extension, gh-desk runs as `gh desk`; installed with Go or Nix, as `gh-desk`. With no command, it opens the pull requests waiting on you.\n")
	var section func(*cobra.Command)
	section = func(cmd *cobra.Command) {
		var help bytes.Buffer
		args := append(strings.Fields(cmd.CommandPath())[2:], "--help")
		Run(args, &help, &bytes.Buffer{})
		b.WriteString("\n## " + cmd.CommandPath() + "\n\n```\n" + help.String() + "```\n")
		for _, sub := range cmd.Commands() {
			// A hidden command (mcp) is run by Claude Code, not by people.
			if !sub.Hidden {
				section(sub)
			}
		}
	}
	section(newRootCommand(io.Discard, io.Discard))
	return b.String()
}

func TestCommandsReferenceIsUpToDate(t *testing.T) {
	want := commandsReference(t)
	got, err := os.ReadFile(commandsReferencePath)
	if *update {
		if err := os.WriteFile(commandsReferencePath, []byte(want), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	if err != nil || string(got) != want {
		t.Errorf("%s is not the help of the commands; run go test ./internal/app -run TestCommandsReferenceIsUpToDate -update", commandsReferencePath)
	}
}
