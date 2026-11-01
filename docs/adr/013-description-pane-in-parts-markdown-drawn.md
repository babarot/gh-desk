# ADR-013: The description pane is in parts beside a column of facts, and Markdown is drawn

## Status

Accepted

## Context

The preview pane's overview tab showed one line of facts (the branches, the team reviewers, the reviews, the merge plan) above the description's lines as they were written. Headings kept their `##`, task lists their `- [x]`, and a table or a code block was cut at the edge. What a reviewer looks for beside the description (whether the checks passed and which failed, the size, the labels, when the pull request was opened and last updated, what was said in the comments) was not in the pane, though gh-triage already reads all of it: the size, labels and comment count with the heavy fields of ADR-011, the checks and the comments with the description, the merge plan with the merge information. gh-news shows the same kind of facts above a description it draws from Markdown.

Several layouts were drawn in a mock and compared: the facts above the description, the description and the checks and comments as parts that fold, the facts in a column on the right, and the last two together.

## Decision

- The overview tab shows, beside the description, the facts a reviewer looks for (status, merge plan, reviews, checks, labels, size and the like), from what gh-triage already reads. On a wide pane they stand in a column on the right that does not scroll; on a narrow one they go above the description. The width where this changes is a constant.
- A fact that has a page opens it in the browser on a click, as ADR-012 lets the mouse open pages.
- The description, the checks and the comments are parts that fold, by key and by click. Folds hold for every pull request until gh-triage quits.
- A description, and a comment's body, is drawn from its Markdown by gh-triage's own code in `internal/tui/markdown.go`, covering the part of Markdown that pull request descriptions use. Anything else is shown as written.
- `description.style` in the config file chooses between that (`"rendered"`, the default) and the Markdown as written (`"plain"`). It applies to the pane and to the PR screen's overview.

## Consequences

- A Markdown renderer such as glamour is not used: it would be another outside module (ADR-002), it draws with margins and colors of its own that would have to be matched to the theme, and pull request descriptions use a small part of Markdown. What the own code does not draw (HTML other than the tags removed, nested quotes, footnotes, images) shows as written; a description that relies on it reads better with `"plain"`.
- Nothing new is read from GitHub. A fact not read yet (the heavy fields before they arrive, the merge plan before the merge information) is left out of the column until it is.
- The pane's top lines, the column's facts and the parts' headers are what clicks and `unfold` find; `descPane` records which part each drawn line heads and where its links are, so a change to the drawing keeps them in step. `description_test.go` checks folding by key and by click, and the links by click.
- At the default pane height most of the column is cut off; the facts are ordered so that the ones a review needs first come first.
