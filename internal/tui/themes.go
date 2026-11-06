package tui

import (
	"image/color"
	"maps"
	"slices"

	"charm.land/lipgloss/v2"
)

// claudeOrange is Claude's own orange, which its mascot keeps on every
// theme: the color is Claude's, not the theme's (ADR-081).
func claudeOrange() color.Color { return lipgloss.Color("#d97757") }

// themes are the color schemes that can be chosen by name.
var themes = map[string]palette{
	// default is gh-desk's own: Catppuccin Mocha made lighter, so that
	// the surfaces stand out on a terminal.
	"default": {
		dark: true,
		base: lipgloss.Color("#1e1e2e"), surface0: lipgloss.Color("#3b3b52"), surface1: lipgloss.Color("#525270"),
		overlay: lipgloss.Color("#8888a4"), subtext0: lipgloss.Color("#b0b8d1"), subtext1: lipgloss.Color("#d0d6e8"),
		text: lipgloss.Color("#e0e4f0"), dim: lipgloss.Color("#706f87"),
		blue: lipgloss.Color("#96bfff"), mauve: lipgloss.Color("#d4b0ff"), green: lipgloss.Color("#b5f0b0"),
		yellow: lipgloss.Color("#ffe5a0"), peach: lipgloss.Color("#ffc49a"), red: lipgloss.Color("#f5a3b5"), teal: lipgloss.Color("#94e2d5"),
		pick: lipgloss.Color("#3a3150"), pickSel: lipgloss.Color("#4d4068"),
		claude: lipgloss.Color("#22393d"), claudeSel: lipgloss.Color("#2f4f54"), fresh: lipgloss.Color("#26324a"),
	},
	"latte": {
		base: lipgloss.Color("#eff1f5"), surface0: lipgloss.Color("#ccd0da"), surface1: lipgloss.Color("#bcc0cc"),
		overlay: lipgloss.Color("#7c7f93"), subtext0: lipgloss.Color("#6c6f85"), subtext1: lipgloss.Color("#5c5f77"),
		text: lipgloss.Color("#4c4f69"), dim: lipgloss.Color("#9ca0b0"),
		blue: lipgloss.Color("#1e66f5"), mauve: lipgloss.Color("#8839ef"), green: lipgloss.Color("#40a02b"),
		yellow: lipgloss.Color("#df8e1d"), peach: lipgloss.Color("#fe640b"), red: lipgloss.Color("#d20f39"), teal: lipgloss.Color("#179299"),
		pick: lipgloss.Color("#e6dcf7"), pickSel: lipgloss.Color("#d6c6f2"),
		claude: lipgloss.Color("#d4eeee"), claudeSel: lipgloss.Color("#bde3e4"), fresh: lipgloss.Color("#dce6fb"),
	},
	// github-dark and github-light are GitHub's own, from Primer.
	"github-dark": {
		dark: true,
		base: lipgloss.Color("#0d1117"), surface0: lipgloss.Color("#262c36"), surface1: lipgloss.Color("#3d444d"),
		overlay: lipgloss.Color("#8b949e"), subtext0: lipgloss.Color("#9198a1"), subtext1: lipgloss.Color("#c9d1d9"),
		text: lipgloss.Color("#e6edf3"), dim: lipgloss.Color("#6e7681"),
		blue: lipgloss.Color("#58a6ff"), mauve: lipgloss.Color("#bc8cff"), green: lipgloss.Color("#3fb950"),
		yellow: lipgloss.Color("#d29922"), peach: lipgloss.Color("#f0883e"), red: lipgloss.Color("#f85149"), teal: lipgloss.Color("#39c5cf"),
	},
	"github-light": {
		base: lipgloss.Color("#ffffff"), surface0: lipgloss.Color("#e6eaef"), surface1: lipgloss.Color("#d0d7de"),
		overlay: lipgloss.Color("#6e7781"), subtext0: lipgloss.Color("#57606a"), subtext1: lipgloss.Color("#424a53"),
		text: lipgloss.Color("#1f2328"), dim: lipgloss.Color("#8c959f"),
		blue: lipgloss.Color("#0969da"), mauve: lipgloss.Color("#8250df"), green: lipgloss.Color("#1a7f37"),
		yellow: lipgloss.Color("#9a6700"), peach: lipgloss.Color("#bc4c00"), red: lipgloss.Color("#cf222e"), teal: lipgloss.Color("#1b7c83"),
	},
	// tokyonight is Tokyo Night's night, and tokyonight-day its day.
	"tokyonight": {
		dark: true,
		base: lipgloss.Color("#1a1b26"), surface0: lipgloss.Color("#292e42"), surface1: lipgloss.Color("#3b4261"),
		overlay: lipgloss.Color("#737aa2"), subtext0: lipgloss.Color("#9aa5ce"), subtext1: lipgloss.Color("#a9b1d6"),
		text: lipgloss.Color("#c0caf5"), dim: lipgloss.Color("#565f89"),
		blue: lipgloss.Color("#7aa2f7"), mauve: lipgloss.Color("#bb9af7"), green: lipgloss.Color("#9ece6a"),
		yellow: lipgloss.Color("#e0af68"), peach: lipgloss.Color("#ff9e64"), red: lipgloss.Color("#f7768e"), teal: lipgloss.Color("#73daca"),
	},
	"tokyonight-day": {
		base: lipgloss.Color("#e1e2e7"), surface0: lipgloss.Color("#c4c8da"), surface1: lipgloss.Color("#a8aecb"),
		overlay: lipgloss.Color("#68709a"), subtext0: lipgloss.Color("#6172b0"), subtext1: lipgloss.Color("#4c5a96"),
		text: lipgloss.Color("#3760bf"), dim: lipgloss.Color("#848cb5"),
		blue: lipgloss.Color("#2e7de9"), mauve: lipgloss.Color("#9854f1"), green: lipgloss.Color("#587539"),
		yellow: lipgloss.Color("#8c6c3e"), peach: lipgloss.Color("#b15c00"), red: lipgloss.Color("#f52a65"), teal: lipgloss.Color("#118c74"),
	},
	// gruvbox-dark and gruvbox-light are Gruvbox's medium contrast.
	"gruvbox-dark": {
		dark: true,
		base: lipgloss.Color("#282828"), surface0: lipgloss.Color("#3c3836"), surface1: lipgloss.Color("#504945"),
		overlay: lipgloss.Color("#928374"), subtext0: lipgloss.Color("#a89984"), subtext1: lipgloss.Color("#d5c4a1"),
		text: lipgloss.Color("#ebdbb2"), dim: lipgloss.Color("#7c6f64"),
		blue: lipgloss.Color("#83a598"), mauve: lipgloss.Color("#d3869b"), green: lipgloss.Color("#b8bb26"),
		yellow: lipgloss.Color("#fabd2f"), peach: lipgloss.Color("#fe8019"), red: lipgloss.Color("#fb4934"), teal: lipgloss.Color("#8ec07c"),
	},
	"gruvbox-light": {
		base: lipgloss.Color("#fbf1c7"), surface0: lipgloss.Color("#ebdbb2"), surface1: lipgloss.Color("#d5c4a1"),
		overlay: lipgloss.Color("#7c6f64"), subtext0: lipgloss.Color("#665c54"), subtext1: lipgloss.Color("#504945"),
		text: lipgloss.Color("#3c3836"), dim: lipgloss.Color("#a89984"),
		blue: lipgloss.Color("#076678"), mauve: lipgloss.Color("#8f3f71"), green: lipgloss.Color("#79740e"),
		yellow: lipgloss.Color("#b57614"), peach: lipgloss.Color("#af3a03"), red: lipgloss.Color("#9d0006"), teal: lipgloss.Color("#427b58"),
	},
	// nord is Nord, which has no light one. Its frost is blue and teal;
	// blue is nord9, so that rows read again and rows Claude selected
	// differ.
	"nord": {
		dark: true,
		base: lipgloss.Color("#2e3440"), surface0: lipgloss.Color("#3b4252"), surface1: lipgloss.Color("#4c566a"),
		overlay: lipgloss.Color("#7b88a1"), subtext0: lipgloss.Color("#d8dee9"), subtext1: lipgloss.Color("#e5e9f0"),
		text: lipgloss.Color("#eceff4"), dim: lipgloss.Color("#616e88"),
		blue: lipgloss.Color("#81a1c1"), mauve: lipgloss.Color("#b48ead"), green: lipgloss.Color("#a3be8c"),
		yellow: lipgloss.Color("#ebcb8b"), peach: lipgloss.Color("#d08770"), red: lipgloss.Color("#bf616a"), teal: lipgloss.Color("#8fbcbb"),
	},
}

// ThemeNames are the names of the themes, sorted.
func ThemeNames() []string { return slices.Sorted(maps.Keys(themes)) }

// theme is the palette named name, or default if there is none by that
// name; the config file's check keeps that from happening.
func theme(name string) palette {
	if p, ok := themes[name]; ok {
		return p
	}
	return themes["default"]
}
