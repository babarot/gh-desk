# ADR-115: The cursor stays on the pull request the user put it on

## Status

Accepted

## Context

The rows of a tab change without the user: a reading again adds and drops pull requests, heavy fields and bodies make exclusion rules leave rows out or let them back, a batch's result changes what a rule sees, and a rule that counts days changes its verdict as time passes. The cursor was a line number. Each place known to change the rows noted the pull request under the cursor and put the cursor back on it afterwards, and every other place left the cursor on its line, now another pull request. Places were missed: `r` put the merged rows away before noting where the cursor was, so with a merged row above it the cursor ended on the next pull request, and the clock's reading did the same after putting away the results that held rows in place (ADR-064). Rows of a tab not shown changed with nothing to put the cursor back. A pull request that left the list left the cursor on the one that moved into its line.

A key acts on the row under the cursor. A cursor moved onto another pull request unseen makes the next key act on a pull request the user did not look at.

## Decision

- The cursor is on a pull request, not on a line. After anything but a key or the mouse changes the rows, and after a key that did not itself move the cursor (an action on a row, switching tabs), the cursor is put back on the row of that pull request. This is done in one place after every message, not by each change of the rows, so a new way for rows to change needs nothing for it.
- The cursor follows a pull request only once the user has pressed a key or used the mouse with it there. Until then it stays on its line, so that a list filling in as it is first read keeps the cursor at its top.
- When the pull request under the cursor leaves the rows by anything but the user, its row stays on its line, as last shown and marked, until the cursor leaves it. The status line says it left. `r`, which puts away the results shown on rows, puts it away too.
- What the user does to leave a row out goes as it did: putting it away or hiding it, a typed filter that does not match it, and folding its group. The cursor then stays on its line.
- Rejected: moving the cursor onto the next or the nearest pull request and saying so. The user still has to see that the cursor moved before the next key, which is the very thing that failed.
- Rejected: noting the pull request at each place that changes the rows, as before. Each new place had to remember it, and several did not.

## Consequences

- A key that moves the cursor tells it by changing the line; one that changes the rows without changing the line is taken as not moving it. A key that moves the cursor and changes the rows at once must leave the line it chose.
- The row kept for the cursor is not among the tab's pull requests: counts, selections by group or rule and the cache do not see it, but an action on the row under the cursor does, and acts on a pull request that may be merged or closed.
- Finding the cursor's row again reads the rows of the shown tab after every message, as drawing does.
- The guarantees are checked by [follow_test.go](../../internal/tui/follow_test.go).
