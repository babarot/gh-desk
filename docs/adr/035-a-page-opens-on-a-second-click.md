# ADR-035: A page opens on a second click

## Status

Accepted

## Context

ADR-012 lets the mouse open pages in the browser, and ADR-013 lets a fact of the pane open its page on a click. A click on a link opened its page at once. Links sit among what a click is also used for: a pull request's name and its author are on the row a click picks, and the pane's facts are beside the parts a click folds. A click meant to pick a row or fold a part could land on a link and throw the user out to the browser, taking the focus away from the terminal.

## Decision

- A click on a link does not open its page. It marks the link as waiting, and the screen says so where the link is drawn. A second click on the same link while it waits opens the page.
- The wait ends after a while, at any key, and at any other mouse input. A click after that waits anew.
- This holds for every link the mouse can open, so that no link opens on one click. It replaces the words "on a click" in ADR-013's sentence "A fact that has a page opens it in the browser on a click".
- How long a link waits is a constant, not a setting: it is how long a hand takes to click again, which does not differ between users (ADR-014).
- Rejected: a double click on a link. It is what opens a pull request on a row, and two quick clicks give no time to see what is about to open.
- Rejected: asking in a popup. A popup takes the focus and a key to answer, which is more than opening a page in the browser needs.

## Consequences

- Opening a page with the mouse takes two clicks.
- A new link the mouse can open has to be found by `linkUnder` in `internal/tui/mouse.go`, which both the click and the drawing of the waiting link use; a link found elsewhere would open on one click or not show that it waits.
