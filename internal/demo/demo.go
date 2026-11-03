// Package demo runs the TUI against a made-up GitHub and a made-up Claude,
// for the recordings in the README. It is built only into hack/demo, never
// into gh-desk.
package demo

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"github.com/babarot/gh-desk/internal/app"
	"github.com/babarot/gh-desk/internal/config"
	"github.com/babarot/gh-desk/internal/logging"
	"github.com/babarot/gh-desk/internal/runner"
	"github.com/babarot/gh-desk/internal/tui"
	"github.com/babarot/gh-desk/internal/tui/keymap"
	"github.com/babarot/gh-desk/internal/version"
)

// Run runs the TUI with the config file a first launch writes, until it
// quits. Nothing is read from or written to the user's files.
func Run(ctx context.Context, speed float64) error {
	log := logging.Discard()
	dir, err := os.MkdirTemp("", "gh-desk-demo")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, "config.toml")
	if err := config.WriteTemplate(path, app.KeysTemplate()); err != nil {
		return err
	}
	cfg, _, err := config.Load(path)
	if err != nil {
		return err
	}
	set := app.Settings(cfg)
	set.Viewer = Viewer
	set.Host = "github.com"
	set.Version = version.String()
	set.CanWait = true
	gh := NewGitHub(time.Now())
	gh.Speed = speed
	rn := runner.New(gh, log, runner.Options{Settings: set.Plan, Viewer: Viewer, CanWait: true,
		ChecksTimeout: cfg.Merge.ChecksTimeout.D, BlockedTimeout: cfg.Merge.BlockedTimeout.D})
	cl := &Claude{Dir: filepath.Join(dir, "worktrees"), Speed: speed}
	return tui.Run(ctx, gh, rn, cl, keymap.Default(), log, set)
}
