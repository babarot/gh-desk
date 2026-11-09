# ADR-109: The settings page runs gh-desk itself, against the lists last read

## Status

Accepted

## Context

The page `gh desk config edit --ui` opens shows gh-desk above the settings, drawn again in JavaScript (ADR-100) over the lists last read (ADR-102). A drawing does only what it was made to: it does not scroll, `a` does not show what a rule leaves out, and its popups are pictures. What a user wants from the page is to see what a rule does to their own pull requests, and that is what the drawing shows least.

The command is gh-desk itself. Bubble Tea runs on any reader and writer, as the playground runs it in WebAssembly (ADR-047), and the page can draw a terminal with xterm.js. ADR-100 rejected WebAssembly for the page because of the build step it adds before `go build`; running the TUI in the command's own process adds none.

The TUI acts in the viewer's name. Run on a page, it must not act at all: it reads from GitHub, writes to it, starts an editor, keeps the lists it read, and takes the lock on the record of pending merges.

## Decision

- The command's page shows gh-desk itself: the TUI runs in the command's process and the page draws it in a terminal. It runs on the lists last read, as they were read, and follows the file as it is edited on the page without starting again, so that what is open stays open.
- It reads nothing from GitHub and writes nothing. This is held by what the command gives it, not by what it is asked: the TUI runs offline ([internal/tui/offline.go](../../internal/tui/offline.go)), and what it reads from and acts through refuses every read and write ([internal/app/configterm.go](../../internal/app/configterm.go)), holding no client and no token. It starts no editor and keeps nothing: no lists, no layout, no conversation, no lock.
- What the lists last read do not hold is not read: the pane says so. A merge confirm decides on what the lists hold and takes what they do not (the methods a repository allows, whether a pull request can merge) as the most a merge may do, and says so on the confirm.
- Auto-approve does not run on the page: its rules search on their own, which the lists last read cannot answer. What they would approve is shown as ADR-102 counts it.
- The terminal is reached as the page's other routes are (ADR-101): a token in the URL and this host in `Host`, and for what the page sends, this origin in `Origin`. What gh-desk draws goes to the page as server-sent events; what is typed comes back one request after another. One page has one terminal; a new one ends the one before.
- xterm.js is kept in the repository and inlined in the page, which stays one HTML file with nothing from outside (ADR-100). Only the command's page has it.
- The site's settings page has no gh-desk behind it, and keeps the drawing of ADR-100 and ADR-103.
- Rejected: drawing the TUI again in JavaScript for the command's page. It falls behind what it draws, and shows least what the page is for.
- Rejected: WebAssembly for the command's page. It needs the build step ADR-100 rejected, and the command is already gh-desk.
- Rejected: answering the TUI's reads from the lists as if they were read now. It shows lists hours old as fresh, against ADR-102.
- Rejected: starting the TUI again on each change of the file. What is open, such as `?` or a merge confirm, would close on every edit.

This replaces, for the command's page only, two items of ADR-100's Decision: the one that begins "The page draws the list and its popups again in JavaScript", and "What is compared is the list, the menu of enter, the review box and the confirm of a merge." It replaces this sentence of ADR-102: "The page acts in nobody's: it only draws and counts, and it says whose lists they are." The page now runs gh-desk as the viewer of the lists, with every act refused.

## Consequences

- A read the TUI gains must be stopped when it runs offline, or the page shows it as failing; a write must go through what the command gives it, which refuses it.
- A setting the TUI takes only when it starts does not follow the page until the TUI takes new settings for it.
- The merge confirm on the page is as right as its assumption: a repository that allows fewer methods, or a pull request that cannot merge as it is, is not shown so.
- Pull request titles of private repositories go to the page as they did (ADR-102), now as what gh-desk draws.
- The page carries xterm.js; a new version of it is copied in by hand with its license.
