package app

import (
	"github.com/babarot/gh-desk/internal/config"
	"github.com/babarot/gh-desk/internal/tui"
	"github.com/babarot/gh-desk/internal/tui/keymap"
	"github.com/babarot/gh-desk/internal/version"
)

// Demo is what the TUI runs with when the config file is the one written
// on first run: its settings, its keys, and the file itself. Nothing is
// read from the disk, so the demo on the site, which has no disk, runs
// with what a new user sees.
func Demo() (tui.Settings, keymap.Map, config.File, error) {
	const path = "~/.config/gh-desk/config.toml"
	cfg, _, err := config.Parse(path, []byte(config.Template(KeysTemplate())))
	if err != nil {
		return tui.Settings{}, keymap.Map{}, config.File{}, err
	}
	km, ps := applyKeys(cfg.Keys)
	if len(ps) > 0 {
		return tui.Settings{}, keymap.Map{}, config.File{}, config.Report(path, ps)
	}
	set := settings(cfg)
	set.Version = version.String()
	return set, km, cfg, nil
}
