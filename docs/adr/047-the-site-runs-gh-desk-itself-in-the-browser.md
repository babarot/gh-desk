# ADR-047: The site runs gh-desk itself in the browser

## Status

Accepted

## Context

gh-desk needs a place where it can be tried without installing it, a gh login, or pull requests of one's own: a playground, hosted on GitHub Pages. The recording of ADR-045 does not do this. It is a showcase for the README and for posts, and follows a script; a visitor to a playground goes anywhere, opens any pull request, and asks Claude anything.

gomi's site draws its TUI again in JavaScript and checks the drawing against its golden views. gh-desk's TUI is many times larger, with the runner, Claude and the config file behind it; drawn again, it would fall behind the real one with each change, and a check could compare only the screens the golden views cover.

Everything the TUI needs compiles to WebAssembly, except two dependencies with no files for js: Bubble Tea (raw mode, the resize signal, suspending) and atotto/clipboard, which the text boxes of Bubbles import.

## Decision

- The site runs gh-desk itself, compiled to WebAssembly, in a terminal drawn by xterm.js. The TUI, the runner and the settings of the config file a first launch writes are the real ones, put together as for the recordings (ADR-045); only GitHub and Claude are made up.
- The playground has a made-up world of its own, [internal/demo/playground](../../internal/demo/playground), apart from the recordings' [internal/demo](../../internal/demo). The two have different jobs: the recordings need what their script shows to stay the same, and the playground needs every pull request to have its diff and Claude's answers, and the triage and the merges to work through to the end. Kept apart, a change made for one does not move the other. Both are made-up data, as ADR-045 asks, and neither is built into gh-desk.
- gh-desk in the page sends nothing anywhere; the page itself loads only its fonts and xterm.js from outside.
- The program that runs in the browser, the files added to the two dependencies, and the page are in [site](../../site), a Go module of its own. Its build copies Bubble Tea and atotto/clipboard out of the module cache with one file each added, and builds through a go.mod of its own, so that no build of gh-desk, and neither go.mod, takes the patched copies.
- Rejected: drawing the TUI again in JavaScript, as gomi does, for the reason above. Rejected: one made-up world for the recordings and the playground, for the reason above. Rejected: a fork of Bubble Tea in the main module, which would put the browser's needs on every build of gh-desk.

## Consequences

- A method added to the TUI's `Source` or `Claude`, or to the runner's `GitHub`, needs one in both made-up worlds. The playground is in the main module, so `go test ./...` fails to build until it has one; the site's own build is checked by `mise run site:test` in CI.
- An update of Bubble Tea may need the file added to it changed: it fills in functions Bubble Tea leaves to each platform, which are not its API.
- What the playground shows of Claude is written in advance. A feature that asks Claude something new needs an answer in the playground, or it answers with its default.
- Features that need the machine (opening the browser, `$EDITOR`, Claude Code in the terminal, the files kept between launches) fail or stay off in the playground.
