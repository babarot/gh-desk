# ADR-063: A tab's count leaves out what it excludes

## Status

Accepted

## Context

ADR-062 has exclusion rules leave pull requests out of every tab, and the inbox's unread count leave out the notifications it excludes, as it did for settled ones. A search tab's count stayed what GitHub's search counted, the excluded among them. Used with rules that left out a third of a tab, the count in the tab's title no longer matched the rows under it: a tab listing some fifty rows said 86.

## Decision

- A tab's count leaves out what the tab excludes, while it does not show the excluded: the inbox's unread count, and the count of what a search tab's search found. Shown with the key of ADR-062, they count again.
- A search tab's count is GitHub's count less the excluded among the pull requests read; those not read are not known to be excluded.
- This replaces, in ADR-062, "An excluded notification's unread mark is not counted in the inbox's count while it is left out, as a settled one was not." with the first sentence above.
- Rejected: keeping GitHub's count and saying the excluded beside it. The title has room for one number, and the count under the rows already says how many are left out.

## Consequences

- A search tab's count changes as rules come to tell its pull requests, after their heavy fields are read; it is the count read first that ADR-050 shows, less what is excluded so far.
- With more pull requests found than a tab reads, its count is not what its rows would hold if all were read: those beyond the limit are counted, excluded or not.
