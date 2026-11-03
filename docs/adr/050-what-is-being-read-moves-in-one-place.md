# ADR-050: What is being read moves in one place

## Status

Accepted

## Context

ADR-030 put the spinner wherever something was not read yet: in each cell of a column being read, in place of a count, and before a line that says what is going on, with `theme.wave` putting each line of a table a frame ahead and `theme.shimmer` running a light over the line. Each place was right alone, but they show at the same time. A tab of twenty pull requests being read turned some sixty spinners in its cells, another in its count, and a shimmer in its line; the confirm popup of a merge or an approval of a few pull requests turned one in its title, one on each row and one at its foot, with the shimmer over the foot. The motion said no more than one spinner would, and drew the eye from the rows.

A bar of how far a reading is was tried in the confirm popup. Its reads come back a few pull requests at a time, so a popup of a few was read in one go: the bar went from empty to full at once. Filling it a cell at a time after such a read showed progress that was not there, and held the question back for it. A tab's reading is not like that: its search comes page by page and its heavy fields in many reads, so a large tab has real progress to show.

## Decision

- A cell or a count not read yet does not move. A cell shows a still mark in its place, one cell wide on any terminal as a spinner's frame is (ADR-030). A tab's count is the one read as soon as it is known, while the rest of the tab is still read.
- How far the reading of the shown tab is, is said on the status line, as work in progress (ADR-025): while it is the tab's first reading, which has nothing to show before it, and while it is r's. The reading the clock starts says nothing, nor the one of a tab shown from the cache: the rows are there, and they change in place.
- A tab that lists more pull requests than one read of heavy fields takes is shown with a bar, of a cell for each pull request up to a constant number of cells. A pull request counts once when it is listed and once when its heavy fields are back, so that the bar moves while the pages of the search come too. A tab whose count is not known yet, a small one and the inbox are said with the spinner instead.
- A bar shows only what is back. It never fills ahead of a read, and nothing waits for it to fill.
- The confirm popup turns one spinner, at its foot, with the count read so far and no shimmer. Its rows do not move: one not read yet is marked as such, and fills in when its read is back.
- `theme.wave` is gone: no table has a column of spinners left to put out of step.
- This replaces, in ADR-030, "`theme.wave` is the only offset: each line of a table a frame ahead of the one above" (the spinners have no offset), "in cells and counts it stays, so a column still shows that it is not read" (cells and counts have no spinner), and, in its Consequences, "takes the spinner as a cell, as a count, or before a line through the one function that adds the shimmer" (a new place takes it before a line only, and a popup's line has no shimmer).
- Rejected: filling a bar a cell at a time after a read that brought back many. It shows progress that is not there and holds back what comes after it.
- Rejected: one spinner on every row being read, without the wave. It still turns as many as there are rows.

## Consequences

- A cell being read and a count not known look the same whatever `theme.spinner` is; only lines that say what is going on show the spinner chosen.
- The status line depends on how far the shown tab's reading is: what is listed and which heavy fields are back belong to the reading, and a new kind of read for a tab must count in it or be said apart.
- A config file that still has `theme.wave` is told that it is not a setting, as any key gh-desk does not know (ADR-014).

## Not decided here

- The progress of a batch on the status line keeps the spinner and its count: its steps come apart, one pull request at a time, and were not found too much.
