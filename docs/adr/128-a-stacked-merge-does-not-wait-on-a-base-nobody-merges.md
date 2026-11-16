# ADR-128: A stacked merge does not wait on a base nobody merges

## Status

Accepted
Partly replaced by ADR-129

## Context

ADR-006 bounds the wait of a merge that GitHub blocks after its checks finished with `merge.blocked_timeout`, which covers a second approval or code owners: things that end on their own while gh-desk waits. ADR-104 has a pull request whose base is the head of another merge at work wait for that merge to end, and go on however it ended.

A pull request stacked on another open one, its base being that one's head branch, is blocked until that one is merged and GitHub retargets it. When the merge of the base was not confirmed, or ended without merging (a conflict, a failed write, GitHub refusing it), nothing gh-desk does will unblock the child. It still waited out `merge.blocked_timeout`, and since the time counts only while gh-desk runs and the wait is resumed at every launch (ADR-009), it could sit in the status line as waiting for days, looking as if what it waited for had been lost.

## Decision

- A merge that GitHub blocks after its jobs ended, whose base branch is the head of an open pull request of the same repository, stops without a write unless a merge of gh-desk's is at work on that pull request. Its record goes with it, as with any end that is known. The row names the base pull request.
- While a merge of the base is at work, across batches and on a resume, the child waits as before, bounded by `merge.blocked_timeout`. Once that merge ends, the child decides again from what it reads: retargeted and clean, it merges; still stacked and blocked, it stops.
- Which open pull request has the base branch for its head is read with every read of the pull request, from GitHub, not from the branch names gh-desk read at the start of a batch: a base merged or retargeted meanwhile is seen at the next read.
- A stacked pull request that GitHub does not block, one merged into its parent's branch, is not concerned: it merges as any other.
- Rejected: waiting out `merge.blocked_timeout` as before. It bounds waits that can end on their own; this one cannot.
- Rejected: merging the base first on the user's behalf. The user confirmed the child only; merging another pull request is a write nobody confirmed.

## Consequences

- A base the user means to merge by hand, or on GitHub, is not waited for: the child stops at once, and is merged by confirming it again once GitHub retargets it.
- What blocks a stacked pull request besides its base, such as a review it also lacks, is not told apart: it stops for its base.
- [lanes_test.go](../../internal/runner/lanes_test.go) checks that a resumed stacked merge stops, that a base that did not merge stops its blocked child, and that a child waits while its base is being merged.
