# ADR-106: The landing page, the docs and the playground are pages of their own

## Status

Accepted

## Context

The site grew a page at a time. ADR-047 put the playground at the root, and that page came to explain gh-desk as well. ADR-052 needed the terminal on the whole window and in the docs' frames, so one HTML file served both: a parameter hid what was around the terminal, and the build copied the file to `play/`. ADR-103 added `settings/` beside it.

The word "playground" then meant two addresses, and the page people land on had no name. The settings page on the site and the one `gh desk config edit --ui` opens shared a name too, though only one saves. Every page sat side by side, whatever it was for.

Nothing outside the repository depends on these addresses yet, so they are set by what they are for.

## Decision

This replaces three sentences. Of ADR-052: "`play/` is the playground on the whole window, a page of its own beside the page about gh-desk; the build writes it from the same page, so the two cannot differ." Of ADR-055: "The playground and the docs are built and published together, as one site: the playground at the root, the docs under `docs/`, and the playground on the whole window at `play/` (ADR-052)", which now holds for the addresses below. Of ADR-103: "The site publishes the page at `settings/`, and the configuration guide embeds it", for its address, with "`ui.Input.Demo` makes this page", now `ui.Input.Playground`, and "in a layout that fits the column of a docs page", since it also has the whole window.

- The site has three parts, by what a visitor does there, and these names are used in the code, the docs and the UI:
  - the landing page, at the root, says what gh-desk is
  - the docs, at `docs/`, are read
  - the playground, at `play/`, is tried; the settings playground, at `play/settings/`, is the same for the settings page
- The playground is what runs, not an address: gh-desk in the browser against a made-up GitHub. The landing page and the docs embed it. "The settings page" is the one the command opens.
- Each of these is its own page in [demo/site/web](../../demo/site/web). The landing page runs no terminal, and frames the playground as the docs do, so there is one way to embed it.
- The playground takes being in a frame from where it is, not from a parameter.
- The landing page shows how to change gh-desk with the settings page, and embeds the settings playground. It keeps TOML only for what the settings page cannot do.
- Rejected: `settings/` beside `play/`. It would be a fourth part for what is the same kind of thing as the playground, and the address would read as the page that saves.
- Rejected: `playground/` as the address. `play/` is shorter to share and is what other playgrounds use.
- Rejected: "home" for the landing page. Hugo's home is the first page of the docs.
- Rejected: one HTML file for the landing page and the playground. The copy, its base, and the styles that hid the landing page were what made them hard to change apart.

## Consequences

- A scenario played from the landing page plays in its frame. Driving from there leaves the frame for the playground, as it does in the docs.
- The playground and the landing page share only `site.css`; a style for one does not reach the other.
- A new page that lets a visitor try something goes under `play/`.
- The site keeps the theme under one storage key, and a frame hears of a switch through that key. A page that switches the theme has to write it there.
