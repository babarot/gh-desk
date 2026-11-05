# ADR-068: A merge waits for the jobs at work on the head, required or not

## Status

Accepted

## Context

ADR-006 made a merge wait for the required checks only: whether a check is required came from `isRequired`, a merge went as soon as GitHub said it could be merged, and a check that was not required and failed was only a note in the confirm popup. In a repository with no required check, GitHub says a pull request can be merged while its tests still run, and gh-desk merged it then: gh-desk#47 was merged four seconds after it was marked ready, while two of its jobs were still at work. The popup had said "wait for checks, then merge".

Waiting for the required checks has two more gaps, found while designing an approval that waits for CI:

- A required check that has not reported yet is not in the checks GitHub lists for a commit: the list holds only check runs and commit statuses that exist. Before it reports, nothing tells it is to come.
- GitHub Actions makes a job that needs another only when that one ends; until then it has no check run either. Only its workflow run, which is not completed until every job in it is, says that more is to come. A workflow run itself is made some seconds after the push, the opening or the ready for review that starts it.

And some required checks wait for the approval itself, such as a policy bot's commit status. Waiting for them before approving waits forever; ADR-006 left them to `checks.approval`.

Approving is part of a merge (ADR-006: approve if needed, then merge as soon as the approval is in). An approval cannot be taken back, so an approval sent before the jobs end stays on a pull request whose job then fails.

## Decision

These replace, in ADR-006, "Whether a check is required comes from `isRequired`" for what a merge waits for and what stops it; "required checks still running (`merge.checks_timeout`)"; "Auto-merge is used where the repository allows it" while jobs are at work; and "approve if needed, then merge as soon as that approval is in".

- Before a merge, gh-desk waits for the jobs at work on the head: check runs not completed and workflow runs not completed, whether required or not. Commit statuses are not jobs and are not waited for as such; GitHub's own blocking still waits for the required ones, and the admin merge, which goes past GitHub's blocking, still waits for the required checks and stops on a failed one, as ADR-048 says.
- A job that ended without passing, required or not, stops the merge. A job waiting for someone to approve its environment is not waited for: it would not end while gh-desk waits.
- After something that may start workflows (the user's yes when the head has no workflow run yet, gh-desk marking the pull request ready, gh-desk updating its branch), gh-desk waits a quiet time before taking what it sees as all there is. After gh-desk's own writes the quiet time holds even when the head already has runs, since workflows started by ready for review come on top of them. Its length is a constant: it follows how long GitHub takes to make a workflow run, not anything a user chooses.
- When a merge has to approve, it approves only once the jobs ended and none failed.
- While jobs are at work, auto-merge is not turned on: GitHub's auto-merge waits for the required checks only. A pull request the popup did not tell goes to auto-merge is merged by gh-desk itself.
- How long a merge waits for jobs is the setting `checks.wait_timeout`, which replaces `merge.checks_timeout`; the old name is kept in the table of moved settings (ADR-016). `merge.blocked_timeout` stays: it bounds a later wait, for rules other than checks.
- Workflow runs are read only when deciding a merge. When they cannot be read (Actions turned off, or a token that may not read them), the check runs alone tell, and the gap between a job and the one that needs it stays open in that repository.
- [`internal/plan/jobs.go`](../../internal/plan/jobs.go) decides what is at work and what failed; [`internal/plan/jobs_test.go`](../../internal/plan/jobs_test.go) and [`internal/runner/jobs_test.go`](../../internal/runner/jobs_test.go) hold the cases above.
- Rejected: waiting for the required checks only, as before. It merges in a repository with no required check, and does not see a required check that has not reported.
- Rejected: telling the checks that wait for an approval from their kind or their words, without `checks.approval`, as a basis for approving. A CI that reports commit statuses would be approved before it ends; the error goes the unsafe way.
- Rejected: reading the required checks from the branch's rules to see those not reported yet. It needs another read and permissions the token may not have, and the workflow runs cover the same gap for GitHub Actions, which is what the repositories checked use.

## Consequences

- A merge in a repository with long jobs that are not required waits for them, up to `checks.wait_timeout`, where it used to merge at once.
- A CI that reports commit statuses rather than check runs is not waited for unless it is required and GitHub blocks on it; a setting for such statuses would be a later decision.
- A workflow started by another workflow's end (`workflow_run`) is not seen until it is made.
- Every decision that may merge reads the workflow runs of the head too: one more request per pull request.
