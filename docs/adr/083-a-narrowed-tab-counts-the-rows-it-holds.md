# ADR-083: A narrowed tab counts the rows it holds

## Status

Accepted

## Context

ADR-063 has a tab's count leave out what the tab excludes, so that the count in its title matches the rows under it. The filter typed with `/` and the rules of `S` (ADR-027) narrow the rows too, but the count stayed whole: a tab narrowed to one row still said 36. The line under the rows already counted only the excluded the filter matches, and the tab's count was kept whole on purpose then, with no reason written down.

The filter is kept once for every tab, so a count that follows it tells, from the header alone, which tabs hold what was typed. The rules of `S` narrow one tab.

## Decision

- While a tab's rows are narrowed, by the filter or by the rules, its count is what the narrowed rows hold: the pull requests of a search tab, and the unread notifications of the inbox's rows. A folded group still counts. With nothing narrowing, the count is that of ADR-063.
- The excluded are counted as ADR-063 counts them: only while the key of ADR-062 shows them.
- Only a pull request that was read can be matched, so a narrowed search tab counts none of those GitHub found beyond what the tab reads.
- A narrowed count is drawn in the filter line's color, so that it reads as the filter's and not as the tab's whole.
- Rejected: showing the narrowed count beside the whole one. The title has room for one number, as ADR-063 found, and clearing the filter brings the whole count back.
- Rejected: following only the filter typed, not the rules. Both say what the tab shows (ADR-027), and the count would again disagree with the rows.

## Consequences

- A new way to narrow the rows must be one that the question deciding whether a row is shown asks, or the count will not follow it.
- The count of every tab is matched again at every draw while the filter is typed, as the rows of the tab shown are.
