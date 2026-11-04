# ADR-060: The log viewer takes the keys while it is open

## Status

Accepted

## Context

A batch of approvals and merges can take minutes: pull requests wait for their checks one after another, in parallel across repositories. The rows say what each is doing now, and the status line how many are done, but not how far each has come, what it waits for and since when, or why one was skipped. The log has all of that since ADR-057, and gh-desk keeps it in memory as it is written (ADR-058). It is to be looked at for a moment, while something takes long or many pull requests go at once, and put away again.

The screen has a list, a bottom pane that shows the pull request under the cursor (or the conversation with Claude, ADR-038), a status line, and popups. The claude pane takes every key while it is shown (ADR-040).

## Decision

- A key on the list opens the log viewer, which shows the log's records as they are written. It is not a tab of the preview pane: the preview shows the pull request under the cursor, and the log is about everything gh-desk does.
- While it is open it takes the keys, as the claude pane does. The list's keys do nothing; the moving keys scroll the log; of the keys that work anywhere, only the one that tells the keys does. Moving back stops following the newest records; going to the bottom follows them again.
- The mouse on the log viewer scrolls it and does nothing else. Pointing anywhere else closes it, and what was pointed at takes the click or the wheel, as pointing at the list closes the claude pane (ADR-040). The log viewer writes nothing (ADR-012).
- It is drawn over the right side of the list when the list keeps enough room beside it, and in the bottom pane's place otherwise, whether the preview is on or off. Where neither fits, the key that opens it says why and nothing opens. Its width and the room the list keeps are constants (ADR-014).
- It and the claude pane are never open together: each takes the keys that would open the other.
- It appears and goes at once, with no animation.
- Rejected: leaving the keys to the list while it is open. The list's actions would mix with looking at the log (asking Claude opens a pane under it, moving the cursor changes what the log is narrowed to), the keymap would have to treat its keys as meeting the list's, and it would have to be hidden and shown again around the claude pane.
- Rejected: a popup, a screen of its own, or a band between the list and the preview. A popup or a screen hides the list whose rows say what each pull request is doing; a band leaves both the list and the preview too short.
- Rejected: sliding it in. Bubble Tea can draw frames, but a state that moves with time would have to be held still in the tests and in the playground's replays, for little to look at.

## Consequences

- A key for the log viewer goes in its own scope, read alone while it is open; a new key of the list does nothing there.
- A new way to point at the screen while the log viewer is open must close it first, or be on it.
- What the log viewer can show is what is logged at the level the log is written at (ADR-057, ADR-058).
