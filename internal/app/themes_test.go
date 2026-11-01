package app

import (
	"slices"
	"testing"

	"github.com/babarot/gh-triage/internal/config"
	"github.com/babarot/gh-triage/internal/tui"
)

// TestThemesInStep keeps the themes the config file takes the same as
// those the TUI draws.
func TestThemesInStep(t *testing.T) {
	if got, want := slices.Sorted(slices.Values(config.Themes)), tui.ThemeNames(); !slices.Equal(got, want) {
		t.Errorf("config.Themes = %q, the TUI draws %q", got, want)
	}
}
