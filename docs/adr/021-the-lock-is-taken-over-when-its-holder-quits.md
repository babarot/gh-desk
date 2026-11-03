# ADR-021: The lock on pending merges is taken over when its holder quits

## Status

Accepted

## Context

ADR-009 has one gh-desk at a time hold the record of pending merges, under a lock taken at launch. A gh-desk started while another held it ran without merges that wait until it was started again: quitting the other one changed nothing, and what the second one said about the holder was no longer true. Starting it again loses what it showed (results on rows, the selection). Nor could the user tell which gh-desk held the lock, with several terminals and worktrees open.

## Decision

This replaces two sentences of ADR-009: "At launch, the runner goes on with what is recorded" now holds at launch and when the lock is taken over; and "A gh-desk started while another holds the lock works without it" now holds until it takes the lock over.

- A gh-desk without the lock tries it again as its screen opens, when the user is about to merge, and when the user reads the lists again, never on a timer. A timer was rejected: it would start going on with another gh-desk's merges at a moment the user did nothing, in a window they may not be looking at.
- Once it takes the lock, it goes on with what is recorded as it does at launch, and says so.
- A job takes the record and whether it may wait together, when it is made, and keeps them for its run. A job made before the lock was taken never waits, so no merge waits without being recorded.
- The holder writes its process ID and the directory it was started in into the lock file, so the others can name it. The lock is still `flock(2)`; what the file says is only shown, never trusted to decide who holds it.

## Consequences

- Quitting one gh-desk hands its merges that wait to another one still open, at that one's next merge or refresh, not at the next launch.
- A holder from before this change writes nothing into the lock file; the others then name it only as another gh-desk.
- The check stays cheap only while taking the lock does not block; a lock that waits would freeze the screen.
