# ADR-012: The mouse points; keys act

## Status

Accepted

## Context

gh-triage was driven by keys alone. Moving to a tab or a row far down the list took several presses, and the preview pane's height was fixed by `preview.height` in the config file, so a long diff or description could only be read a few lines at a time or in the PR screen. claude-recall, another TUI used alongside it, takes clicks, scrolls with the wheel and resizes its pane by dragging its edge, and the same moves were expected here.

Turning the mouse on in a terminal has a cost: the terminal no longer selects text on a plain drag, and Shift (Option in some terminals) has to be held to copy. Mouse actions also bypass the keymap of ADR-004, so they cannot be rebound or turned off one by one.

## Decision

- The TUI reports clicks, the wheel and motion while a button is held (`tea.MouseModeCellMotion`), always; there is no setting to turn it off.
- The mouse only points and moves: a click picks a tab, a row or a preview pane tab, a double click opens a row as `enter` does, the wheel scrolls what is under it, and dragging the preview pane's top border resizes it. Nothing that writes to GitHub (approve, comment, merge, marking done) is done with the mouse; those stay on keys, which are rebindable and shown in the hints.
- A popup, the filter being typed and the review being written take no mouse input, so a click never acts behind what has the focus. A click closes the `?` list.
- The preview pane's height can also be changed with keys (`pane_grow`, `pane_shrink`), bound like any other action.
- A height set with the mouse or those keys is kept in `layout.json` in the state directory and is used over `preview.height` at the next launch. `internal/state` owns the file, as it owns the other state kept between launches; it is not locked, and when two gh-triage write it the last write is kept.

## Consequences

- Copying text from the screen needs Shift or Option held.
- Mouse handling lives in `internal/tui/mouse.go` and finds what is under the pointer from the same values the drawing uses (the header's tab labels, the layout's line counts, the preview pane's tab names). A change to how a part is drawn has to keep those in step; the tests in `mouse_test.go` click on what the screen shows to catch a drift.
- Once a height is kept, a new `preview.height` changes no height until `layout.json` is removed; it still decides whether the pane is shown at start (`0` hides it). The config file's comment says so.
- A later mouse action that writes to GitHub would need a new ADR.
