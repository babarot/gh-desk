package app

import (
	"bytes"
	"flag"
	"os"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "rewrite docs/reference/commands.md")

// The commands page of the documentation is the help gh-desk prints, so
// it says what -h and gh desk config help say.

const commandsReferencePath = "../../docs/reference/commands.md"

func commandsReference(t *testing.T) string {
	// The default of --config is written as a reader's own path would
	// start, not as the path of whoever runs the test.
	t.Setenv("XDG_CONFIG_HOME", "~/.config")
	var help, configHelp bytes.Buffer
	Run([]string{"-h"}, &bytes.Buffer{}, &help)
	configCommand([]string{"help"}, "", false, &configHelp, &bytes.Buffer{})

	var b strings.Builder
	b.WriteString("# Commands\n\n")
	b.WriteString("<!-- Generated from internal/app by TestCommandsReferenceIsUpToDate. Do not edit; run go test ./internal/app -run TestCommandsReferenceIsUpToDate -update. -->\n\n")
	b.WriteString("Installed as a gh extension, gh-desk runs as `gh desk`; installed with Go or Nix, as `gh-desk`. With no command, it opens the pull requests waiting on you.\n\n")
	b.WriteString("## gh desk\n\n```\n" + help.String() + "```\n\n")
	b.WriteString("## gh desk config\n\n```\n" + configHelp.String() + "```\n")
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
