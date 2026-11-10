# Reading a pull request

The pane under the list shows the pull request under the cursor. `p` shows or hides it, and `+` and `-`, or dragging its top edge, change its height. The height is kept for the next launch.

## Overview

The overview tab draws the description as Markdown (headings, lists, code, quotes and tables) beside a column of facts: the checks, the reviews, and what blocks a merge. The comments follow the description. HTML in a description is drawn as Markdown too, shortcodes such as `:rocket:` as their emoji, and `description.style = "plain"` shows it as written.

## Diff

`v` switches between the overview and the diff. The diff shows one file at a time, with the words that changed marked within each line:

- `]` and `[` go to the next and the previous file
- `z` folds a file to its name and line counts; `diff.collapse` lists files to fold from the start, such as lock files
- `J` and `K` scroll the pane while the cursor stays in the list

[The diff of a pull request](https://babarot.me/gh-desk/play/?scenario=diff "playground")

`preview.tab = "diff"` opens the pane on the diff.

## Checks

The CI column sums up the checks. Checks that pass only after an approval, such as a policy bot's, can be named in `checks.approval`: while only they are pending, CI shows ◌, and they count as passing when a merge is decided.

## Conflicts

The MERGE column marks a pull request that conflicts with its base with ✗, and one behind its base with ↓; · is GitHub still working it out. It is read again with each reading of the list, so a pull request that comes to conflict when its base moves is marked at the next one. The pane's merge line says what a merge would do to the pull request it shows, such as "conflicts" or "wait for checks → merge". A rule's `merge` condition reads the same state ([Selecting by rules](selecting.md)).
