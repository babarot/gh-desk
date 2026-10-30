// gh-triage is a gh extension for reviewing and merging the pull requests
// that reach you, from the terminal.
package main

import (
	"os"

	"github.com/babarot/gh-triage/internal/app"
)

func main() {
	os.Exit(app.Run(os.Args[1:], os.Stdout, os.Stderr))
}
