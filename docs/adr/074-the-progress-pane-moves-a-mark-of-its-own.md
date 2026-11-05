# ADR-074: The progress pane moves a mark of its own

## Status

Accepted

## Context

The progress pane (ADR-061) shows a line for each pull request of a batch, with the steps it has been through, the one it is on and those ahead. It drew the step a lane is on as a still half circle among the dots of the others, and turned the spinner of `theme.spinner` twice more: in the title of the batch's box, and before the word of the step each lane is on. Three things said one fact, that the batch is at work, and two of them moved.

The lanes are drawn in two ways: a row of dots, and the steps side by side with their times. Which one was drawn depended on where the log viewer was: side by side only in the bottom pane's place, so a viewer over the list drew dots even where the steps side by side fitted.

Mocks compared moving the half circle itself in several ways. Alternating it with the filled circle drew, for a frame, a step not done as done, since the filled circle is the mark of a step done. Filling it and emptying it at once, back to the empty circle, read as a start over; going back through the half circle did not.

## Decision

- The step a lane is on is shown by a mark that moves; no spinner leads the word of that step.
- A way to draw lanes carries its own mark, and places it where its drawing puts the step a lane is on. The pane picks the way, never the mark: the ways are tried widest first, and a batch is drawn in the first whose lanes all fit the pane, wherever the viewer is. The ways and their marks are in [internal/tui/progress.go](../../internal/tui/progress.go).
- The title of a running batch's box turns a spinner of its own, the same whatever `theme.spinner` is. It looks like the marks of the lanes, which the spinner chosen need not.
- The spinner that does not move stops the title's spinner and the marks too: whoever chose it chose a screen that does not move.
- The marks and the title's spinner turn from the same clock as the other spinners. Their frames and pace are constants (ADR-014).
- This replaces, in ADR-030, "There is one spinner for the whole screen, chosen by name in `theme.spinner`" and "A spinner per place (one for cells, another before a line) was rejected" for the progress pane only: its title and its marks are not the spinner chosen. Everywhere else they hold.
- A mark may be an East Asian Ambiguous character, as the dots of a lane already are; ADR-030's width rule is for the spinners of `theme.spinner`, which are drawn in columns and before text all over the screen.
- Rejected: alternating the half circle with the filled circle. It draws a step not done as done.
- Rejected: one mark for every way to draw lanes. Among the dots a blinking half circle is enough to tell which step is on; side by side the mark stands alone before a word and is read better as filling and emptying.
- Rejected: a setting for the marks. They would need the width rule checked in the config file, which ADR-030 rejected for spinners.

## Consequences

- A new way to draw lanes is an entry in the list of ways, with its mark; the pane does not change.
- The screen must be drawn often enough for the marks and the title's spinner while the progress pane shows a running batch, whatever the pace of `theme.spinner` is.
- On a terminal that draws East Asian Ambiguous characters two cells wide, a lane can be cut short, as it could with the still dots.
