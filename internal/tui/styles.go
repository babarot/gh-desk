package tui

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// palette is a color scheme: Catppuccin Mocha on a dark terminal, Latte on
// a light one.
type palette struct {
	base, surface0, surface1, overlay, subtext0, subtext1, text, dim color.Color
	blue, mauve, green, yellow, peach, red, teal                     color.Color
	// pick and pickSel are the backgrounds of a selected row, and of a
	// selected row under the cursor; claude and claudeSel those of a row
	// Claude selected; fresh is that of a row a reading again added or
	// changed.
	pick, pickSel, claude, claudeSel, fresh color.Color
}

var (
	mocha = palette{
		base: lipgloss.Color("#1e1e2e"), surface0: lipgloss.Color("#3b3b52"), surface1: lipgloss.Color("#525270"),
		overlay: lipgloss.Color("#8888a4"), subtext0: lipgloss.Color("#b0b8d1"), subtext1: lipgloss.Color("#d0d6e8"),
		text: lipgloss.Color("#e0e4f0"), dim: lipgloss.Color("#706f87"),
		blue: lipgloss.Color("#96bfff"), mauve: lipgloss.Color("#d4b0ff"), green: lipgloss.Color("#b5f0b0"),
		yellow: lipgloss.Color("#ffe5a0"), peach: lipgloss.Color("#ffc49a"), red: lipgloss.Color("#f5a3b5"), teal: lipgloss.Color("#94e2d5"),
		pick: lipgloss.Color("#3a3150"), pickSel: lipgloss.Color("#4d4068"),
		claude: lipgloss.Color("#22393d"), claudeSel: lipgloss.Color("#2f4f54"), fresh: lipgloss.Color("#26324a"),
	}
	latte = palette{
		base: lipgloss.Color("#eff1f5"), surface0: lipgloss.Color("#ccd0da"), surface1: lipgloss.Color("#bcc0cc"),
		overlay: lipgloss.Color("#7c7f93"), subtext0: lipgloss.Color("#6c6f85"), subtext1: lipgloss.Color("#5c5f77"),
		text: lipgloss.Color("#4c4f69"), dim: lipgloss.Color("#9ca0b0"),
		blue: lipgloss.Color("#1e66f5"), mauve: lipgloss.Color("#8839ef"), green: lipgloss.Color("#40a02b"),
		yellow: lipgloss.Color("#df8e1d"), peach: lipgloss.Color("#fe640b"), red: lipgloss.Color("#d20f39"), teal: lipgloss.Color("#179299"),
		pick: lipgloss.Color("#e6dcf7"), pickSel: lipgloss.Color("#d6c6f2"),
		claude: lipgloss.Color("#d4eeee"), claudeSel: lipgloss.Color("#bde3e4"), fresh: lipgloss.Color("#dce6fb"),
	}
)

// styles are the styles drawn with one palette.
type styles struct {
	p                                        palette
	header, app, tabOn, tabOff, colHeader    lipgloss.Style
	sel, dim, muted, sub0, sub1, text, bold  lipgloss.Style
	blue, mauve, green, yellow, peach, red   lipgloss.Style
	teal, key, border, title, group, add     lipgloss.Style
	del, hunk, file, tabPill, tabPillOff, me lipgloss.Style
	picked, pickedSel, claude, claudeSel     lipgloss.Style
	fresh                                    lipgloss.Style
	// fileTab is the file tab of the diff shown.
	fileTab lipgloss.Style
	// code is inline code in a description; block a code block, and
	// blockLang its language.
	code, block, blockLang lipgloss.Style
}

func newStyles(p palette) styles {
	fg := func(c color.Color) lipgloss.Style { return lipgloss.NewStyle().Foreground(c) }
	return styles{
		p:          p,
		header:     lipgloss.NewStyle().Background(p.surface0).Foreground(p.subtext0),
		app:        fg(p.blue).Bold(true),
		tabOn:      fg(p.blue).Bold(true).Underline(true),
		tabOff:     fg(p.subtext0),
		colHeader:  fg(p.subtext1).Bold(true),
		sel:        lipgloss.NewStyle().Background(p.surface0),
		dim:        fg(p.dim),
		muted:      fg(p.overlay),
		sub0:       fg(p.subtext0),
		sub1:       fg(p.subtext1),
		text:       fg(p.text),
		bold:       fg(p.text).Bold(true),
		blue:       fg(p.blue),
		mauve:      fg(p.mauve),
		green:      fg(p.green),
		yellow:     fg(p.yellow),
		peach:      fg(p.peach),
		red:        fg(p.red),
		teal:       fg(p.teal),
		key:        fg(p.blue).Bold(true),
		border:     fg(p.surface1),
		title:      fg(p.subtext1).Bold(true),
		group:      fg(p.subtext0).Bold(true),
		add:        fg(p.green),
		del:        fg(p.red),
		hunk:       fg(p.teal),
		file:       fg(p.mauve).Bold(true),
		tabPill:    lipgloss.NewStyle().Background(p.blue).Foreground(p.base).Bold(true),
		tabPillOff: fg(p.overlay),
		me:         fg(p.peach).Bold(true),
		picked:     lipgloss.NewStyle().Background(p.pick),
		pickedSel:  lipgloss.NewStyle().Background(p.pickSel),
		claude:     lipgloss.NewStyle().Background(p.claude),
		claudeSel:  lipgloss.NewStyle().Background(p.claudeSel),
		fresh:      lipgloss.NewStyle().Background(p.fresh),
		fileTab:    lipgloss.NewStyle().Background(p.surface1).Foreground(p.mauve).Bold(true),
		code:       lipgloss.NewStyle().Background(p.surface0).Foreground(p.peach),
		block:      lipgloss.NewStyle().Background(p.surface0).Foreground(p.subtext1),
		blockLang:  lipgloss.NewStyle().Background(p.surface0).Foreground(p.overlay),
	}
}

// onBackground draws a line whose parts are already colored on the
// background of bg, w cells wide. Each part ends with a reset, which would
// also end a background put around the whole line, so the background is
// set again after every reset.
func onBackground(line string, bg lipgloss.Style, w int) string {
	probe := bg.Render("\x00")
	set := probe[:strings.IndexByte(probe, 0)]
	line = fit(line, w)
	for _, reset := range []string{"\x1b[0m", "\x1b[m"} {
		line = strings.ReplaceAll(line, reset, reset+set)
	}
	return set + line + "\x1b[m"
}

// fit cuts s to w cells, with … when it is cut, and pads it to w.
func fit(s string, w int) string {
	if w <= 0 {
		return ""
	}
	s = ansi.Truncate(s, w, "…")
	if n := lipgloss.Width(s); n < w {
		s += spaces(w - n)
	}
	return s
}

func spaces(n int) string {
	if n <= 0 {
		return ""
	}
	b := make([]byte, n)
	for i := range b {
		b[i] = ' '
	}
	return string(b)
}
