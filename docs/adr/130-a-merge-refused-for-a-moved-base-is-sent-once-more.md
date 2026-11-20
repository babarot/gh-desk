# ADR-130: A merge refused for a moved base is sent once more

## Status

Accepted

## Context

ADR-082 has the merges of a repository go one at a time, and each reads the pull request again before it writes (ADR-006). GitHub works out whether a pull request can be merged on its own time after its base moves: a read a few seconds after the merge ahead may still say it is clean, from the old base. A merge sent on that read is refused with "Base branch was modified. Review and try the merge again.", and the pull request ended as failed in the middle of a batch, though nothing about it was wrong. The user had to confirm it again.

## Decision

- A merge GitHub refuses because its base branch was modified is not the end of the pull request. gh-desk waits as long as it waits between reads while GitHub works out whether it can be merged, reads it again, decides again, and acts on that decision, holding the repository's lane. A merge decided again is sent once more.
- It is sent once more only once per turn in the lane. Refused again, the pull request fails as any refused merge does.
- What is sent again carries the confirmed head, as every write does (ADR-006): a head that moved meanwhile still stops it.
- The refusal is told apart in [internal/github](../../internal/github) by GitHub's message and passed to the runner as its own error, as the clean status of auto-merge is; any other refusal is not sent again.
- A stacked pull request, merged through the asynchronous merge API (ADR-129), is not concerned: it does not go through this refusal.
- Rejected: sending again until GitHub takes it, or for as long as a timeout. A refusal that repeats is not the race this answers, and a loop of writes on a refused merge is what a bound exists to stop.
- Rejected: waiting a fixed time after every merge before the next of the same repository. It costs every merge the wait to spare the few that race, and GitHub's time to work out a new base has no bound to pick it from.

## Consequences

- The guarantee is only that one refusal for a moved base does not end a batch's pull request. A busy base branch, moved again before the second send, still fails it.
- Telling the refusal apart rests on GitHub's message text, which GitHub may change; then the merge fails as it did before this decision.
- [runner_test.go](../../internal/runner/runner_test.go) checks that a merge refused once is merged, that one refused twice fails after two sends, and that another refusal is not sent again.
