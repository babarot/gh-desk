# ADR-061: The progress pane keeps a line for each pull request

## Status

Accepted

## Context

The log viewer (ADR-060) shows the log as it is written, which tells what happened, one line at a time. Following a batch also asks where each pull request stands now: which steps it has been through, which it is on and for how long, and which are expected ahead. The runner says this in its events (ADR-059), and the TUI keeps each pull request's steps from them.

The log viewer is opened to be looked at for a moment and closed. A batch can end while it is open, and what the user was watching should not vanish under them.

## Decision

- While the log viewer is open, a progress pane above the log shows the batches that were running when it opened and those started since.
- A batch that ends while the viewer is open stays in the pane, shown as done, until the viewer closes. A batch that ended before the viewer opened is not shown.
- A pull request takes exactly one line in the pane, whatever it waits for; what does not fit the width is cut, not wrapped, so the pane's height depends on the batch only and the log under it does not move. A batch longer than the room says how many more it has.
- The pane is built from the runner's step events and forecasts, never from the log; what is ahead is drawn as the runner's latest forecast, and may change.
- The status line counts how a running batch stands besides how many are done, built from the same state (ADR-025).
- Rejected: wrapping a pull request's line when what it waits for is long. The pane would change height as waits begin and end, and the log under it would jump.
- Rejected: removing a batch from the pane as soon as it ends. The user was watching it.
- Rejected: opening the log viewer when a batch starts. It is to be opened when wanted; the status line tells that a batch runs.

## Consequences

- What a step is called in the pane is the TUI's; a new step in the runner's set needs a word there too.
- A batch's lines are as many as its pull requests; a long batch on a short screen shows only its first ones.
