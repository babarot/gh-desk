# ADR-127: A popup opens where the eye is

## Status

Accepted

## Context

ADR-125 drew every popup from the same row near the top of the screen, under the header and the list's column heads, so that a popup whose height changes keeps its title and text box in place. That row is far above where the eye rests on a terminal, the middle of the screen: every popup made the eye go up to its top edge, the further the taller the terminal. A popup that opens small (the menu, the confirm popup, the picker) took the top rows and left the rest of the screen below it.

Drawing every popup a quarter down the screen instead was tried in a mock: the small popups came near the middle, but a tall one (the `?` list) then ran to the bottom of the screen with the space above it left empty, and had fewer lines than before.

## Decision

- When a popup opens with none open, it is placed a quarter down the screen. A popup too tall to fit below that is placed higher, as far from the top of the list as from the status line, and never above the row ADR-125 drew popups from.
- The row is decided once, when the first popup opens, by the height that popup has then. It holds while popups are open: a popup that takes the place of another (ADR-121) opens on the same row, and a popup whose height changes grows or shrinks downward from it. It is decided again when the terminal is resized, and when a popup opens after all have closed.
- The row is not a setting. A quarter, the measure of where the eye is, is a constant.
- This replaces, in ADR-125, the sentence "Every popup is drawn from the same row near the top of the screen and grows downward, scrolling past the height it may take." The rest of that bullet, and its rejected alternative, still hold.
- Rejected: every popup a quarter down. A tall popup runs to the bottom, with the space above it empty, and loses lines it had.
- Rejected: shrinking a popup so that the space under it matches the space above. A tall popup loses a third of its lines, and scrolls for what it showed before.
- Rejected: centering each popup by its height as long as it is open. It moves whenever its height changes, which ADR-125 rejected.
- Rejected: placing a popup by the row of the cursor. It would open at a different height for each row, and have little room near the bottom.

## Consequences

- A popup that grows after it opens (a long answer from Claude in the triage) can reach the bottom of the screen and scroll there, though it would have fitted had it opened that tall; opening it again places it anew.
- A popup that shrinks after it opens (a section of the `?` list folded) stays where it opened, above the middle.
- The settings page draws its popups by the same rule, in [internal/config/ui/web/tui.js](../../internal/config/ui/web/tui.js), and must change with it.
