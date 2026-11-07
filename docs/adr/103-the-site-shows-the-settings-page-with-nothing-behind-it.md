# ADR-103: The site shows the settings page with nothing behind it

## Status

Accepted

## Context

`gh desk config edit --ui` is hard to find from the docs, and what it is for, a list that follows the settings as they change, does not come across in words. ADR-100 makes the page one HTML file drawn by its own JavaScript, so it can be shown on the site as it is. It was built to run with gh-desk behind it, though: saving, closing and judging a rule are all answers from `internal/config/ui`'s server.

## Decision

- The site publishes the page at `settings/`, and the configuration guide embeds it. `demo/site/build.sh` makes it with `app.SettingsDemo`, the same `ui.Page` the command uses, so it is the page of the release the site is of (ADR-055).
- `ui.Input.Demo` makes this page. It draws the made-up pull requests, holds no file, and asks nothing of a server. A page on the site answers no one, so it takes this from what it was made with, not from its address.
- It shows only what needs no gh-desk and what most people change first: Appearance, List, Tabs, Merging and Keys, in a layout that fits the column of a docs page. It keeps the list above them, which is what it is there to show.
- It saves nothing. In place of saving it shows the file that would be written, and names the command.
- Rejected: the pages of the rules without their counts. A rule that matches nothing because nothing judged it reads as a result (ADR-102).
- Rejected: judging the rules in the browser with `gh-desk.wasm`. It loads the whole of gh-desk for a count on a docs page.
- Rejected: a screenshot or a recording. The page's point is that it answers, and a picture falls behind the page.

## Consequences

- The page on the site cannot show the rules, the settings that gain most from it. The guide says they are on the page gh-desk opens.
- A change that makes the page need the server for something else has to stay out of what `Demo` shows.
- Nothing on the site reaches the visitor's machine: the page is static, and its settings live only in the frame until it is closed.
