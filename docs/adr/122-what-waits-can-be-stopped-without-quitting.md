# ADR-122: What waits can be stopped without quitting

## Status

Accepted

## Context

A merge or an approval that waits (ADR-009, ADR-068, ADR-069) could only be ended by quitting gh-desk, which does not end it either: the record of pending merges keeps it, and the next launch goes on with it. A user who confirmed a merge and then changed their mind, or confirmed a selection with one pull request too many, had to wait for it to end on its own, or close it on GitHub.

Every wait of a job already ends with the batch's context, which only quitting cancels. Reads and writes to GitHub take the same context, so canceling it in the middle of a write leaves it unknown whether the write reached GitHub; quitting keeps the record, and the next launch reads the pull request to find out. A stop that forgets the record has no next launch to find out at.

A pull request handed to GitHub's auto-merge or to a merge queue is not waited for by gh-desk: GitHub merges it, and gh-desk only watches. Ending the watch does not stop the merge.

## Decision

- The user can stop the jobs on chosen pull requests while gh-desk runs. A stop ends the job's waits as quitting does, and also forgets its record, so the next launch does not go on with it. Quitting is unchanged: it keeps the record.
- A stop takes effect at the job's waits only. A write already on its way is sent to the end, and no write starts once the stop is seen; the job checks before each write. What was written before stays on GitHub (an approval, a draft marked ready, an updated branch), and the pull request's result says so as it does for any stop.
- The record is forgotten when the stop is asked, under the same lock as the job's writing of it, so a record forgotten is never written back. Quitting right after a stop does not bring the job back.
- A job whose read or write fails after a stop ends without its record. Its result says it was stopped, and, for a write, that whether it reached GitHub is not known. Nothing goes on with it to find out, since a stop allows no further write.
- A pull request handed to auto-merge or a merge queue is not stopped. The hand-over, the record of it being watched and the end of the stop's reach happen under the stop's lock, so a stop that comes while the write handing it over is on its way does not end the watch, nor lose the record of it: the next launch watches it again. Turning auto-merge off or taking a pull request out of a merge queue is GitHub's state shared with others, possibly set by someone else, and is left to an action of its own if one is ever made (ADR-108 leaves taking one out of the queue open).
- A stop writes nothing to GitHub, so it has no confirm popup. It spends the selection as a confirmed action does (ADR-094).
- A pull request the user stopped is not kept to try again. This replaces, in ADR-094, "What failed or stopped never stays in, or comes back to, the selection. It is kept apart, with the action and its text, and a key opens that action again for those pull requests alone." with: what failed, or stopped on its own, is kept apart as that sentence says; what the user stopped is not, since stopping is a decision, as a skip is, and mixing the two would put what the user stopped beside what needs another try.
- Rejected: canceling the context of reads and writes too, so a stop takes effect at once. A write cut short is not known to have reached GitHub, and with the record forgotten nothing finds out.
- Rejected: a stop that also ends the watch of auto-merge or a merge queue. GitHub still merges, so the pull request would be merged after the user saw it stopped, and a merge it is the base of would no longer wait for it.
- Rejected: a stop that turns auto-merge off or takes the pull request out of the queue. It makes a stop a write to GitHub, on a state another person may have set.

## Consequences

- Every new write a job makes must check for a stop first, and every new wait must end with the stop's context, not the batch's. The watch of auto-merge and of a merge queue must keep the batch's context.
- The result of a job tells a stop by the user from one stopped by what it read, and what reads results (the notification of a batch's end, what is kept to try again) must keep them apart.
- A merge stopped while it holds its repository's lane gives it up, and a merge based on it goes on, as ADR-104 has it for any end.
- [stop_test.go](../../internal/runner/stop_test.go) checks that a stop writes nothing new, lets a write on its way finish, forgets the record, leaves a watch alone, and keeps the record of one handed over while the stop came.
