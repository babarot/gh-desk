# ADR-064: A row showing a result keeps its place

## Status

Accepted

## Context

A tab can sort drafts after the rest (`drafts_last`) and sorts by when a pull request was last updated. A merge marks a draft of the user's or a bot's ready first (ADR-046), and the batch changes the very fields the tab sorts by: the draft becomes ready at once, and the next automatic reading brings a newer update time. The row jumped from the drafts to the top of the rest while its batch went on, and the user, watching it go through ready, approve and merge, lost it.

The cursor already comes back to the pull request it was on after a reading, but it follows one row, and a batch acts on every selected one. A row that jumps also scrolls the list under the eye, which a cursor that follows it does not undo.

## Decision

- While a row shows a step or a result of a batch (ADR-007), it sorts by what it was when its batch started, not by what the batch or a reading since has changed. Every field a tab sorts by is held this way.
- When `r` puts the results away, the rows sort as they are, as it already removes merged pull requests from the list.
- Rejected: having the cursor follow the row. It does that already for one row; it cannot follow every row of a batch, and it moves the screen as much as the row does.
- Rejected: marking the rows of a batch with a background. They are marked by their progress already, and a marked row that moves still has to be looked for.

## Consequences

- A new way of sorting the rows of a tab must hold its field for a row showing a result too, or such a row moves again.
- Until `r`, a tab is not in its sort order: a draft made ready can sit among the drafts. Its row says what happened to it, which is why it is there.
- The guarantee is checked by [held_test.go](../../internal/tui/held_test.go).
