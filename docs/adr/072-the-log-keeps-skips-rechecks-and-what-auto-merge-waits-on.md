# ADR-072: The log keeps why the confirm popup skipped a pull request, the rechecks of mergeability, and what auto-merge waits on

## Status

Accepted

## Context

ADR-057 and ADR-071 have the log tell a merge's waits, and keep reads at debug. Three things gh-desk does behind the screen still left no line:

- The confirm popup leaves out pull requests it would not act on (closed, checks failing, changed since read, locked by another gh-desk) and says why on their rows. A row says it only until the list is read again; the runner, which logs a batch, never sees them. After the fact, the log did not tell why only some of the pull requests went.
- While GitHub works out whether a pull request can be merged, the runner reads it again for a while before it decides. This waits, but was not logged as a wait.
- Watching a pull request on auto-merge can last a day, and logged only how it ended. What GitHub's auto-merge waited on in between (a required check, a review, a branch behind) did not show.

## Decision

This adds to ADR-057 what is logged at info; reads stay at debug.

- A pull request the confirm popup leaves out is logged with what the popup does and why it was left out, as its row says, when the user confirms.
- Reading a pull request again while GitHub works out whether it can be merged is a wait, logged as the waits of ADR-057 are, on its mergeability.
- Watching a pull request on auto-merge logs GitHub's merge state when the watch first reads it and each time it changes. A read that finds it as it was logs nothing.
- The attribute keys are the ones [`internal/logging`](../../internal/logging) holds.
- Rejected: logging the merge state at every read of the watch. A watch of a day reads every minute, and nearly every read finds it as it was.

## Consequences

- A reason the popup gives for leaving a pull request out is logged as written, so it must stay free of what ADR-003 keeps out of the log.
