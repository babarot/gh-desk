# ADR-090: Clawd plays a scene picked at random from themes

## Status

Accepted

## Context

ADR-089 let Clawd, Claude's mascot in the welcome of the claude pane, move while the worktree is made, with one motion. Whether the worktree was made or failed, Clawd then stood still the same way, though the line beside it says which.

Claude Code animates its own Clawd with named motions (jumping, peeking, winking and more) at a frame every 60 ms, and draws it in a newer shape than the one gh-desk copied, with its eyes in black. Motions of gh-desk's own were tried in mocks, among them Clawd hammering while the worktree is made and waving a flag when it is ready, which need cells beside Clawd.

Picking each part of what Clawd does at random, on its own, gives thousands of combinations, many of which do not hold together: a hammer that vanishes when the worktree is ready, a flag held that was never raised.

## Decision

- What Clawd does is a scene: a motion as the pane opens, one repeated while the worktree is made, one when it is made and one when it fails, and the frame Clawd then stays in, one for each end. A part may be nothing.
- Scenes come from themes, registered in [clawd.go](../../internal/tui/clawd.go). A theme gives the candidates for each part, and the scene picks one of each. When the worktree begins to be made, a theme is picked at random, never the one picked last, and the scene is fixed until the worktree is made again. Picking parts of different themes freely was rejected: most combinations do not hold together, and too many to check.
- A thing Clawd holds does not come or go as a scene comes to rest: an end that ends holding something rests holding it. A scene that holds anything takes the cells for it beside Clawd from its start, so the text beside Clawd does not move while it is shown. Widening only while something is held was rejected: the pull request beside Clawd would jump left and right. [clawd_test.go](../../internal/tui/clawd_test.go) checks every scene the themes can make for both.
- The end of the worktree shows in Clawd once the motion going on comes to an end, the opening or a round of the motion while it is made; the line beside Clawd tells it at once. Cutting the motion short was rejected: it looks broken, and tells nothing the line does not.
- Clawd is drawn as Claude Code draws it, its eyes in black on every theme. The black is Claude's, as the orange is, and is written in [themes.go](../../internal/tui/themes.go) beside it. What Clawd holds takes its colors from the theme shown (ADR-019).
- While Clawd moves, the screen is drawn at the pace of Claude Code's frames, and only then; no frame is shorter.
- When the spinner chosen does not move, Clawd does not move either: it stands while the worktree is made, and stands in its rest once it ended. Whoever turns the spinner off has asked for a screen that does not move.

This replaces ADR-089 whole, and in ADR-081 the sentence "It is the only color on the screen that does not come from the theme": Clawd's black is the second.

## Consequences

- A new theme must hold together by itself: a thing held is held from where it is first raised to the rest. The test catches a thing that comes or goes at the rest, and a line of a scene wider or narrower than the scene; it does not catch a motion that looks wrong.
- A theme whose scenes hold something takes more width for Clawd every time it is picked, also in the frames that hold nothing.
- The motions copied from Claude Code are copies; a change to them in Claude Code is a reason to look at them again, and gh-desk does not follow it on its own (ADR-081).
- The frames of a motion are fixed lengths written with the motion; the pace the screen is drawn at while Clawd moves follows the shortest of them.

## Not decided here

- Whether a theme can be chosen, or the scenes turned off, in the config file. There is no setting; the spinner setting stops them.
- Scenes for other moments, such as while Claude answers.
