# ADR-030: One spinner, of one cell, chosen by name

## Status

Accepted

## Context

gh-triage showed one spinner, a braille one written in the code, wherever something was being read: in cells of a table, in place of a count not known yet, and before a line that says what is going on. Dozens can be on the screen at once, and they turn together, since each frame is worked out from the clock and one tick draws them all.

Users differ in how much motion they want and in what their terminal draws well, so the spinner became a setting (ADR-014). Collections such as cli-spinners and bubbles' presets were looked at for what to offer. Many of their frames take more than one cell, are emoji, or contain East Asian Ambiguous characters (`●`, `•`, `◐`, the block elements of a bar, `…`), which a terminal set for Japanese draws two cells wide, so a spinner in a column would move every column after it.

## Decision

- There is one spinner for the whole screen, chosen by name in `theme.spinner` from a set built into `internal/tui`; `config.Spinners` and the TUI's set are kept the same by a test, as the themes are (ADR-019).
- Every frame of every spinner takes one cell on any terminal: no emoji, nothing wider, and no East Asian Ambiguous character. A test checks each frame. A spinner that cannot meet this is not offered.
- A spinner is its frames and how long each shows, both constants (ADR-014). Frames written in the config file, as lazygit allows, were rejected: the width rule would have to be checked there, and they are a choice most users would not make.
- A spinner per place (one for cells, another before a line) was rejected. The places show at the same time, so the same "being read" would look two ways, and the places are a split of the code that users do not see.
- `theme.shimmer` runs a light over the text that says what is going on, and only there: cells and counts have no text. It changes colors, never the text or its width.
- The spinner that does not move leads no text, since the text says what is going on and ends in an ellipsis of its own; in cells and counts it stays, so a column still shows that it is not read.
- The spinners turn in step, from the clock. `theme.wave` is the only offset: each line of a table a frame ahead of the one above. A color pulse was rejected: it would need its colors worked out from the theme on every frame, a second kind of animation for a small difference.

## Consequences

- A new spinner is a line in the TUI's set and a name in `config.Spinners`; the test rejects one whose frames are too wide.
- A new place that shows something is being read takes the spinner as a cell, as a count, or before a line through the one function that adds the shimmer, so that the settings reach it.
- With the shimmer on, the screen is drawn more often while something is read.
