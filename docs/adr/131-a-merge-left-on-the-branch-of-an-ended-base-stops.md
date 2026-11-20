# ADR-131: A merge left on the branch of an ended base stops

## Status

Accepted

## Context

ADR-104 has a pull request whose base is the head of another merge at work wait for that merge to end, and go on however it ended. ADR-128 stops such a child when GitHub blocks it and no merge is at work on its base, and lets one GitHub does not block merge into its parent's branch as any other.

A user who confirms a parent and its child together means both to land on the parent's base: the child goes there once the parent is merged and GitHub retargets it. When the parent's merge ended without merging (GitHub refused it, it was skipped, the user stopped it), the child was still merged, into the parent's branch, where nobody merges it on. The same happens when the parent is merged but its branch stays, so GitHub does not retarget the child. The user then finds the child merged and its change missing from the branch they meant.

## Decision

These replace, in ADR-104, "How the parent ended does not matter", and, in ADR-128, "A stacked pull request that GitHub does not block, one merged into its parent's branch, is not concerned: it merges as any other" for a child whose parent was a merge at work alongside it.

- A merge is tied to the merge at work it is based on the first time both are at work in one gh-desk with their branches known, whichever was confirmed first. The tie stays after the parent's merge ends.
- Once the parent's merge ended, and no merge at work is one the child waits for, a child whose base is still the parent's head branch stops, whatever GitHub decides of it: with the parent open, at once, as one whose base did not merge; with the parent closed, as one that still targets its branch, once GitHub has had a time to retarget it, a constant as GitHub's own delay is. A child retargeted by GitHub, or by hand, goes on as any other.
- The check is made at every read that decides a write of the merge, so a child stops before any of them, the approval and marking it ready included: an approval may let auto-merge already on merge it.
- Whether the parent's merge is at work or ended is taken on both sides of the read the check uses, and a read across a change is made again, so the check and the wait for a parent at work see the child as it stands after that change. How the parent ended is read from GitHub at that read, not from how gh-desk's merge of it ended, so a parent merged or retargeted by someone else meanwhile is seen.
- A child whose parent was not a merge at work alongside it is not concerned: confirmed alone, it is merged into its base's branch, as the user asked, and ADR-128 stops it when it is blocked.
- Rejected: merging the child into the parent's branch when the parent did not merge, as before. It writes the child where the user did not mean it, and only the user can tell the change is missing.
- Rejected: retargeting the child onto the parent's base on the user's behalf. It changes what the user confirmed, and the diff they saw against the parent's branch is not the diff against its base.
- Rejected: stopping every child whose base is the head of an open pull request. Merging into a parent's branch is a way of working, and a child confirmed alone asks for it.

## Consequences

- A child confirmed with its parent is merged only after the parent, and only once retargeted. In a repository that does not delete merged branches, every such child stops, and is merged by retargeting it and confirming it again.
- What is guaranteed is that no write follows a read that found the parent's merge ended. An approval sent while the parent is still at work, as ADR-104 lets approvals go, is not held back, and a parent that ends in the moment between that read and the write is not caught: auto-merge already on the child may then merge it into the parent's branch. Holding approvals until the parent ends would change ADR-104, and is left to a decision of its own.
- The tie lives in one running gh-desk, as the lane does (ADR-082). A child resumed at a next launch after its parent ended is not tied, and goes on as ADR-128 says.
- [lanes_test.go](../../internal/runner/lanes_test.go) checks that a parent skipped, refused or merged without a retarget stops its child, before its approval too, that a child retargeted late is merged, and that a child confirmed alone is merged into its base's branch.
