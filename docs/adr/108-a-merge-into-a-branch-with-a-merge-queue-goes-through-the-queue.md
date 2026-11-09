# ADR-108: A merge into a branch with a merge queue goes through the queue

## Status

Accepted

## Context

A repository can require that its branch be merged only through a merge queue. GitHub then refuses `mergePullRequest` ("Changes must be made through the merge queue"), and a pull request in the queue is refused too ("Pull Request is in the merge queue"). While it is in the queue GitHub reports its merge state as unknown, so a merge that waited for GitHub to work out whether it can be merged gave up after a while. Auto-merge on such a branch puts the pull request in the queue once it can be merged and is then off, which the watch of auto-merge took for being turned off.

GitHub says of each pull request whether its base branch has a merge queue and whether it is in it. Being in the queue is not the same as being merged: the queue runs its own checks and may take it out.

## Decision

- Whether a pull request goes through a merge queue is what GitHub says of its base branch when it is read. There is no setting: a merge GitHub would refuse is never tried.
- A pull request whose base has a merge queue is merged by putting it in the queue, on the head the user confirmed, as every merge write takes it. Where gh-desk would merge it now, it puts it in the queue; where it would turn on auto-merge, it turns it on, and GitHub puts it in the queue. Waiting for jobs at work, approving first and marking ready first are as for any merge, and the write is inside the repository's lane as ADR-104 says.
- One already in the queue is only followed, whoever put it there: no write, and its unknown merge state is not waited on.
- Its branch is not updated when it is behind its base, whatever `merge.update_branch` says, since the queue tests it on the latest base.
- A pull request in the queue is followed as auto-merge is: its record is kept, it is at work for the merges based on it, and the follow ends when it is merged, closed, or leaves the queue unmerged, or after a time. A merge that could not wait for the record (another gh-desk holds it) still puts it in the queue, and leaves it there unwatched.
- Rejected: merging as an admin past the queue whenever the viewer may. A ruleset that requires the queue is the repository's choice; an admin merge is left to what GitHub says the viewer may do, as ADR-048 says.
- Rejected: a setting to turn queues off. Without the queue GitHub refuses the merge, so turning it off would only fail.

## Consequences

- The merge method of a queued pull request is the queue's, not `merge.methods`; the popup says so.
- The end of a queued merge is known only by following it, so the result of the batch says it is in the queue, and its end comes later, as auto-merge's does.
- A pull request taken out of the queue (a failed check of the queue, or a person) stops; confirming it again puts it back.
- A new way to merge must check the queue first, since it decides which write GitHub takes.
- [merge.go](../../internal/plan/merge.go) decides the steps; [queue_test.go](../../internal/runner/queue_test.go) checks that a pull request is put in the queue, never merged directly, and followed there.

## Not decided here

- Jumping the queue, taking a pull request out of it from gh-desk, and showing its place in the queue.
