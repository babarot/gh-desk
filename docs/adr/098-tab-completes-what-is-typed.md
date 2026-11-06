# ADR-098: Tab completes what is typed

## Status

Accepted

## Context

`fill_suggestion` filled a suggestion into an empty box (ADR-039). With the questions kept (ADR-096), a text box knows much of what a user is about to type, and a triage is told every row of the tab, so a question can name a pull request or an author, which took typing them out. The filter already completes with `complete` and draws what would follow, faint, after the cursor (ADR-078).

## Decision

- The two are one action, `complete`, and `fill_suggestion` is its old name (ADR-023). This replaces, in ADR-039, that it fills in only an empty box: it also fills in what follows the text typed, when a question offered or asked before starts with it, ignoring case.
- What would be filled in is drawn faint after the cursor, while the cursor is at the end of the text and no question is recalled. `right` at the end fills it in as well.
- The cursor stays where the typed text ends, for an input method (CLAUDE.md). A terminal draws the text being composed there and tells the program nothing until it is committed, so the faint text cannot be taken away while composing. It is covered as far as the composed text goes, its end stays beside that, and it goes on commit. This was looked at in a terminal with an input method and kept: it hides nothing typed, and a menu that opens on a key would show the rest of a question only when asked for.
- In a triage, `#` and `@` at the start of a word open a menu of the tab's pull requests and authors, chosen as the rows Claude is given, so nothing is named that Claude does not have. Choosing replaces the word, as the list writes the name, and sends nothing. `esc` closes the menu for that text.
- The claude pane has no menu. A conversation about one pull request is not given the others, so naming one would offer what Claude cannot read.
- Slash commands are not offered: a text starting with `/` is sent as typed, as Claude Code may read it as a skill of its own.

## Consequences

- A `[keys]` entry for `fill_suggestion` is reported as a mistake that names `complete`.
- `complete` is read beside the actions of the claude pane, so its key is checked against theirs.
- With the menu open, the keys that move through the questions move through the menu.
