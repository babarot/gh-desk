# ADR-105: The waits of a repository share a light read

## Status

Accepted

## Context

A waiting job reads its pull request at every poll with the full read: its checks, whether they are required, its changed files, and a request for the workflow runs of its head (ADR-068). That costs little while one job of a repository waits, but merges and approvals that wait side by side each pay it, for an answer that stays the same while their jobs run.

GitHub's rollup of a commit's checks comes with the light read, which covers many pull requests in one query. It tells when jobs end, except where something else keeps it pending: a commit status that waits for an approval stays pending after the jobs end, and so does a job that waits for someone to approve its environment.

## Decision

- The waits of a repository share one light read. It decides only whom to wake: what decides a merge or an approval is the full read of the job woken, as ADR-068 says.
- A pull request is left asleep only while what had not finished at its last full read was all jobs at work, and the light read shows it open, on the head the job acts on, with its rollup pending.
- A wait the light read cannot tell about reads as it did, without the light read.
- A failed light read wakes all it covered: each job then handles the failure as it would its own.
- Every sleeping pull request is woken after a time that is a constant, and at the end of its own wait for jobs, whatever the light read says. The constant is not a setting: it bounds how stale the names of the jobs on the row get, not anything about a repository.
- Rejected: one full read of all the waits. Its query grows with the checks and files of each pull request, and the workflow runs stay a request each.
- Rejected: reading only the waits whose jobs may have ended, from the time their last job took. Nothing says how long a job takes.

## Consequences

- A read that wakes a job must never be taken as a decision: a new use of the light read must stay on this side.
- While a pull request sleeps, the jobs named on its row may be out of date, by at most the constant above.
- The light read must keep the rollup, the state and the head.
- [reads_test.go](../../internal/runner/reads_test.go) checks that the reads are shared, what wakes a wait, and which waits are not left to the light read.
