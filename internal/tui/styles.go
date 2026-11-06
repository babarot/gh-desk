package tui

import (
	"image/color"
	"math"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// palette is a color scheme: the colors a theme gives, each named by the
// role it has in Catppuccin, whose names they were first. The
// backgrounds of rows and of the tab shown are worked out from these (see
// backgrounds), unless a palette gives its own.
type palette struct {
	// dark says the palette is for a dark terminal.
	dark bool
	// base is the background; surface0 that of the header, the cursor's
	// row and a choice shown, and surface1 the borders. overlay, dim and
	// subtext0 and subtext1 are text fainter than text, dim the faintest.
	base, surface0, surface1, overlay, subtext0, subtext1, text, dim color.Color
	blue, mauve, green, yellow, peach, red, teal                     color.Color
	// pick and pickSel are the backgrounds of a selected row, and of a
	// selected row under the cursor; claude and claudeSel those of a row
	// Claude selected; fresh is that of a row a reading again added or
	// changed. Left nil, they are worked out.
	pick, pickSel, claude, claudeSel, fresh color.Color
}

// shares are how much of a color each background has, laid over base,
// from 0 to 1: as if the color were drawn at that opacity, which terminals
// cannot do. A light background needs less of a color to show it.
type shares struct{ tab, pick, pickSel, claude, claudeSel, fresh float64 }

var (
	darkShares  = shares{tab: 0.35, pick: 0.16, pickSel: 0.26, claude: 0.12, claudeSel: 0.25, fresh: 0.12}
	lightShares = shares{tab: 0.25, pick: 0.11, pickSel: 0.24, claude: 0.10, claudeSel: 0.22, fresh: 0.08}
)

// backgrounds are the backgrounds p does not give, worked out from its
// colors: blue for the tab shown, mauve for a selected row, teal for one
// Claude selected, and blue for one read again.
func (p palette) backgrounds() (tab color.Color, q palette) {
	sh := lightShares
	if p.dark {
		sh = darkShares
	}
	or := func(c, from color.Color, t float64) color.Color {
		if c != nil {
			return c
		}
		return mix(from, p.base, t)
	}
	p.pick, p.pickSel = or(p.pick, p.mauve, sh.pick), or(p.pickSel, p.mauve, sh.pickSel)
	p.claude, p.claudeSel = or(p.claude, p.teal, sh.claude), or(p.claudeSel, p.teal, sh.claudeSel)
	p.fresh = or(p.fresh, p.blue, sh.fresh)
	return mix(p.blue, p.base, sh.tab), p
}

// styles are the styles drawn with one palette.
type styles struct {
	p                                       palette
	header, app, colHeader                  lipgloss.Style
	sel, dim, muted, sub0, sub1, text, bold lipgloss.Style
	blue, mauve, green, yellow, peach, red  lipgloss.Style
	teal, key, border, title, group, add    lipgloss.Style
	// focus is the frame of what takes the keys while it is shown (a
	// popup, the log viewer, the claude pane); border is that of what takes
	// none (ADR-079).
	focus                                lipgloss.Style
	del, hunk, file, me                  lipgloss.Style
	picked, pickedSel, claude, claudeSel lipgloss.Style
	fresh                                lipgloss.Style
	// tabOn and tabOff are a tab that switches the screen or the pane
	// (the header, the pane's overview and diff), shown or
	// not; subOn and subOff a choice within one (a file of the diff, the
	// kind of a review). Both are drawn as pills, the second on a fainter
	// background, so that one reads as under the other.
	tabOn, tabOff, subOn, subOff lipgloss.Style
	// tabKeyOn is the key of the header's tab shown, inside its pill but
	// fainter than its name.
	tabKeyOn lipgloss.Style
	// code is inline code in a description; block a code block, and
	// blockLang its language.
	code, block, blockLang lipgloss.Style
	// band is the background of a question in the chat.
	band lipgloss.Style
	// mascot is Claude's mascot, in Claude's color on every theme.
	mascot lipgloss.Style
}

func newStyles(p palette) styles {
	fg := func(c color.Color) lipgloss.Style { return lipgloss.NewStyle().Foreground(c) }
	tab, p := p.backgrounds()
	return styles{
		p:         p,
		header:    lipgloss.NewStyle().Background(p.surface0).Foreground(p.subtext0),
		app:       fg(p.blue).Bold(true),
		tabOn:     lipgloss.NewStyle().Background(tab).Foreground(p.text).Bold(true),
		tabOff:    fg(p.subtext0),
		tabKeyOn:  lipgloss.NewStyle().Background(tab).Foreground(p.subtext0),
		subOn:     lipgloss.NewStyle().Background(p.surface0).Foreground(p.blue).Bold(true),
		subOff:    fg(p.overlay),
		colHeader: fg(p.subtext1).Bold(true),
		sel:       lipgloss.NewStyle().Background(p.surface0),
		dim:       fg(p.dim),
		muted:     fg(p.overlay),
		sub0:      fg(p.subtext0),
		sub1:      fg(p.subtext1),
		text:      fg(p.text),
		bold:      fg(p.text).Bold(true),
		blue:      fg(p.blue),
		mauve:     fg(p.mauve),
		green:     fg(p.green),
		yellow:    fg(p.yellow),
		peach:     fg(p.peach),
		red:       fg(p.red),
		teal:      fg(p.teal),
		key:       fg(p.blue).Bold(true),
		border:    fg(p.surface1),
		focus:     fg(p.blue),
		title:     fg(p.subtext1).Bold(true),
		group:     fg(p.subtext0).Bold(true),
		add:       fg(p.green),
		del:       fg(p.red),
		hunk:      fg(p.teal),
		file:      fg(p.mauve).Bold(true),
		me:        fg(p.peach).Bold(true),
		picked:    lipgloss.NewStyle().Background(p.pick),
		pickedSel: lipgloss.NewStyle().Background(p.pickSel),
		claude:    lipgloss.NewStyle().Background(p.claude),
		claudeSel: lipgloss.NewStyle().Background(p.claudeSel),
		fresh:     lipgloss.NewStyle().Background(p.fresh),
		code:      lipgloss.NewStyle().Background(p.surface0).Foreground(p.peach),
		block:     lipgloss.NewStyle().Background(p.surface0).Foreground(p.subtext1),
		blockLang: lipgloss.NewStyle().Background(p.surface0).Foreground(p.overlay),
		band:      lipgloss.NewStyle().Background(p.surface0),
		mascot:    fg(claudeOrange()),
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

// mix is a laid over b with the share t of a, from 0 to 1.
func mix(a, b color.Color, t float64) color.Color {
	ar, ag, ab, _ := a.RGBA()
	br, bg, bb, _ := b.RGBA()
	ch := func(x, y uint32) uint8 {
		return uint8(math.Round((float64(x)*t + float64(y)*(1-t)) / 0x101))
	}
	return color.RGBA{R: ch(ar, br), G: ch(ag, bg), B: ch(ab, bb), A: 0xff}
}
