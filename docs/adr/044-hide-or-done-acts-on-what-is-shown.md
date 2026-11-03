# ADR-044: Hide or done acts on what is shown

## Status

Accepted

## Context

ADR-042 has hide or done act on the selected pull requests of the tab shown, without a confirm. A selection is not what the screen shows: a filter typed after selecting, the rules a tab is narrowed to, or the inbox leaving settled pull requests out can leave a selected notification off the screen. Done on the selection marked those done too, though the user never saw them, and gh-desk cannot take a done back. Approve and merge also act on a selection out of sight, but their confirm popup lists every pull request before anything is written; done has no popup.

A done that failed brought its notification back to the inbox, but not to the selection. After a comment or a review that failed on some, those are selected again; after done they were not, so which ones failed had to be read from the notifications.

## Decision

- Hide or done acts on the selected pull requests the tab shows: through the filter and the rules, and in the inbox not left out as settled. A folded group counts as shown, since its header is. The selected ones not shown stay selected, and the status line says how many.
- Done on many stays without a confirm (ADR-042): what it acts on is on the screen.
- A selected pull request whose done failed is selected again. One that was not selected before is not: a failure does not make a selection that the next key would act on.
- This replaces, in ADR-042, "Hide or done acts on the selected pull requests of the tab shown, as it does on one: hidden in a section, done in the inbox. Those of other tabs stay selected." with the first sentence above, and adds the third to "A pull request put away leaves the selection, so nothing out of sight is acted on by a later key."
- Rejected: a confirm popup for done on many, acting on the whole selection as approve and merge do. Putting away is done often and fast, and a confirm on every batch of it costs more than leaving out what is not shown.

## Consequences

- To put away a selection the filter hides, the user clears the filter, or shows the settled ones, first.
- What counts as shown for done is what the rows of the tab show; a change to how a tab chooses its rows changes what done acts on.
