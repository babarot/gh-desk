# ADR-094: A confirmed action uses up its selection

## Status

Accepted

## Context

A selection outlived some actions and not others: an approve kept it so that `M` merged the same set next, a successful comment kept it with no decision behind it, and a merge took its pull requests out. A key on a selection acts on it, never on the cursor (ADR-042), so a selection left over from the last action is what the next key acts on. Done and the browser have no confirm popup, and a popup read many times stops being read.

The selection also carried two intents: the same set for the next action, and the unfinished part of the same action. They conflict when a batch is partly done. An approve of ten with one stopped left that one selected and dropped the nine approved, so `M` listed only the pull request whose head had moved. The batch's end replaced the whole selection, including one made while it ran.

`M` approves what needs it, so approving first is a choice and not the only way to approve and merge.

## Decision

- Confirming an action takes its pull requests out of the selection, however the action ends. Closing the popup without confirming takes nothing out. The end of a batch does not touch the selection.
- What an action does not act on stays selected: a draft from Claude on another pull request, and what hide or done leaves unshown (ADR-044).
- The last selection an action or the clear key took away can be brought back by the key that clears, when nothing is selected. It comes back without what left the list or was put away, so nothing out of sight comes back (ADR-042).
- What failed or stopped never stays in, or comes back to, the selection. It is kept apart, with the action and its text, and a key opens that action again for those pull requests alone. Writes go through their confirm popup as ever; done has none (ADR-044). Keeping it apart means it acts only when asked and cannot join a selection made since.
- Skipped pull requests are not tried again: a skip is a decision, not a failure.
- A batch from Claude leaves nothing to try again: it acts on what its plan names, not on what the user chose. Nor does a batch of an auto-approve rule, which judges the pull request again on its own (ADR-093).
- This replaces, in ADR-006, "Skipped pull requests leave the selection at once and keep their reason on their row; after an approve batch the approved ones stay selected, so `M` merges the same set next." with: skipped pull requests keep their reason on their row. It replaces, in ADR-043, "After a review sent to every pull request, approved ones stay selected, so `M` merges them next; the others leave the selection. When any was not sent, only those are left selected and the text is kept for the next review, so nothing is sent twice." It replaces, in ADR-044, "A selected pull request whose done failed is selected again. One that was not selected before is not: a failure does not make a selection that the next key would act on."
- Rejected: keeping the selection after an approve, with a confirm on done of many. It leaves the partly done batch unsolved, and ADR-044 refused that confirm.
- Rejected: offering "merge the approved ones" from a batch's result. It adds a third target besides the selection and the cursor; bringing the selection back lets the popup sort it as ever.
- Rejected: retrying through the restored selection. A review with a text or a comment would go again to the pull requests that already have it.

## Consequences

- A new action that acts on a selection takes its pull requests out when confirmed, and says what is left to try again, if anything.
- Approving and then merging the same set takes one more key.
- Only the last action that failed is kept to try again; a later failure replaces it. Rows and notifications still tell of the earlier one.
- A retry after a request that timed out may send a comment twice, since GitHub may have taken it. Retried reviews and comments carry that risk as sending them again by hand does.
