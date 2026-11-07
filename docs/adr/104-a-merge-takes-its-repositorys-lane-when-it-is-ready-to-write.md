# ADR-104: A merge takes its repository's lane when it is ready to write

## Status

Accepted

## Context

ADR-082 put the merges of a repository in one lane, and a merge took its place when its batch started and held the lane while it waited for jobs. A pull request that could be merged at once waited behind one whose jobs had just started. That order came from nothing the user chose: inside a batch it was the order of the list, and a selection keeps no order. ADR-082 rejected holding the lane only around the write, since a merge waiting for checks would let later ones pass it and the order would not follow the confirms.

What the lane must prevent is two of gh-desk's merges reading a repository as clean at once. That needs the read before a write and the write to be inside the lane; it does not need the wait for jobs to be.

## Decision

These replace, in ADR-082, "A merge takes its place when its batch starts, so the lane follows the order the batches were confirmed in, and inside a batch the order of its pull requests", "A merge holds the lane from its turn until its job is done with it", "Waiting for jobs, for checks and for a branch update happen while it holds the lane, as they did inside a batch", "the next launch resumes the merges that were waiting, in the order of the record, and a merge that had not yet started is not resumed", and the rejection of "holding a lock only around the merge write". They replace, in ADR-006, "those of one repository one at a time" for merges.

- Every confirmed merge starts at once and is recorded from then on, so a next launch resumes all of them; the merges wait for their jobs side by side.
- A merge enters its repository's lane once it is ready to write: no job is at work on its head, and it was decided to merge, to turn on auto-merge, or to update its branch. The lane follows that order, whatever the batches and the list.
- In its turn a merge reads the pull request again and decides again. A decision that it must wait after all gives up the lane.
- The read before a write that may move the base, and that write, are inside the lane. Turning on auto-merge is such a write, since GitHub may merge at once.
- A merge that updates its branch holds the lane until it is merged or given up: another merge passing it would leave it behind again, and branches updated side by side run their jobs again at every merge. As ADR-082 says the lane lives in one running gh-desk, a next launch does not keep this hold; a merge passed then updates again.
- A pull request whose base is the head of another merge at work waits for that merge to end before it enters the lane, across batches and on a resume, since a child merged first moves its parent's head. A merge handed to auto-merge is at work until its watch ends. How the parent ended does not matter. A pull request whose bases lead back to itself stops without a write rather than wait, since no such wait would end; the first to find it stops, which leaves the other free. Branch names are read when a batch starts, not recorded, and a batch that cannot read them writes nothing. A dependency that the bases do not show is not seen: a user who needs an order confirms the next merge after the last one ended.
- The waits of a repository share one light read of all of them. It decides only whom to wake: what decides a merge is the full read, as ADR-068 says. A pull request is left asleep only while the checks not finished at its last full read were all jobs at work, since a commit status that waits for an approval keeps the rollup pending after the jobs end. Every waiting pull request is woken after a time that is a constant, whatever the light read says.
- Rejected: sorting a batch when it starts. It uses what is true at the confirm only: it does not help a merge confirmed later, or one whose jobs end after the confirm.
- Rejected: a merge at the head of the lane giving up its turn after a time. A merge that is ready waits that time, and the time would have to fit every repository's jobs.

## Consequences

- The order of merges follows when their jobs end. It cannot be chosen by confirming in an order, nor told in advance, and which of two pull requests that touch the same files stops on a conflict depends on it.
- In a repository that requires branches to be up to date, merges run one after the other as before, and jobs at work on a pull request that falls behind are wasted.
- Approvals go out as soon as jobs end, not in the turn of the merge, so more of them stay on a pull request whose merge stops later.
- Every waiting merge reads at once, which the shared read of ADR-105 keeps to one read a repository.
- A new step that moves the base must be inside the lane; a new wait must stay outside it.
- [lanes_test.go](../../internal/runner/lanes_test.go) checks that a merge passes one whose jobs are at work, that merges of a repository do not overlap, and that a stacked pull request waits for its base.
