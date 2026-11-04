# ADR-059: The runner says its steps, and a forecast of those ahead made again at each decision

## Status

Accepted

## Context

A merge in a batch goes through steps one after another: marking a draft ready, approving, updating the branch, waiting, merging or turning on auto-merge. The runner told the TUI only a few words of what it was doing, which the TUI drew on the pull request's row. To show how far each pull request has come, which steps are behind it and which are ahead, the TUI would have had to read those words back, and words written for people change.

What is ahead is not known at the start. The runner decides a merge's next step from what it reads, and decides again after marking a draft ready and after approving, since both change what GitHub allows; while it waits it decides again at every read. A pull request blocked only for want of an approval is decided to wait at first and to merge at once after the approval; marking a draft ready can start checks that it then waits for. What a wait is for (checks, approval checks, another rule) can change while it waits.

## Decision

- The runner's events say which step a pull request begins or goes on with, as a value of a fixed set of steps, besides the words for the row. A wait's events say what the wait is for and name the checks it waits on.
- At each decision the runner sends a forecast of the steps it expects ahead. A forecast is a guess and is replaced whole by the next one; it is sent only when it changes. It says nothing else, and the TUI does not let it change a row.
- The steps a pull request has been through are what the step events said, in their order, never rebuilt from a forecast. A wait that changes what it waits on begins a new step.
- [`internal/runner`](../../internal/runner) holds the set of steps; the TUI reads them from the events and does not parse the words.
- Rejected: one plan sent at the start. It is wrong whenever a decision changes, which happens after every approval that unblocks a merge and every draft marked ready.
- Rejected: reading the steps from the words of the row. The words are for people and change without the code that reads them knowing.

## Consequences

- A new step a merge can take is added to the set of steps and said when it begins, or it does not show.
- Where the runner decides again, it sends the forecast again.
- What is ahead can shrink or grow while a pull request goes on, and whatever shows it must allow for that.
