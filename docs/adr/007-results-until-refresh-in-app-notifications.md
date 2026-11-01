# ADR-007: Results stay until refresh; in-app notifications instead of desktop ones

## Status

Accepted

## Context

A batch of merges can take minutes (checks, approval bots) while the user looks at another tab or another window. Showing results for a few seconds, or removing merged pull requests from the list as soon as they merge, means a result the user did not see is lost. Growing a column for progress shifts every other column while it runs.

Desktop notifications would reach the user outside the terminal, but they depend on the terminal and the operating system, and a dismissed one is gone as well.

## Decision

- While a pull request is in a batch, its step is written over cells of its row, whatever the columns are (ADR-017), so no column changes width. The header and the other rows keep their columns.
- When it ends (approved, merged, on auto-merge, skipped, stopped), the result stays on its row; a merged pull request stays listed, dimmed. Nothing is put away by time or by the automatic refresh. `R` puts the results away and merged pull requests leave the list; steps still running stay.
- The header counts the results on rows in place of the tab's counts.
- Events that may be missed are kept as in-app notifications for the session: a batch's end with each pull request's result, every skip with its reason, each pull request put on auto-merge that GitHub merges or that stops, and at launch the config file's warnings, which do not stop gh-triage. A toast shows the newest for a moment, the header counts the unseen ones, and the list of them leads to the pull request each one names.
- No desktop notifications.

## Consequences

- A result can always be found again: on its row until `R`, in the notifications until gh-triage exits.
- `R` has two meanings, reading the tab again and putting results away, which match: after reading, the results on rows are no longer news.
- Notifications are not kept across runs.
