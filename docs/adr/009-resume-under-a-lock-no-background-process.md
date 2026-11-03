# ADR-009: No background process: pending merges resume at the next launch under a lock

## Status

Accepted
Partly replaced by ADR-021

## Context

Some merges wait: for required checks, for an approval check to pass after gh-desk approved, for GitHub's auto-merge to happen. These waits can outlast a terminal session (a laptop closed for the night). Three ways were open: keep waiting in a process that outlives the TUI, give up when the TUI quits, or record what is being waited for and go on at the next launch.

A process left running would be a second thing to install, stop and debug, with its own log and life cycle, for a tool that otherwise only runs while it is looked at. Giving up loses work the user confirmed.

Two gh-desk running at once (two terminals) must not both go on with the same record, or the same pull request is handled twice.

## Decision

- No background process. When gh-desk quits, waiting stops.
- What a merge batch is waiting for is recorded in `$XDG_STATE_HOME/gh-desk/pending.json` (`internal/state`): the pull request, its confirmed head, whether the user agreed to auto-merge, whether the user's approval was on it (sent by gh-desk or there when the merge started), whether it is only being watched on auto-merge, and how long it has waited for checks, for its rules, and on auto-merge. A pull request is recorded before gh-desk first writes to GitHub for it, updated at every read while it waits, and removed when it ends.
- The times waited are recorded, not the times waiting started, so the time gh-desk was not running does not count against `merge.checks_timeout`, `merge.blocked_timeout` and the time auto-merge is watched, whether it quit or crashed.
- At launch, the runner goes on with what is recorded, reading each pull request again first: a head that moved, a pull request closed meanwhile, or a recorded approval that was dismissed stops it and says so; one merged meanwhile is reported as merged.
- The record is guarded by `flock(2)` on `pending.lock`, which the operating system releases when its holder exits, however it exits. Only the holder reads the record and resumes. A gh-desk started while another holds the lock works without it: its confirm popup skips merges that would wait (checks, branch update, auto-merge), so nothing is waited for that would not go on after quitting.
- Approve batches are not recorded: an approval is a single write that ends at once.
- One pull request is handled by one job at a time, watching included; a second batch skips it, so no two jobs write the same record.

## Consequences

- Closing gh-desk pauses the merges it waits for; they go on when it is opened again, with their limits counted as if no time had passed.
- A second gh-desk is fully usable except for merges that would wait.
- The record is a small JSON file a user can read; deleting it while gh-desk is closed drops the pending merges.
