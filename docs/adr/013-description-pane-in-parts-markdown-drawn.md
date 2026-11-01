# ADR-013: The description pane is in parts beside a column of facts, and Markdown is drawn

## Status

Accepted

## Context

The preview pane's overview tab showed one line of facts (the branches, the team reviewers, the reviews, the merge plan) above the description's lines as they were written. Headings kept their `##`, task lists their `- [x]`, and a table or a code block was cut at the edge. What a reviewer looks for beside the description (whether the checks passed and which failed, the size, the labels, when the pull request was opened and last updated, what was said in the comments) was not in the pane, though gh-triage already reads all of it: the size, labels and comment count with the heavy fields of ADR-011, the checks and the comments with the description, the merge plan with the merge information. gh-news shows the same kind of facts above a description it draws from Markdown.

Several layouts were drawn in a mock and compared: the facts above the description, the description and the checks and comments as parts that fold, the facts in a column on the right, and the last two together.

## Decision

- The overview tab has the title and author, the unread comment and a bot's updates at its top, and, when the pane is at least 100 cells wide inside its border, a column of facts on the right, 34 cells wide: status, merge plan, reviews, requested reviewers, checks counted by result, labels, size, branch, opened and updated. Narrower, the facts go above the parts, in the scrolled lines. A value longer than the column breaks onto the next line; facts that do not fit in the pane are counted on its last line (`+2 more`), and the column does not scroll.
- A click on a fact that has a page opens it in the browser, as ADR-012 lets the mouse open pages: a reviewer's profile, a team's page, the checks tab for the checks, the files tab for the size, and the repository's open pull requests with a label for that label. The branch has no link, as its repository is not read for a pull request from a fork.
- The left side holds the parts the column does not already say, in this order: the description, the checks one by one (required ones marked, from the merge information), and the comments read with the description. Each part has a header and folds. `next_file` and `prev_file` jump between parts as they jump between files in the diff; `unfold` folds the first part whose header is in sight, or the part the pane is in; a click on a header folds it, as ADR-012 lets the mouse point. Folds hold for every pull request until gh-triage quits.
- A description, and a comment's body, is drawn from its Markdown by gh-triage's own code in `internal/tui/markdown.go`: headings without their `#`, bullets, task boxes, numbered items, quotes, rules, inline code and bold, code blocks on their own background with their language, and tables with their columns lined up. Prose wraps at the edge; code blocks and tables are cut there. Anything else is shown as written.
- `description.style` in the config file chooses between that (`"rendered"`, the default) and the Markdown as written (`"plain"`). It applies to the pane and to the PR screen's overview.
- The diff tab keeps its line of facts.

## Consequences

- A Markdown renderer such as glamour is not used: it would be another outside module (ADR-002), it draws with margins and colors of its own that would have to be matched to the theme, and pull request descriptions use a small part of Markdown. What the own code does not draw (HTML other than the tags removed, nested quotes, footnotes, images) shows as written; a description that relies on it reads better with `"plain"`.
- Nothing new is read from GitHub. A fact not read yet (the heavy fields before they arrive, the merge plan before the merge information) is left out of the column until it is.
- The pane's top lines, the column's facts and the parts' headers are what clicks and `unfold` find; `descPane` records which part each drawn line heads and where its links are, so a change to the drawing keeps them in step. `description_test.go` checks folding by key and by click, and the links by click.
- At the default pane height most of the column is cut off; the facts are ordered so that the ones a review needs first come first.
