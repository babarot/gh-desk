# ADR-020: HTML in a description is drawn as its Markdown, read by an HTML tokenizer

## Status

Accepted

## Context

ADR-013 draws a description and a comment's body from Markdown with gh-desk's own code, and shows anything else as written, HTML included, apart from a few tags it removes. The comments bots post are often HTML: a coverage report is a `<h2>` and a `<table>`, Renovate folds its release notes in `<details>`, and links are `<a href>`. Shown as written, such a comment is a wall of tags in which the numbers are hard to find, though the HTML only says what Markdown also says: a heading, a table, a list, bold text, a link.

A body on GitHub is Markdown with HTML in it, not an HTML document. Converting it whole with an HTML-to-Markdown library would read the Markdown between the tags as HTML text and escape it. Reading the tags with regular expressions breaks on what HTML allows and bots write: attributes without quotes (`href=https://...`), `>` inside a quoted attribute, entities, end tags left out.

## Decision

- Before a body is drawn from its Markdown, its HTML is turned into the Markdown that `internal/tui/markdown.go` draws: a tag that has a Markdown form becomes it, and other tags go while their text stays. The text between tags is Markdown and is left as written; so are fenced code blocks and code spans, whose `<` is not a tag.
- The tags are read with the tokenizer of `golang.org/x/net/html`, an outside module of `internal/tui` (ADR-002), not with regular expressions of gh-desk's own.
- `description.style = "plain"` still shows the body as written. It applies to the comments as well as to the description, in the pane and on the PR screen.

This replaces two sentences of ADR-013: "Anything else is shown as written." in so far as it concerns HTML, and "It applies to the pane and to the PR screen's overview." The rest of ADR-013 holds: gh-desk draws the Markdown with its own code, not glamour.

## Consequences

- Only HTML that has a Markdown form is drawn as such. Tags that have none (images, alignment, colors) go and leave their text; a table nested in a table is read as text of the outer one's cell.
- The tokenizer reads a tag where GitHub would also read HTML, so a `<word>` in prose outside code goes, as GitHub hides it too. An autolink, `<https://...>`, is the one tag-shaped text that is kept.
- `golang.org/x/net` is another module to keep up to date.
