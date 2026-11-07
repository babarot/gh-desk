# ADR-082: Merges of a repository wait their turn across batches

## Status

Accepted
Partly replaced by ADR-104

## Context

ADR-006 decided that a batch runs the pull requests of one repository one at a time, since merging one can leave the next behind its base or in conflict. That order was kept inside a batch only. A batch starts at each yes in a confirm popup, and nothing queues a new one behind those still running, so a user who confirmed five merges in a repository and then a sixth, while the first ones still waited for checks, had two merges of that repository at work side by side. Whether the order held depended on how the user happened to split the selection.

Every write carries the confirmed head (ADR-006), but `expectedHeadOid` checks the head of the pull request merged, not its base: a merge that moved the base does not stop another one already read as clean.

Ordering does not keep a merge from leaving the next pull request behind, nor does it hold off merges made outside gh-desk. What it does is keep gh-desk's own merges of a repository from racing each other, whatever the batches.

## Decision

- The merges of one repository wait their turn in one lane kept by the runner, across the batches it runs. A merge takes its place when its batch starts, so the lane follows the order the batches were confirmed in, and inside a batch the order of its pull requests.
- A merge holds the lane from its turn until its job is done with it: merged, skipped, stopped, failed, or handed to GitHub's auto-merge. Waiting for jobs, for checks and for a branch update happen while it holds the lane, as they did inside a batch. A pull request watched on auto-merge does not hold it, since GitHub merges it on its own time and the watch may outlast gh-desk's interest.
- Approvals, reviews, marking ready and closing do not take the lane: they do not move the base. A merge resumed at launch takes it; a recorded approval or a resumed watch on auto-merge does not.
- A pull request is claimed when its batch starts, not when its turn comes, so a batch started later skips one an earlier batch still waits to act on.
- A batch stopped while its merges wait gives up their places, and the lane goes on with the next.
- The time spent waiting for the turn is not counted in the bounds of the merge's own waits (`checks.wait_timeout`, `merge.blocked_timeout`): those bound how long one pull request waits for GitHub, and a queue ahead would make them depend on how much the user confirmed before.
- The lane lives in one running gh-desk. It is not recorded: the next launch resumes the merges that were waiting, in the order of the record, and a merge that had not yet started is not resumed. A second gh-desk without the record of pending merges (ADR-009) may still merge in the same repository at the same time.
- Rejected: refusing a merge popup while a batch of the same repository runs. It is the simplest, but the user must come back for the rest.
- Rejected: adding the new pull requests to the running batch. Which batch owns a pull request's events, counts, end and cancellation becomes unclear.
- Rejected: one lane for every merge. Unrelated repositories would wait for each other.
- Rejected: holding a lock only around the merge write. A merge waiting for checks would let later ones pass it, and the order would not follow the confirms.

## Consequences

- A merge confirmed later in a repository may wait as long as the merges ahead of it wait for their checks, on top of its own wait.
- ADR-006's "its merges do not knock each other out of date" means only that gh-desk's merges of a repository do not run at once; a merge still leaves the next behind its base, and the next decides again from what it reads.
- A new kind of batch that moves a base must take the lane too.
- [runner_test.go](../../internal/runner/runner_test.go) checks the order across batches, that approvals and other repositories go aside, and that a queued pull request is claimed and gives up its place when stopped.
