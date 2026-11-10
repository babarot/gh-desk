# ADR-116: URLs are copied through the terminal

## Status

Accepted

## Context

A pull request's URL is often wanted somewhere else: a chat, an issue, a note. The browser key opens it, and copying it from there takes several more steps; with a selection it takes as many as there are pull requests. babarot/gist, a TUI of the same author, copies the URL of the gist under the cursor, or of each selected one a line each, and gh-desk should do the same so that the two read alike.

A program can put text on the clipboard in two ways. It can run the platform's tool (`pbcopy`, `wl-copy`, `xclip`, `clip.exe`), which needs `os/exec` in `internal/tui` and a choice of tool per platform, and which reaches the clipboard of the machine gh-desk runs on, not of the one the user sits at over SSH. Or it can write the OSC 52 escape sequence and let the terminal set its clipboard, which Bubble Tea does without running anything. A terminal or multiplexer that does not pass OSC 52 on drops it, and the program cannot tell.

## Decision

- One key copies URLs: of the pull request under the cursor or, with a selection, of every selected pull request, a line each in the order of the list. It acts on what the menu acts on.
- The URLs go to the clipboard through the terminal, by OSC 52. Nothing is run.
- Copying changes nothing, on GitHub or in gh-desk: there is no confirm, and the selection stays so that the next key can act on it too.
- Rejected for now: running the platform's clipboard tool. It brings `os/exec` into `internal/tui` and a tool per platform, and misses the user's clipboard over SSH. It is to be taken up again only if a terminal the user works in does not pass OSC 52 on.

## Consequences

- The status line says what was copied, but not that it arrived: where OSC 52 is dropped, the copy fails without a word. A user whose terminal or tmux does not pass it on has to turn it on there.
- A later copy of something else (a diff, a body) goes the same way, through the terminal, and does not bring in a subprocess for it alone.
- The behavior is checked by [selection_test.go](../../internal/tui/selection_test.go).
