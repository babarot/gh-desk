# ADR-085: A golden view holds the colors as well as the text

## Status

Accepted

## Context

The golden files of `internal/tui` held what a screen drew with its escape sequences stripped. A change of color, of bold or of reverse passed every test, though the colors carry meaning on this screen: the focused pane by its frame (ADR-079), a row's state, the selection, Claude's orange on every theme (ADR-081). A theme could swap two colors of its palette, or a style could lose its foreground, and nothing would fail.

gomi, another Bubble Tea v2 program, compares its golden views by the characters on screen and then by the runs of styled cells on each line, and has done so across all its screens.

## Decision

- A golden file holds two parts: the characters on the screen, then, for each line, the runs of cells that share a style, with their colors and attributes. A change of color fails a golden test as a change of text does. The format and the comparison are in [screen_test.go](../../internal/tui/screen_test.go).
- The comparison is by how the screen looks, not by the bytes written: the same color written in two ways is one color, and what a blank cell cannot show (its foreground, bold) is not compared.
- A 24-bit color may differ by one in each channel. Renderers round colors differently, and a difference that small cannot be seen.
- An escape sequence other than SGR in a golden view fails the test instead of being dropped. Something drawn that the format cannot hold must be decided on, not lost.
- `screen`, which strips the styles, stays for the checks that look for text on the screen; only golden files hold the styles.
- Rejected: keeping the escape sequences in the golden files as they are written. A change of renderer that writes the same look in other bytes would rewrite every file, and the files could not be read in a diff.

## Consequences

- Changing a color of a theme or a style rewrites the golden files of every screen that shows it. Their diff is the list of what the change touched, and is read as such before `-update` is accepted.
- Drawing anything other than text and SGR, such as a hyperlink or an image, needs a decision on how golden views hold it before such a screen gets a golden file.

## Not decided here

- Which themes and which background, dark or light, the golden views are drawn on.
