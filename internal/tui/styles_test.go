package tui

import (
	"image/color"
	"reflect"
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

// TestThemesFilled checks that every color of every theme is there once
// the backgrounds are worked out, so that a color added to palette and
// left out of a theme, or a theme that leaves out a color, fails. It goes
// through palette's fields, not a list of them, so a new one is checked
// without a change here.
func TestThemesFilled(t *testing.T) {
	missing := func(p palette) []string {
		var out []string
		v := reflect.ValueOf(p)
		for i := range v.NumField() {
			if f := v.Type().Field(i); f.Type == reflect.TypeFor[color.Color]() && v.Field(i).IsNil() {
				out = append(out, f.Name)
			}
		}
		return out
	}
	for _, name := range ThemeNames() {
		p := themes[name]
		// A color the backgrounds are worked out from makes it panic.
		q, ok := func() (q palette, ok bool) {
			defer func() { ok = recover() == nil }()
			_, q = p.backgrounds()
			return q, true
		}()
		if !ok {
			t.Errorf("theme %s: the backgrounds cannot be worked out; it has no %q", name, missing(p))
			continue
		}
		if m := missing(q); len(m) > 0 {
			t.Errorf("theme %s has no %q", name, m)
		}
	}
	if _, q := themes["default"].backgrounds(); q.pick != themes["default"].pick {
		t.Error("default's own pick is replaced")
	}
}
