# ADR-019: Themes are built-in palettes

## Status

Accepted

## Context

gh-desk had two color schemes, `mocha` and `latte`, chosen by `theme.name`, whose `auto` picked one of the two by the terminal's background. Each was a palette of colors written in the code, among them backgrounds made by laying a color faintly over the base, worked out by hand. A third scheme would have had to work those out again, and `auto` could not pick any other pair. `mocha` was not Catppuccin Mocha as published: its surfaces were made lighter so that they show on a terminal, so its name promised what it was not.

## Decision

- A theme is a palette built into gh-desk, chosen by name. A user does not define colors in the config file.
- A palette gives its base colors and whether it is a dark one. The backgrounds laid faintly over the base (the tab shown, selected rows, rows Claude selected, rows read again) are worked out from those colors by a share that depends only on whether the palette is dark. A palette may give its own backgrounds of rows instead, as `default` and `latte` do to keep the colors they had; the tab shown is always worked out, which gave the colors they had.
- The theme is chosen by two settings, `theme.dark` and `theme.light`, the themes used on a dark terminal and on a light one; a terminal that does not tell its background gets `theme.dark`. Either takes any theme, and the same theme in both is used on any terminal. A setting that names one theme for every terminal beside the two was rejected: while it is set, the other two are not read, and the file does not show which is in effect.
- gh-desk's own scheme is named `default`, not `mocha`. A theme named after a published scheme keeps that scheme's colors.
- The names of the themes live in `internal/config`, which checks the file, and the palettes in `internal/tui`, which draws them; `config` cannot import `tui` (ADR-002), so a test in `internal/app` keeps the names the same in both.

## Consequences

- Adding a theme is a palette in `internal/tui` and its name in `internal/config`; the backgrounds come with it.
- A color is written only in the themes. `colors_test.go` fails on a color written anywhere else in the code, and a test in `internal/tui` on a theme without a color that palette has, so a color added to the screen is added to every theme.
- `theme.name` is an old name (ADR-016): a file that has it is told at that line to use `theme.dark` and `theme.light`.
- A theme that follows the terminal's own sixteen colors is not possible this way: their values are not known, so nothing can be laid over them. It would need a decision of its own.
- A file that names the theme `mocha` is a mistake, reported at that line. gh-desk had no release when it was renamed, so no old name is kept for it.
- Colors defined by the user would need checks of every color and a way to work out the backgrounds from colors gh-desk has not seen; this ADR leaves them out, and a case for them is decided in a new ADR.
