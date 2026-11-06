# ADR-081: Claude's mascot keeps Claude's orange on every theme

## Status

Accepted
Partly replaced by ADR-090

## Context

The claude pane with nothing asked showed only the text box, under an empty pane. It now shows a welcome: Claude's mascot beside the pull request, the worktree being made, and questions to start with, as Claude Code greets a session.

ADR-019 makes every color on the screen come from the theme shown, so that nothing stays the same color across themes. The mascot is the one thing on the screen that is not gh-desk's: it is Claude's, and users know it in Claude's orange from Claude Code. Drawn in a theme's peach or red, it is a different creature on each theme, and on some themes it is not orange at all.

## Decision

- Claude's mascot is drawn in Claude's orange, one fixed color, on every theme. It is the only color on the screen that does not come from the theme.
- The color is still written in [themes.go](../../internal/tui/themes.go), the one place colors are written (ADR-019), apart from the palettes, so that [colors_test.go](../../colors_test.go) keeps failing on a color written anywhere else. A test in `internal/tui` checks that every theme draws the mascot in it.
- Nothing else of the welcome takes the fixed color: the pull request, the worktree and the questions are drawn from the theme as the rest of the pane is. A second color outside the themes needs a decision of its own.
- The welcome says nothing that the key line says (ADR-024): it lists the questions, and the key that puts them in the text box is told by the key line.

## Consequences

- A new theme does not give the mascot a color; on a theme whose background is close to the orange the mascot is hard to see, and the theme is the one to change.
- A change to the mascot or the orange in Claude Code is a reason to look at the welcome again; gh-desk does not follow it on its own (as ADR-032 says of the chat).

## Not decided here

- Whether the welcome can be turned off. It has no setting; it takes only the lines the pane has free, and gives way when they are few.
