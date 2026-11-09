# ADR-114: The list says a pull request conflicts, and the pane what a merge would do

## Status

Accepted

## Context

Whether a pull request could be merged was known only from the merge information that deciding a merge reads. The TUI read it for a confirm popup, and for pull requests whose checks were pending when `checks.approval` is set. The pane's merge line showed only for those, so a pull request that conflicted with its base looked like any other until a merge was tried on it.

GitHub's `mergeStateStatus` says whether a pull request conflicts or is behind its base. It is worked out per pull request, like the review decision and the checks' rollup, and it changes when the base moves while the head stays where it was.

GitHub may also say that a draft conflicts, rather than that it is a draft. Deciding a merge marked a draft of the viewer's or of a bot ready without looking at the merge state, so a merge of a conflicting draft wrote to GitHub, marking it ready, before it was found it could not merge.

## Decision

- `mergeStateStatus` is a heavy field (ADR-011), read with the review decision and the checks after a list's search. Heavy fields are read again at every reading of a tab, so a pull request that came to conflict because its base moved is shown so at the next reading, by the clock or by r.
- The list has a MERGE column. It says only what CI and REVIEW do not: that a pull request conflicts, is behind its base, or that GitHub is still working it out. One that would merge, or is held only by its reviews or checks, shows nothing, and so does one that is not open.
- The pane reads the merge information of the open pull request it shows, in the background, so its merge line says what a merge would do. It is read once a pull request, and again after r or when the pull request's head changes. A read that fails is not tried again until r, so moving the cursor does not repeat it. A read that finds GitHub still working out whether the pull request can be merged, which it does when first asked, is tried again shortly after, a fixed number of times, so the pane does not keep "unknown" until r.
- A draft that conflicts is skipped by a merge before it is marked ready ([`internal/plan/merge.go`](../../internal/plan/merge.go)).

## Consequences

- The column takes room from TITLE, so on a narrow terminal the columns ADR-017 leaves out go at a wider width than before.
- GitHub may answer that it does not know yet, such as just after a push; the column says so until the next reading.
- Each pull request the pane shows for the first time costs a query with its single checks and changed files, and a read of its workflow runs.
- The settings page's sample pull requests carry a merge state, and its source answers it in their merge information, so the page shows the column as gh-desk does.

## Alternatives

- Reading `mergeStateStatus` with the light fields of a list's search: GitHub works it out for each pull request, which ADR-011 keeps out of the search for speed.
- Marking every merge state in the column: a blocked pull request repeats what CI and REVIEW say, and a mark on every clean row hides the few that conflict.
- A mark beside CI, or before the title: it mixes the checks and whether the pull request merges in one cell, or moves the titles of the rows that have it.
