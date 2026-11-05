# ADR-071: A wait logs the jobs it waits on as they end and begin

## Status

Accepted

## Context

ADR-057 logs a merge's wait when it starts, when what it waits on changes (checks, approval checks, another rule), and when it ends, and keeps the reads in between at debug. Since ADR-068 and ADR-069, a merge and an approval wait for the jobs at work on the head, which can take hours. The log of such a wait was two lines: it started, and it ended so long after. It named no job, so the log did not tell what it waited on, which job ended when and how, or that a job only started later, as one that needs another does. The quiet time after the user's yes, marking ready or updating the branch, when nothing is at work yet, was logged as a wait on checks like any other.

The steps of a wait had two names in the log for the same thing: a merge's wait on jobs was `checks`, an approval's `jobs`.

## Decision

This replaces, in ADR-057, "with what it waited on (checks, approval checks, or another rule that blocks it)" for a wait on the head's work, and adds to when a wait is logged.

- A wait on the head's work is one of three: on the jobs at work, on the workflows that may still start in the quiet time, or, with neither, on the required checks still running (as an admin merge waits). The log and the runner's steps tell them apart.
- A wait on jobs or checks names them when it starts. While it waits, a line is logged at info each time one of them ends, with how it ended, or a new one begins, with those still waited on. Those that end the wait, the last of them, are told by the line of its end, with how they ended. A read that finds them as they were logs nothing; the reads stay at debug.
- Job and check names are not text a user wrote, and may be logged; the rules of ADR-003 on what is never logged are unchanged.
- The attribute keys are the ones [`internal/logging`](../../internal/logging) holds.
- Rejected: a line at every read while waiting. A wait of hours polled every minute would fill the log with lines that say nothing changed.
- Rejected: showing more of the wait in the progress pane (the time left of `checks.wait_timeout`, when it was last read). A lane is one line that is cut, not wrapped (ADR-061), and the names would be cut first.

## Consequences

- A wait logs as many lines as its jobs end and begin, not as it reads.
- A new kind of wait on the head's work is named apart from the three, or it is logged as one of them.
