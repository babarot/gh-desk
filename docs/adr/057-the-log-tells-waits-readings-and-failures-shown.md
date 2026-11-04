# ADR-057: The log tells how long a merge waited, when the lists were read, and every failure shown

## Status

Accepted

## Context

ADR-003 logs every write to GitHub at info and leaves reads and the reasons behind merge decisions at debug. A log written at the default level therefore said nothing between a merge's approval and its merge: a pull request that waited minutes for its checks, or for approval checks, or stayed blocked until it timed out, left no line until it ended. It also said nothing about reading the lists again, by r or by the clock, so it could not tell what the list on screen was as of. And some failures were said on the status line only: the status line keeps a message for a moment (ADR-025), so once it was gone nothing told what had gone wrong.

## Decision

- A merge that waits is logged at info when the wait starts, when what it waits on changes, and when it ends, with what it waited on (checks, approval checks, or another rule that blocks it), how it ended, and how long it took. The polls while it waits stay at debug.
- Each reading of the lists again is logged at info once, when every tab it reads is done: who started it (r or the clock), how many pull requests it read, how many tabs failed, and how long it took. The reads it is made of stay at debug.
- A failure the status line says is logged as well, at warn or error, by the code that says it, unless the code where it happened already logs it. A failure is logged once.
- This replaces, in ADR-003, "Reads and the reasons behind merge decisions are logged at `debug`." for the waits of a merge and the reading of the lists again; other reads and the reasons behind merge decisions stay at debug.
- Rejected: logging every failure inside the status line's own code. Failures that are logged where they happen, with more attributes than the line says, would be logged twice.

## Consequences

- A new wait in a merge, or a new way to read the lists again, is logged as these are.
- A new failure said on the status line comes with a log line, or a reason why the place it happened logs it.
- At the default level the log grows by a line each time the clock reads the lists again.
- What ADR-003 never logs (tokens, bodies, diffs, comments, reviews, Claude conversations, typed text) stays out of these lines too; a failure's text is an error, not the content it failed on.
