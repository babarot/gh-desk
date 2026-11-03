# ADR-053: The docs are Markdown in docs/, built into a site by Hugo

## Status

Accepted

## Context

gh-desk has more than the README can explain: how tabs and groups are read, how rules select, what a merge waits for, what Claude may and may not do. The playground (ADR-047, ADR-052) can show a screen, but not say why it is so.

The repository is Go only. A documentation site built with a JavaScript toolchain (Starlight, as gh-infra has) brings a package.json, a node_modules and their updates for the site's sake. Hugo is one binary, and its themes are Go modules.

The reference pages are already Markdown in [docs/reference](../reference), written from the code (ADR-051), and read on GitHub.

## Decision

- The pages of the documentation are Markdown files in `docs/`, written to be read on GitHub as they are: a page starts with its `# title`, has no front matter, and links to another page by its `.md` file. `docs/adr`, `docs/architecture.md` and `docs/site` are not pages.
- [docs/site](../site) builds them into a site with Hugo and the Hextra theme: its build copies the pages into a Hugo site of its own, giving each the front matter Hugo needs. The pages never carry what only the site needs.
- A link is taken, as the site is built, to where it leads there: a page's link to another page to that page, and a link to the rest of the repository (an ADR, the code) to the file on GitHub. A link that leads nowhere stops the build, as does any warning of Hugo; the Test workflow builds the site on every pull request.
- A link to the playground goes to its page on the whole window. A link to it titled `playground` is embedded instead, watching its scenario (ADR-052), and loads nothing until the reader asks it to play. On GitHub, both stay links to the playground.
- The site wears the playground's colors and fonts, so that going between them is not going to another site.
- Hugo's version is in [mise.toml](../../mise.toml), the same for a local build and CI.
- Rejected: Starlight, or another site generator on Node, for the toolchain it brings to a Go repository; its components and animations are what the playground already gives.
- Rejected: pages in a directory only the site reads, with front matter. They would not read on GitHub, and the reference pages of ADR-051 would live apart from the rest.
- Rejected: shortcodes in the pages to embed the playground. A shortcode shows as its source on GitHub; a titled link reads as a link there and is embedded on the site.

## Consequences

- A link that works on GitHub but leads to no page on the site, or to no file in the repository, fails CI until it is fixed.
- A page's order in the sidebar is listed in the site's build, not in the page; a new page not listed comes after the listed ones.
- A page named `claude.md` would be read by Claude Code as its instructions on a filesystem that ignores case, so the page about Claude has another name.

## Not decided here

- Where the site is published, and how it is reached from the playground's page.
