# ADR-006: Bulk approve and merge: per pull request, one repository at a time, on the confirmed head

## Status

Accepted
Partly replaced by ADR-046
Partly replaced by ADR-068
Partly replaced by ADR-082
Partly replaced by ADR-092

## Context

The main use of gh-desk is clearing dependency bot pull requests: select a dozen renovate pull requests, approve them, merge them. Approvals and merges cannot be taken back, and the state they act on moves while gh-desk works:

- In repositories that require an approval check (a policy bot) or a second approval, a pull request is `BLOCKED` until the approval is in, then its checks run again.
- After a merge, the other open pull requests of the same repository are `UNKNOWN` for a while, then often `BEHIND` or `DIRTY` (bots touch the same lock files).
- Bots push new commits to their pull requests at any time; renovate also stops updating a branch someone else committed to.
- A review's `commitOID` records which commit was reviewed, but GitHub accepts a review of an older commit, and an approval may count for a newer head depending on the repository. Only `mergePullRequest`'s `expectedHeadOid` refuses to merge a head nobody confirmed.
- Repositories allow different merge methods, and only some allow auto-merge.

## Decision

- A batch is started from one confirm popup, for the selection or the pull request under the cursor. The popup reads each pull request again and lists what will be done and why the rest is skipped. A pull request whose head moved since the list was read is not sent: the user saw another diff.
- `internal/plan` decides each pull request's next step from fresh information: merge now, turn on auto-merge, wait for checks, update the branch, read again (`UNKNOWN`), or skip (closed, draft, conflicts, required checks failing, no allowed method, a bot's branch behind its base). Whether a check is required comes from `isRequired`; rulesets are not read.
- `internal/runner` runs a batch: every pull request is its own sequence (approve if needed, then merge as soon as that approval is in), pull requests of different repositories run side by side, those of one repository one at a time. It reads the pull request again before each write and decides again, so a decision taken when the user said yes is checked against what is true when it acts.
- Every write carries the confirmed head: the review's `commitOID`, the merge's and auto-merge's `expectedHeadOid`, the branch update's `expectedHeadOid`. A head that moved stops the pull request ("head changed · confirm again"); the only new head taken is one gh-desk made by updating the branch, after checking its parents include the confirmed head.
- Waiting is bounded twice, by settings since how long CI runs and how long another person's approval takes differ between repositories and teams: required checks still running (`merge.checks_timeout`), and blocked after the required checks finished (`merge.blocked_timeout`), which covers a second approval or code owners without reading rules. Pull requests put on auto-merge are watched until merged, closed, or auto-merge is turned off, for a time that is a constant: the merge is GitHub's, so the time decides only how long gh-desk watches for the result, and a watch that ends says that auto-merge stays on.
- Auto-merge is used where the repository allows it, and the popup says that auto-merge also merges commits pushed later, as GitHub does. When GitHub refuses auto-merge because the pull request is already clean, it is merged directly.
- Skipped pull requests leave the selection at once and keep their reason on their row; after an approve batch the approved ones stay selected, so `M` merges the same set next.

## Consequences

- Nothing is approved or merged on a head the user did not see, except commits GitHub's own auto-merge takes, which the user is told about.
- A long wait goes on only while gh-desk runs; what it waits for is recorded and goes on at the next launch (ADR-009).
- A batch is slower than firing every merge at once, but its merges do not knock each other out of date.
