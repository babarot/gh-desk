# ADR-024: The key line shows the ways in

## Status

Accepted

## Context

The line of key hints at the bottom of the screen listed most of what could be done on the list, and was cut on a common terminal width before it reached `?`, the key that tells all the others. Most of what it listed is told elsewhere: the menu of enter shows what can be done to a pull request or a selection, each with its key and whether it can run now; the preview pane's border shows its own keys; the header shows the key of the events when there are some. The line repeated them, and what it alone could tell was cut off.

## Decision

- The key line holds the ways in, the menu of enter and `?`, and the keys of what has the focus that nothing else on the screen tells. A key told by the menu, the preview pane or the header is not on it.
- `?` is always on it. Where the line is too short, the other hints go from before it, so `?` and every key it tells can still be found.
- esc is told where it steps back. On the list it is told only when it would clear a selection or a filter, not where it would quit.
- Which hints each screen has, and their words, are code.

## Consequences

- A new action on the list does not go on the key line when the menu or the pane already tells it; one that nothing tells does, or is reached through `?`.
- A user who does not open the menu or `?` sees fewer keys than before; the line points to both.
