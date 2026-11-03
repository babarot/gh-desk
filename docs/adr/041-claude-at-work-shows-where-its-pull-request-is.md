# ADR-041: Claude at work shows where its pull request is

## Status

Accepted

## Context

While Claude answered about a pull request whose conversation was not shown, the status line said so, as work in progress (ADR-025): one line, naming the first pull request and counting the others. The row of that pull request, often in sight, said nothing until the answer came and marked it. With questions asked about several pull requests, the user had to read the status line to know which were still answered, and it named only one.

## Decision

- While Claude answers about a pull request, its row tells: Claude's own spinner turns in the row's mark, and a light runs over the pull request's name. When the answer comes, the mark is the one an answer not yet seen has. A selection's mark is kept over the spinner, since what is selected is what the next action takes; the light still shows that Claude answers.
- A folded group tells what its rows would: the light runs over its name and the spinner turns after its count while Claude answers about one of them, and an answer not yet seen is marked there.
- The status line says that Claude is answering only about a pull request that nothing else on the screen shows: neither its conversation, nor its row in the part of the list drawn, nor the head of its folded group there. This follows ADR-025, which has the status line say that work ended only when nothing else on the screen does.
- Claude's spinner is its own, a constant that the config file does not choose: Claude at work is not something being read, and it ends in the mark of an answer. Its frames follow the rule of ADR-030 on width, and it does not move when the spinner chosen does not.
- This replaces, in ADR-030, "There is one spinner for the whole screen" (Claude's work has a spinner of its own besides it), "A spinner per place ... was rejected" as far as Claude's work goes, and "only there: cells and counts have no text" (the light also runs over a pull request's name and a group's name while Claude answers about it; `theme.shimmer` still turns it off).
- Rejected: naming the pull requests in turn on the status line. Work in progress shows every piece at once (ADR-025), and a third moving thing on the screen would draw the eye from the rows.
- Rejected: saying it on the status line as well, whatever the screen shows. It repeats the rows, and moves with them.
- Not done: a mark on the header's tab of a pull request in another tab. The status line says it.

## Consequences

- What the status line says of Claude depends on what the list draws: which tab, the filter, which groups are folded and how far the list is scrolled. The judgment reads the same state that drawing the list does, after the list is drawn.
- The mark of a row is shared by the selection, Claude's spinner, an answer not seen and an unread notification; a new mark must take its place among them.
- While Claude answers, the screen is drawn at least as often as its spinner turns.
