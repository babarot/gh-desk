package tui

import (
	"fmt"
	"image/color"
	"slices"
)

// SpinnerFrames is a spinner as the settings page draws it.
type SpinnerFrames struct {
	Frames []string `json:"f"`
	Every  int      `json:"t"`
}

// Spinners are the spinners by name, frames and all.
func Spinners() map[string]SpinnerFrames {
	out := map[string]SpinnerFrames{}
	for n, s := range spinners {
		out[n] = SpinnerFrames{slices.Clone(s.frames), int(s.every.Milliseconds())}
	}
	return out
}

func hex(c color.Color) string {
	r, g, b, _ := c.RGBA()
	return fmt.Sprintf("#%02x%02x%02x", r>>8, g>>8, b>>8)
}

// Palettes are the themes by name, each color by its role, so that the
// page of config ui draws the list in them.
func Palettes() map[string]map[string]any {
	out := map[string]map[string]any{}
	for n, p := range themes {
		out[n] = map[string]any{"dark": p.dark, "base": hex(p.base), "surface0": hex(p.surface0), "surface1": hex(p.surface1),
			"overlay": hex(p.overlay), "subtext0": hex(p.subtext0), "subtext1": hex(p.subtext1), "text": hex(p.text), "dim": hex(p.dim),
			"blue": hex(p.blue), "mauve": hex(p.mauve), "green": hex(p.green), "yellow": hex(p.yellow), "peach": hex(p.peach), "red": hex(p.red), "teal": hex(p.teal)}
	}
	return out
}
