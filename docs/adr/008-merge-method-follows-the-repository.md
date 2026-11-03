# ADR-008: Merge method follows what the repository allows, in a configured order

## Status

Accepted

## Context

GitHub's API needs a merge method for every merge, and repositories differ: some allow only squash, some only merge commits, some all three. Which one is right is a repository's convention, which the repository's settings already express by what they allow. A setting per repository in gh-desk would repeat that and drift from it.

## Decision

- The method is the first of `[merge] methods` that the repository allows (`mergeCommitAllowed`, `squashMergeAllowed`, `rebaseMergeAllowed`). The default order is merge, squash, rebase.
- A repository that allows none of them skips the pull request ("no merge method allowed").
- The confirm popup shows each pull request's method, so a surprise is seen before yes.
- There is no per-repository override.

## Consequences

- A repository that allows one method always gets it.
- A repository that allows several gets the user's first preference among them, which may differ from the convention its people follow by habit; such a repository should allow only its method, or a per-repository setting can be added later.
