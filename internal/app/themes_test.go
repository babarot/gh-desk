package app

import (
	"slices"
	"testing"

	"github.com/babarot/gh-desk/internal/config"
	"github.com/babarot/gh-desk/internal/tui"
)

// TestThemesInStep keeps the themes the config file takes the same as
// those the TUI draws.
func TestThemesInStep(t *testing.T) {
	if got, want := slices.Sorted(slices.Values(config.Themes)), tui.ThemeNames(); !slices.Equal(got, want) {
		t.Errorf("config.Themes = %q, the TUI draws %q", got, want)
	}
}

// TestSpinnersInStep keeps the spinners the config file takes the same as
// those the TUI draws.
func TestSpinnersInStep(t *testing.T) {
	if got, want := slices.Sorted(slices.Values(config.Spinners)), tui.SpinnerNames(); !slices.Equal(got, want) {
		t.Errorf("config.Spinners = %q, the TUI draws %q", got, want)
	}
}

// TestMenuActionsInStep keeps the actions the config file takes for the
// menu, and their default order, the same as those the TUI offers.
func TestMenuActionsInStep(t *testing.T) {
	if got, want := config.MenuActions, tui.MenuActions(); !slices.Equal(got, want) {
		t.Errorf("config.MenuActions = %q, the TUI offers %q", got, want)
	}
}
