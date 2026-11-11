# ADR-121: No popup is drawn over another

## Status

Accepted

## Context

A comment (`c`) and a review (`R`, and a review Claude drafts) are written in a popup and then sent through the confirm popup, in two steps, as ADR-043 decided. The confirm popup was drawn over the popup the text was written in, which was left under it so that going back from the confirm popup gave the text box back. On the screen this was a box over a box: two frames, the one under it dimmed and cut off by the one on top.

gh-desk had kept away from this before, each time for one feature: the search of the questions opens in place of the text box, not over the triage's popup (ADR-097), close holds its comment in its confirm popup instead of opening a text box over it (ADR-066), and the release notes are shown in place of the keys list. The comment and the review were the only popups still drawn over another.

The confirm popup already shows the text it sends, so nothing of the text box under it was needed while it was open.

## Decision

- Only one popup is on the screen at a time. When a popup leads to another and the user can go back to the first, the second takes the first's place, and going back draws the first again as it was left, with what was typed in it.
- The confirm popup of a comment or a review takes the place of the popup the text was written in, drawn so that it reads as that popup turned into its confirm, not as another box. Going back from it gives the text box back with the text, as before (ADR-043).
- Writing and sending stay two steps (ADR-043). This decides how they are drawn, not their order.
- Rejected: a text box in the confirm popup of a comment and a review, as close has (ADR-066). Close's comment is optional, which made a text box in its popup fit; a comment and a review exist to send their text, and ADR-043 has it written before the pull requests are read again.
- Rejected: keeping the popup under the confirm popup, dimmed. Two frames on the screen leave the user to tell which takes the keys, which ADR-079 has one frame in the focus color tell.

## Consequences

- A new popup opened from another is drawn in its place, or says in its ADR why not.
- The confirm popup looks a little different by where it was opened from, so that it matches the popup it replaces.
