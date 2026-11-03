# ADR-047: The site runs gh-desk itself in the browser

## Status

Accepted

## Context

gh-desk needs a page where it can be tried without installing it, a gh login, or pull requests of one's own, hosted on GitHub Pages. gomi's site does this by drawing its TUI again in JavaScript, and checks the drawing against its golden views in CI. gh-desk's TUI is many times larger, and has the runner, Claude and the config file behind it; drawn again, it would fall behind the real one with each change, and a test could only compare the screens the golden views cover.

The TUI already learns about GitHub and Claude only through the interfaces it defines (Source, Runner, Claude), and the runner through its own GitHub interface. Everything else it needs compiles to WebAssembly, except two dependencies with no files for js: Bubble Tea (raw mode, the resize signal, suspending) and atotto/clipboard, which the text boxes of Bubbles import.

## Decision

- The site runs gh-desk itself, compiled to WebAssembly, in a terminal drawn by xterm.js. The TUI, the runner and the settings are the real ones; only the outside world is replaced.
- The outside world is made up, in `site/demo`: a GitHub that keeps its pull requests in memory and changes them when the runner and the TUI write, and a Claude that answers from a script. Every repository, person and pull request in it is invented, as the rule on test data asks. gh-desk in the page sends nothing anywhere; the page itself loads only its fonts and xterm.js from outside.
- The demo runs with the config file written on first run, as a new user sees it; `app.Settings` turns it into the TUI's settings, read without a disk through `config.Parse`.
- The site is a Go module of its own, `site/`, so that the main module's dependencies, `go test ./...` and the layer table of ADR-002 stay as they are. It imports the internal packages, which Go allows from a path under the module.
- Bubble Tea and atotto/clipboard get one file each for js, kept in `site/patch`. The build copies the two modules out of the module cache with the file added and builds through a go.mod of its own; `go.mod` and `site/go.mod` are not changed by it.
- Rejected: drawing the TUI again in JavaScript, as gomi does, for the reason above. Rejected: a fork of Bubble Tea in the main module, which would put the browser's needs on every build of gh-desk.

## Consequences

- A change to an interface the TUI or the runner defines, or to `app.Settings`, has to be followed in `site/demo`. CI builds the site and runs its tests on every pull request, so a change that breaks it shows there.
- An update of Bubble Tea may need the file in `site/patch` changed: it fills in functions Bubble Tea leaves to each platform, which are not its API.
- What the demo shows of Claude is written in advance. A feature that asks Claude something new needs an answer in the script, or the demo answers with its default.
- Features that need the machine (opening the browser, `$EDITOR`, Claude Code in the terminal, the files kept between launches) fail or stay off in the demo.
