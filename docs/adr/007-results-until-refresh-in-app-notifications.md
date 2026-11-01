# ADR-007: Results stay until refresh; in-app notifications instead of desktop ones

## Status

Accepted

## Context

A batch of merges can take minutes (checks, approval bots) while the user looks at another tab or another window. Showing results for a few seconds, or removing merged pull requests from the list as soon as they merge, means a result the user did not see is lost. Growing a column for progress shifts every other column while it runs.

Desktop notifications would reach the user outside the terminal, but they depend on the terminal and the operating system, and a dismissed one is gone as well.

## Decision

- While a pull request is in a batch, its step is written over cells of its row, in one cell spanning them, so no column changes width: WHY and SIZE, or REVIEW and SIZE where there is no WHY, when the row has them next to each other, or WHY (REVIEW) alone when there is no SIZE. With other columns (ADR-017), it is written over every column right of TITLE, and when they leave less than 23 columns (the width of WHY and SIZE), the rest is taken from the end of TITLE, so a row shows its step whatever the columns are. The header and the other rows keep their columns.
- When it ends (approved, merged, on auto-merge, skipped, stopped), the result stays on its row; a merged pull request stays listed with its title dimmed. Nothing is put away by time or by the automatic refresh. `R` puts the results away: rows show WHY and SIZE again and merged pull requests leave the list; steps still running stay.
- The header counts the results on rows (`✓5 ⧗1 ◌2 –2 R to clear`) in place of the tab's counts.
- Events that may be missed are kept as in-app notifications for the session: a batch's end with each pull request's result and every skip with its reason, each pull request put on auto-merge that GitHub merges or that stops, and at launch the config file's warnings, which do not stop gh-triage, each with where it is in the file. A toast shows the newest for four seconds; the header shows how many are unseen (`✦3`); `n` lists them, newest first, and enter jumps to the pull request (switching tab, unfolding its group, clearing the filter); on a warning, which names none, it closes the list.
- No desktop notifications.

## Consequences

- A result can always be found again: on its row until `R`, in the notifications until gh-triage exits.
- `R` has two meanings, reading the tab again and putting results away, which match: after reading, the results on rows are no longer news.
- Notifications are not kept across runs.
