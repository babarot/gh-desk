// Writes the settings playground to the file named by its
// argument. demo/site/build.sh runs it, on the machine that builds the site
// and not in WebAssembly: the page is HTML, drawn by its own JavaScript.
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/babarot/gh-desk/internal/app"
)

func main() {
	if err := write(os.Args[1]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func write(path string) error {
	page, err := app.SettingsPlayground()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, page, 0o644)
}
