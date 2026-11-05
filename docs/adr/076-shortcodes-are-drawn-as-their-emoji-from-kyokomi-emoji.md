# ADR-076: Shortcodes in a description are drawn as their emoji, named by kyokomi/emoji

## Status

Accepted

## Context

People write emoji in pull request descriptions and comments as shortcodes such as `:rocket:` or `:white_check_mark:`, and GitHub shows them as emoji. gh-desk reads the body as Markdown (`body`, not `bodyHTML`), so it showed the shortcodes as written, and a description read differently in the pane than on GitHub.

Turning a shortcode into its emoji needs a table of names. GitHub's own is gemoji. Keeping a table generated from it in this repository would match GitHub exactly, but leaves a generator and its updates to gh-desk. `github.com/kyokomi/emoji/v2` holds such a table, generated from gemoji and Unicode's names, and needs nothing beyond the standard library.

## Decision

- When a description or a comment is drawn from its Markdown (ADR-013, ADR-020), a shortcode in prose, bold text or a table cell is drawn as the emoji it names. Code, in a span or a fenced block, stays as written, as GitHub keeps it.
- A shortcode with no emoji in the table stays as written. GitHub's own images, such as `:shipit:`, have no Unicode character and are among them.
- The names come from `github.com/kyokomi/emoji/v2`, an outside module of `internal/tui` (ADR-002). Only its table is used: its functions that print or replace text are not, as they add a space after each emoji through a package variable and do not tell code from prose.
- `description.style = "plain"` shows the shortcodes as written, as it shows the rest of the body.

## Consequences

- The table holds names GitHub does not know, Unicode's names such as `:grinning_face:`, so a few shortcodes that GitHub shows as written are drawn as emoji. They are rarely written in a pull request.
- An emoji is two cells wide by the width gh-desk measures; one that a terminal draws at another width, such as one with a variation selector, can shift what follows on its line, as an emoji typed into a body already does.
- `github.com/kyokomi/emoji/v2` is another module to keep up to date, and new emoji reach gh-desk only through its releases.

## Not decided here

- Whether pull request titles, in the list and elsewhere, are drawn with their emoji too.
