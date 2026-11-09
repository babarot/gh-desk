# ADR-100: The settings page draws the list again, and compares it with gh-desk's

## Status

Accepted
Partly replaced by ADR-102
Partly replaced by ADR-109

## Context

Settings such as the rules, the tabs and the keys are hard to write by hand, and harder still to picture. `gh desk config edit --ui` shows them on a page, with gh-desk drawn above them as they change, so the page needs something that draws gh-desk.

ADR-047 runs gh-desk itself in the browser, compiled to WebAssembly, and rejects drawing the TUI again in JavaScript because the TUI is too large and would fall behind. That is about a site that runs the whole TUI. The page needs the list and a few popups, and it has no keys of its own to answer. WebAssembly would have to be built before `go build`, by a step `go install` and a build from a release's source do not run, and that build patches Bubble Tea.

## Decision

- `gh desk config edit --ui` serves one HTML page on 127.0.0.1 for as long as it runs, and opens it. Nothing comes from outside: the code is embedded with `go:embed`, so no build step is added.
- Saving on the page checks the file with the same function gh-desk starts with, and shows what is wrong on the page. What happens to a file with no mistakes is ADR-101.
- A request must carry a random token from the URL and name this host in `Host`, and saving must come with this origin in `Origin`, so that a web site cannot reach it. What this protects, and what it does not, is ADR-101. A file is taken once. A file that changed while the page was open is refused, as saving would lose the change.
- The page draws the list and its popups again in JavaScript, in a pure function of the settings, apart from the page's own code, on a grid of cells with box lines drawn by the stylesheet, as gomi's site does: a terminal's columns hold whatever the browser's font does. It is checked against gh-desk: a test writes what the TUI draws from the page's pull requests, and `mise run config-ui:test` compares the page's text with it. A change to either side that the other does not follow fails.
- Closing the page, with its button or by closing the tab, ends the command and writes nothing, so that the comments of a file nobody changed stay. A reload also tells the tab went, so the command waits a moment for it to come back. A tab that goes unannounced, as when the browser is killed, leaves the command running until ctrl+c.
- What is compared is the list, the menu of enter, the review box and the confirm of a merge. The other scenes (the keys, the popup of S, the panes) follow gh-desk by hand, and colors are not compared.
- The checks on the page help with typing; what decides is the check on saving. They are the ones a value can be judged by alone.
- The frontend is Preact through htm's single file, kept in the repository with Lucide's icons, used as they are, without a build.
- What is not known from the settings is made up: the pull requests are the page's own, not the user's.
- Rejected: WebAssembly, for the build step. Rejected: a file opened with no server and its result copied by hand: a failed copy left an old clipboard to be applied, and the real check came only after leaving the page.

## Consequences

- A change to the list that the page does not follow fails `mise run config-ui:test`, and is fixed in `internal/config/ui/web/tui.js`.
- A new setting needs words on the page; the same check fails without them.
- A broken GitHub search cannot be told on the page; a tab has a link to try it.
