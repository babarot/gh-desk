// demo runs gh-desk against a made-up GitHub and a made-up Claude, for the
// recordings in the README: mise run demo records them.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/babarot/gh-desk/internal/demo"
)

func main() {
	speed := flag.Float64("speed", 1, "scale every wait of the made-up GitHub and Claude; 0 answers at once")
	flag.Parse()
	if err := demo.Run(context.Background(), *speed); err != nil {
		fmt.Fprintln(os.Stderr, "demo:", err)
		os.Exit(1)
	}
}
