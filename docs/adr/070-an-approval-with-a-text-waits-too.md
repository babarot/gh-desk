# ADR-070: An approval with a text waits for the jobs too, its text kept in the record until it is sent

## Status

Accepted

## Context

ADR-069 let an approval wait for the jobs at work, and kept an approval with a body out of it: such an approval was sent at once, so that its body would not be kept on disk while it waited. The reason was weaker than it read. gh-desk already keeps the conversations with Claude about a pull request on disk (ADR-031), drafts of reviews among them, in the same state directory, which only the user can read; and what gh-desk keeps out of its log (ADR-003) is a rule about the log, not about the state directory.

ADR-043 had decided that no review is recorded for the next launch, partly so that the record would not hold a body on disk, and that the runner holds a review's body in memory only. ADR-069 began recording approvals that wait without naming those sentences of ADR-043.

Against that, an approval written with `R`, a short "LGTM" or a review drafted with Claude, is as often one the user wants sent once CI passes as an approval from `A`. Only the approvals without a body waiting was a difference the confirm popup did not explain.

## Decision

This replaces, in ADR-069, "An approval with a body is sent at once and never waits, so that its body is not kept on disk." and the rejected alternative "recording the body to let an approval with a body wait". It replaces, in ADR-043, "A review is not recorded for the next launch." and "The runner holds a review's body in memory only." for an approval that waits, with or without a body; a comment, a request for changes, and an approval sent at once are still not recorded.

- An approval with a body waits for the jobs as one without does, and the key of the confirm popup that sends approvals at once covers it too.
- While it waits, its body is kept in the record of pending merges, in the state directory that only the user can read, and is removed with the record when the approval is sent or stops. It goes on at the next launch with its body.
- The body is never written to the log, as ADR-003 says.
- A comment and a request for changes are sent at once; they answer the code, not the checks.

## Consequences

- The record of pending merges can hold review text, and must stay readable by the user only.
- Whatever reads or reports the record must leave the body out of the log.
