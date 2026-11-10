# ADR-117: A rule asks whether GitHub would let a pull request be merged now

## Status

Accepted

## Context

Whether the merge button of a pull request can be pressed was shown only in part. The MERGE column marks a conflict and a branch behind its base, and leaves blank both a pull request that would merge and one held by its reviews or checks (ADR-114), so telling the two apart meant reading CI and REVIEW together. Rules could not ask it at all, so a tab of one's own pull requests could not pick those ready to merge, and a tab of review requests could not leave out those that no longer need a review.

Two facts could answer it. GitHub's `mergeStateStatus` is a heavy field read for every row (ADR-114). What deciding a merge would do ([internal/plan/merge.go](../../internal/plan/merge.go)) is read only for the pull request the pane shows and a few others, and answers another question: what gh-desk would do on M. For a pull request held by a missing review it says to wait for the checks and merge, which would count a pull request that still needs the viewer's review as ready.

## Decision

- A rule takes `merge`, a list of states of merging one of which the pull request must be in: `ready`, `behind`, `conflict`, `blocked` and `unknown`. They are read from `mergeStateStatus`. `ready` is what GitHub lets merge, CLEAN, UNSTABLE and HAS_HOOKS alike; `blocked` is held by reviews or required checks, or a draft; `unknown` is GitHub still working it out, or not saying. The values cover every pull request, so "not ready" is the list of the rest (ADR-065). [internal/plan/rule.go](../../internal/plan/rule.go) holds the mapping.
- `merge` is a condition on a heavy field: a pull request whose heavy fields are not read for its head is Pending on it, as on checks and labels, so an exclusion never leaves out a row on a state not read (ADR-062).
- `requested` takes `"@*"`, a review requested from any team, beside `"me"` and `"@slug"`.
- Rejected: what deciding a merge would do, for the reasons above, and because it is not read for every row.
- Rejected: a flag `mergeable = true`. It cannot ask for a branch only behind, which is fixed by updating it, apart from one that conflicts.
- Rejected: GitHub's own values (`clean`, `dirty`, `has_hooks`, ...). Three of them mean the same to a person at the merge button, and DRAFT says nothing a rule cannot ask with `draft`.

## Consequences

- A rule on `merge` waits for the heavy fields, so where it is used rows that wait for it show as not judged yet, as rules on checks do.
- GitHub may say `unknown` for a while after a push; a rule asking for `ready` does not match then, and does at the next reading.
- A value added to `mergeStateStatus` by GitHub falls in `unknown` until it is placed.
