package tui

import (
	"image/color"
	"testing"

	"charm.land/lipgloss/v2"
)

func TestMix(t *testing.T) {
	for _, c := range []struct {
		a, b string
		t    float64
		want color.RGBA
	}{
		{"#96bfff", "#1e1e2e", 0.35, color.RGBA{0x48, 0x56, 0x77, 0xff}},
		{"#1e66f5", "#eff1f5", 0.25, color.RGBA{0xbb, 0xce, 0xf5, 0xff}},
		{"#96bfff", "#1e1e2e", 0, color.RGBA{0x1e, 0x1e, 0x2e, 0xff}},
		{"#96bfff", "#1e1e2e", 1, color.RGBA{0x96, 0xbf, 0xff, 0xff}},
	} {
		if got := mix(lipgloss.Color(c.a), lipgloss.Color(c.b), c.t); got != c.want {
			t.Errorf("mix(%s, %s, %v) = %v, want %v", c.a, c.b, c.t, got, c.want)
		}
	}
}
