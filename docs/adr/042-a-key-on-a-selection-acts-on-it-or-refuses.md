# ADR-042: A key on a selection acts on it or refuses

## Status

Accepted
Partly replaced by ADR-043
Partly replaced by ADR-044

## Context

With pull requests selected, some keys of the list acted on the selection (approve, merge, ready for review, comment), review refused, and the rest (hide or done, the browser, asking Claude) acted on the pull request under the cursor without saying so. The menu of enter told those last three as one pull request at a time, while their keys still ran. A user who selected ten pull requests and pressed the key of done had one marked done and believed ten were.

Hiding a pull request also left it selected. It was out of sight, but a later approve or merge on the selection still took it.

## Decision

- With a selection, a key of the list that acts on pull requests acts on the selection, or refuses and says why. It never acts on the cursor's pull request instead. The menu of enter tells the same as the key does.
- Hide or done and the browser act on the selection. Review and asking Claude refuse: a review and a conversation are about one pull request read on its own.
- Hide or done acts on the selected pull requests of the tab shown, as it does on one: hidden in a section, done in the inbox. Those of other tabs stay selected.
- A pull request put away leaves the selection, so nothing out of sight is acted on by a later key.
- Done on many is not confirmed, as done on one is not: it removes a notification and writes no review or merge.

## Consequences

- A new key that acts on pull requests decides what it does with a selection, and the menu's line for it says so.
- Opening many pull requests in the browser opens as many browser tabs; there is no limit.
