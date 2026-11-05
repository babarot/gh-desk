# ADR-069: An approval can wait for the jobs at work, and is recorded while it waits

## Status

Accepted
Partly replaced by ADR-070

## Context

An approval was one write sent as soon as the user said yes; only a required check that had failed kept a pull request out of it. A reviewer who wants to approve only once CI has passed had to watch the pull request and come back. ADR-068 made a merge wait for the jobs at work on the head, check runs and workflow runs, required or not, and it already approves only after them. An approval on its own had nothing of the kind.

ADR-009 records what a merge waits for, so that it goes on at the next launch, and decided "Approve batches are not recorded: an approval is a single write that ends at once." An approval that waits no longer ends at once.

An approval may carry a body. Recording a waiting approval would keep that body on disk until it is sent, and gh-desk keeps review bodies out of its log for the same reason (ADR-003).

## Decision

This replaces, in ADR-009, "Approve batches are not recorded: an approval is a single write that ends at once." for an approval that waits.

- The confirm popup of an approval without a body waits for the jobs by default, when some of the pull requests have jobs at work, and a key turns it off to approve at once. What counts as a job at work, the quiet time after the user's yes, and a job that failed are as ADR-068 says.
- An approval with a body is sent at once and never waits, so that its body is not kept on disk.
- An approval that waits is approved on the head the user confirmed, once no job is at work; a head that moved stops it, as every write does (ADR-006). A job that failed stops it, unless the user let in pull requests whose checks failed, which then also covers jobs that fail while it waits.
- It waits aside from the other pull requests of its repository, as auto-merge is watched, since approvals do not knock each other out of date as merges do.
- It is recorded while it waits, with whether failing ones were let in, and goes on at the next launch under the lock, as a merge does. A gh-desk without the lock skips approvals that would wait. Its wait is bounded by `checks.wait_timeout`, as a merge's is.
- Rejected: always waiting, with no key. An approval the user wants sent now, a dependency update reviewed while slow optional jobs run, would have to wait too.
- Rejected: recording the body to let an approval with a body wait. It would put review text on disk; writing the text after the checks pass costs the user less.

## Consequences

- The record of pending merges holds approvals too, and whatever reads it tells them apart.
- An approval batch can stay open long after the popup closed, as a merge batch on auto-merge does.
- A new kind of review that should wait would need the same care for its text.
