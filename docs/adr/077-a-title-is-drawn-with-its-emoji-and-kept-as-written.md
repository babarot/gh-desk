# ADR-077: A title is drawn with its emoji and kept as written

## Status

Accepted

## Context

ADR-076 draws the shortcodes of a description as emoji and leaves titles open. GitHub shows a title's shortcodes as emoji too (`titleHTML` of a pull request titled `:sparkles: Renderer benchmarks` is `✨ Renderer benchmarks`), and repositories that follow gitmoji start most titles with one, so the list showed a column of `:sparkles:` and `:bug:` where GitHub shows emoji.

A title is more than text to read. A rule matches it with a regular expression from the config file (`title = "^:arrow_up:"` for a bot's updates), the filter `/` matches it as typed, Claude reads it, and the list cache keeps it on disk. Turning it into emoji when it is read would change every one of them under the user: a rule written against the shortcode would stop matching without a word. Drawing the emoji while the filter matched only the shortcode would leave a row whose title shows `✨` that `✨` does not find.

## Decision

- A pull request's title is kept as GitHub wrote it, with its shortcodes. Rules, Claude and the list cache read it so.
- A title is drawn with its shortcodes as emoji, by the table of ADR-076, wherever gh-desk draws it: the list, the pane, the popups and the menus. Code between backquotes stays as written. Every title drawn goes through one function in [`internal/tui/markdown.go`](../../internal/tui/markdown.go), so that one pull request reads the same on every screen.
- The filter `/` matches a title both as written and as drawn, so either `:sparkles:` or `✨` finds it. What Claude is told of the filter says the same, since a filter Claude plans is matched the same way.
- The title is not fetched as `titleHTML`: turning its HTML back into text is more than a table of names needs.

## Consequences

- A new place that draws a title must call the function, or that screen shows the shortcode while the others show the emoji. No test finds a place that does not.
- What is drawn and what a rule matches differ for a title with shortcodes; the config reference says that a rule's title is matched as written.
- A filter typed as another shortcode of the same emoji, such as `:thumbsup:` for a title written with `:+1:`, does not find it; the shortcodes typed are not turned into emoji.
