# ADR-123: The clock reads every tab

## Status

Accepted

## Context

gh-desk is left open in a pane out of sight and looked at now and then. The automatic refresh read the shown tab and the inbox only. A tab not shown was read again when it was opened after more than an interval, and it kept its old rows until that reading was in, several seconds with a few pages. A pull request merged on GitHub ten minutes before was still listed when the user came back and opened its tab, and left while they moved the cursor, so the list looked as if it were drawn only on a key.

## Decision

- The automatic refresh reads every tab, the inbox among them, so that a tab opened is as fresh as the one shown, give or take an interval.
- `r` still reads the shown tab and the inbox: it is asked for what the user looks at, and puts away its results.
- Opening a tab not read for longer than an interval still reads it, for a reading the clock skipped.
- Rejected: reading only the shown tab and reading a tab again on opening it, as before. The rows shown first are the stale ones, and the user acts on them before the reading is in.

## Consequences

- Every automatic refresh costs a search per tab: a tab added to the config adds to what each interval reads from GitHub, whether or not it is shown.
- A reading of a tab not shown changes its rows unseen; the cursor is put back on its pull request when the tab is opened (ADR-115).
- The guarantee is checked by [refresh_test.go](../../internal/tui/refresh_test.go).
