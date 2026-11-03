# ADR-039: A review is offered in the text box, not by an action of its own

## Status

Accepted

## Context

ADR-036 made a review a question asked by an action of its own (`chat_review`). The action's name and its place among the actions that write to GitHub (a drafted review, approve) did not tell whether it asked Claude a question or posted a review on GitHub. Opening the claude pane (ADR-038) also took a second key before anything could be typed.

## Decision

- Opening the claude pane puts the focus in its text box.
- The text box offers the review question, until it has been asked in the conversation. A key (`fill_suggestion`) fills it in, only into an empty box, and the user sends it as any question. What is sent is what the box shows, so it is plainly a question to Claude.
- There is no action that asks for a review without the text box. This replaces, in ADR-036, "asked by an action of its own (`chat_review`)": a review is a question like any other, offered by the text box, and it can still be asked at any point by typing it.
- Rejected: keeping `chat_review` beside the suggestion. Two ways to ask the same question, one of them unclear, is what this decision removes.
- Rejected: sending the suggestion when enter is pressed in an empty box. An empty box would then send a costly question by accident.

## Consequences

- A `[keys]` entry for `chat_review` is now an unknown action, reported as a mistake in the config file (ADR-004). It is not a rename, so it does not name another action.
- A review takes two keys instead of one, and after the first review the suggestion is gone; asking for another review means typing it.
