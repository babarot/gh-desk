package tui

import (
	"image/color"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/babarot/gh-desk/internal/tui/keymap"
)

// scene is a screen the golden views draw again on a light background and
// on every theme: the keys pressed from the start, then text pasted.
type scene struct {
	name  string
	keys  []string
	paste string
}

// scenes are one of each kind of thing the screen shows: the list, the
// preview of a diff, a popup, a text box, the Claude pane and the inbox.
var scenes = []scene{
	{name: "review"},
	{name: "preview-diff", keys: []string{"v"}},
	{name: "action-menu", keys: []string{"enter"}},
	{name: "compose", keys: []string{"R"}, paste: "Does this retry on a 429?"},
	{name: "claude", keys: []string{"i", "tab", "enter"}},
	{name: "inbox", keys: []string{"4"}},
}

var (
	white = color.RGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}
	black = color.RGBA{A: 0xff}
)

// startOn starts the TUI as start does, with the themes dark and light, and
// with the terminal telling bg as its background before anything is read,
// as a terminal answers at the start.
func startOn(t *testing.T, dark, light string, bg color.Color) *Model {
	t.Helper()
	s := testSettings()
	s.ThemeDark, s.ThemeLight = dark, light
	m := New(fixture(), &fakeRunner{}, &fakeClaude{}, keymap.Default(), nil, s)
	m.Update(tea.WindowSizeMsg{Width: 130, Height: 34})
	m.Update(tea.BackgroundColorMsg{Color: bg})
	run(m, m.Init())
	return m
}

func (sc scene) play(m *Model) {
	press(m, sc.keys...)
	if sc.paste != "" {
		run(m, m.paste(tea.PasteMsg{Content: sc.paste}))
	}
}

// TestLightBackground draws the scenes on a light terminal, which takes the
// light theme in place of the dark one.
func TestLightBackground(t *testing.T) {
	for _, sc := range scenes {
		t.Run(sc.name, func(t *testing.T) {
			m := startOn(t, "default", "latte", white)
			sc.play(m)
			golden(t, "light-"+sc.name, m)
		})
	}
}

// TestThemes draws the Claude pane, with the list and its rows beside it, on
// every theme, each on the background it is for, so that a color of one
// theme taken for another fails.
func TestThemes(t *testing.T) {
	sc := scenes[4]
	for _, name := range ThemeNames() {
		t.Run(name, func(t *testing.T) {
			bg := color.Color(black)
			if !theme(name).dark {
				bg = white
			}
			m := startOn(t, name, name, bg)
			sc.play(m)
			golden(t, "theme-"+name, m)
		})
	}
}
