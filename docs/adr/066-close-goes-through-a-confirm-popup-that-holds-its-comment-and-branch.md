# ADR-066: Close goes through a confirm popup that holds its comment and branch

## Status

Accepted

## Context

gh-desk can approve, merge, review, comment on and mark ready a pull request, but not close one. Closing is what the user does to the pull requests that will not be merged: a bot's update made obsolete by a later one, a change superseded by another pull request, a contribution that went stale. These come in numbers, as the bots' updates do, so closing has to work on a selection as merge does (ADR-042).

Closing is often done with two more things: a comment saying why, and deleting the head branch. GitHub's mutation that closes a pull request takes neither; a comment is a write of its own, sent before the close as GitHub's own "Close with comment" does, and the branch is deleted by deleting its ref. Unlike a merge, a close and a deleted branch can be taken back on GitHub (reopen, restore the branch); a comment cannot.

A head branch cannot always be deleted: it may be in a fork, the user may not be allowed to, it may be gone already, or it may be the base of another open pull request, a stack. Deleting the base branch of an open pull request closes that pull request too, so deleting one branch can close pull requests the user did not choose. GitHub says whether the user can delete a pull request's head branch only once the pull request is closed or merged; while it is open, the answer is always no.

A comment and the choice of the branch could be asked before the confirm popup, in a text box of their own as a review is (ADR-043), or in the confirm popup. Asked before, every close passes through a text box even when nothing is to be written, which is the common case for a bot's pull request. Asked after, behind keys in the popup that open other screens, the user does not see that the popup has them. A mock of each was tried before deciding.

## Decision

- Close acts on the selection or the pull request under the cursor through one confirm popup and the runner, as merge does (ADR-006, ADR-042). The popup reads each pull request again and lists what will be done to each and why the rest is skipped. Nothing is written before the user confirms in it.
- The confirm popup holds the comment and the branch: it shows a text box for an optional comment and a check for deleting the head branches, both drawn from the start, both empty. The text box takes the keys only while the user has put the focus in it: outside it, keys are the popup's and are not typed; inside it, every key is typed and none confirms the popup. The same comment goes to each pull request, as `c` does on a selection.
- A comment written in a popup that is closed without confirming is kept, and is in the text box the next time close is opened, as a failed comment is kept for `c`.
- A paste while the text box does not have the focus is not put into it: pasted text goes only where the user put the focus.
- For each pull request the runner writes, in order: the comment if there is one, the close, and the deletion of the head branch if it was checked. It reads the pull request again before each of these writes and decides again, as ADR-006 has every write do: one whose head moved or that is no longer open stops, and the branch is deleted only if what keeps it (below) still does not hold. Neither the close nor the deletion of a ref takes an expected head on GitHub, so this reading just before is the only check. A step that fails stops the steps after it on that pull request, and the row and the notification say which steps were done.
- A head branch is deleted only when it is in the pull request's own repository, the user can push to it, it is still at the head the user saw, no open pull request has it as its base, and, read again after the close, GitHub offers the user to delete it. Otherwise the branch is kept and the popup says why on that pull request's line; the pull request is still closed. The check is not a reason to skip the close.
- A pull request that is not open is skipped. One the user cannot close, or whose conversation is locked to the user while a comment is written, is told in the popup from what is read again, not found out by a failed write.
- Close has no default key: it is reached from the menu of enter (ADR-022), and a key can be given to it in `[keys]` (ADR-004). A key of its own on the list would let one stray key press start closing pull requests.
- Each write is logged at info with its pull request, step, result and request ID (ADR-003); the comment's text is not logged.
- A close is not recorded for the next launch (ADR-009): each of its writes ends at once, and the record would hold the comment on disk.
- The mouse follows ADR-053: the check is changed by a second click, as any choice in a popup, and the text box takes no mouse input while it has the focus, so that a click does not throw away what was typed. The mouse never confirms the popup.
- Claude's plans do not include close (ADR-018). Closing someone's pull request with a comment in the user's name is the user's to start.

## Alternatives

- Rejected: a text box before the confirm popup, with the branch as a kind to tab through, as a review is written (ADR-043). Every close would pass through a text box, though most closes have nothing to write.
- Rejected: a confirm popup that says the comment and the branch on lines of its own, changed by keys that open a text box over it. In the mock the user did not see that the popup could take a comment or delete a branch until told the keys.
- Rejected: one popup whose fields are moved through with tab, the text box among them. Where tab is in the text box, it is typed or moves on, and either surprises half the time.
- Rejected: deleting the branch first and letting GitHub close the pull request. It closes without the comment's order and cannot be done where the branch is kept, so the close itself would depend on the branch.
- Rejected: deleting a branch that is the base of an open pull request. It would close pull requests the user did not choose.
- ADR-043 rejected a message field in A's popup because it left R's own review unchecked. That reason is about reviews: close has no other way to write its comment, so its field in the popup leaves nothing unchecked.

## Consequences

- The confirm popup of close is the first that takes typed text. The popup has to keep the text box and the check in sight when the list of pull requests is long, and its keys are read only while the text box does not have the focus.
- The check for the branch needs more than the close reads: whether the branch can be deleted and whether an open pull request is based on it. A pull request is read with this only for close.
- A comment sent and a close that failed after it leave a comment saying a pull request is closed on an open one. The row says so, and the user closes it again.
- Whether deleting a base branch closes the pull requests on it, rather than retargeting them, is what GitHub does today. If GitHub changes this, the rule that keeps such a branch can be loosened, not dropped silently.

## Not decided here

- Whether the check for the branch can start checked by a setting. It starts empty until users ask otherwise.
