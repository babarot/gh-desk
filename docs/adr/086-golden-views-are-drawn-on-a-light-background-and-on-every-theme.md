# ADR-086: Golden views are drawn on a light background and on every theme

## Status

Accepted

## Context

ADR-085 made golden files hold the colors of a screen, and left open which themes and which background they are drawn on. Every golden view was drawn on the default theme, on a terminal that never told its background, so the dark theme was the only one any test drew. The switch to the light theme when the terminal says its background is light, and every palette but the default, ran in no test.

Drawing a few screens on a light background found a fault at once: the text boxes kept the colors Bubble Tea's text area has for a dark background, and drew the line being typed in dark text on black.

## Decision

- A set of screens, one of each kind of thing the screen shows, is drawn as golden views on a light background as well as on the default dark one, with the terminal telling its background before anything is read, as a terminal answers at the start.
- One screen is drawn as a golden view on every theme, on the background the theme is for. The themes are taken from the list of themes, so a new theme gets its golden view without a test being written for it.
- The screens and the tests are in [themes_test.go](../../internal/tui/themes_test.go).
- Rejected: drawing every golden view on every theme and background. The files would multiply by the number of themes, and a change of one color would rewrite most of them, so their diff would no longer show what a change touched (ADR-085).

## Consequences

- A new kind of thing on the screen, such as a new pane or a new kind of popup, is added to the screens drawn on a light background.
- A new theme adds a golden file, and a change of a palette rewrites that theme's file.
- Something drawn with colors of its own, not from the theme, shows in the light background's golden views as a color outside the light theme's palette.
