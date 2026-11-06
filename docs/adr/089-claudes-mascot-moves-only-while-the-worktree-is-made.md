# ADR-089: Claude's mascot moves only while the worktree is made

## Status

Accepted

## Context

The welcome of the claude pane draws Clawd, Claude's mascot, in Claude's orange (ADR-081). It stood still, also while the worktree was made, when the only thing moving was the spinner beside "making the worktree". A mascot that moves along with the spinner tells, at a glance, that gh-desk is working, as Claude Code's own Clawd has poses.

The welcome is shown only until something is asked. While Claude answers, the pane shows the conversation and Claude's own spinner (ADR-041), and the welcome is gone.

## Decision

- Clawd moves only while the welcome is shown and its worktree is made. It stands still once the worktree is ready or failed.
- Clawd stands still when the spinner chosen does not move. Whoever turns the spinner off has asked for a screen that does not move, and the mascot is part of it.
- Clawd moves on the clock every spinner turns from, so it keeps in step with them, and no pose is drawn slower than the spinners' step lets the screen be drawn. A test in [welcome_test.go](../../internal/tui/welcome_test.go) checks this, and that every pose is as wide as the still mascot, so the text beside it does not move.
- How long a pose is kept is a constant, not a setting: it is a matter of how the welcome looks, which nothing else depends on.

## Alternatives

- Keeping Clawd on the screen while Claude answers, moving as Claude's spinner turns. Rejected: it needs a place for the mascot beside the conversation, and something moving next to an answer is read against. Claude's spinner already says that Claude is answering.
- A setting to turn the movement off. Rejected: the spinner setting already says whether the screen may move.

## Consequences

- A new spinner must step at least as often as Clawd changes poses, or the test fails: the screen would be drawn too seldom to show every pose.
- A new pose must keep the mascot's width on every line, on terminals that draw East Asian Ambiguous characters wide as well as narrow, as the still mascot does.
- The poses are gh-desk's own; a change to Clawd in Claude Code is a reason to look at them again, as ADR-081 says of the mascot.
