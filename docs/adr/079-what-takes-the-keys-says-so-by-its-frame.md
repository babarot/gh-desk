# ADR-079: What takes the keys says so by its frame

## Status

Accepted

## Context

The claude pane (ADR-040) and the log viewer (ADR-060) take every key while they are shown; the list's keys do nothing. Neither is a popup: a popup dims the screen under it, and these leave the list in its colors, since its rows tell what each pull request is doing, which is why the log viewer was not made a popup. Their frames were drawn in the same faint color as the preview's, which takes no keys. Nothing on the screen told where a key goes, and a key of the list pressed while one was open did nothing without a sign why.

Popups take every key too, and their frames were drawn in that faint color as well; only the dimmed screen under them told that they did.

## Decision

- What takes the keys while it is shown draws its frame in the theme's focus color: a popup, and a pane that takes the keys. What takes none keeps the plain border color, even when it is drawn over the screen, as a notification is: the focus color says where a key goes, not what is on top.
- The focus color is the theme's accent, the color of keys and of the tab shown, so a theme gives it without a new entry (ADR-019). Which color it is lives in [internal/tui/styles.go](../../internal/tui/styles.go).
- What is under a pane that takes the keys is not dimmed: it is no popup, and the list must stay readable beside it. Under a popup the screen is dimmed as before, which leaves one frame in the focus color on the screen.
- Boxes inside a pane (the claude pane's text box) keep the plain border; the pane's frame alone tells the focus.
- Rejected: drawing in the focus color whatever is drawn over the screen. A notification would then look as if it took the keys.
- Rejected: dimming the screen under the panes, as popups do. It would hide the rows' state the log viewer is opened beside.
- Rejected: leaving the list's keys to the list while a pane is open, with the pane's keys taking precedence. ADR-060 rejected it already: the moving keys are wanted by both, and what the rest do would happen out of sight.
- Rejected: a marker in the title instead of the frame's color. It reads only when looked for; a frame in another color is seen from anywhere on the screen.

## Consequences

- A new popup or pane that takes the keys draws its frame in the focus color; one that takes none must not.
- A theme whose accent is close to its border color tells the focus poorly, and nothing checks for it.
