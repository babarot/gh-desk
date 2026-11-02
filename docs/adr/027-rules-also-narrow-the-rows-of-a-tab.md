# ADR-027: Rules also narrow the rows of a tab, which match them again at every draw

## Status

Accepted

## Context

ADR-026 makes the popup of S a place where rules are put together into one set of pull requests, which replaces the selection or is added to it. Selecting is for acting on the set with A and M. It does not help with reading the set: the rows that match stay among those that do not, and taking one out of the set before acting means finding it in the whole tab.

The list already has a filter typed with `/`, kept once for every tab. A filter by rule differs from it in what it needs: a rule may read a pull request's body (the packages) and its heavy fields (the size, the checks), and both are read per tab, when the tab is read and when the popup of S opens on it.

## Decision

This adds to ADR-026; it replaces none of it.

- The set the popup makes can also narrow the rows of the tab shown to it, besides replacing or adding to the selection. Narrowing leaves the selection as it is.
- The rules a tab is narrowed to are kept per tab, not once for every tab like the filter typed. A pull request whose body or heavy fields a rule needs and has not been read never matches, and a row that is not shown is not read from the pane either, so with the rules kept for every tab, a tab whose bodies were never read would show none of its matches, for good. When a tab narrowed to a rule that reads bodies is read again, search tab or inbox alike, the bodies of its pull requests not read yet are read too.
- The rules are matched again at every draw, as the filter typed is; the pull requests that matched when the user chose are not kept. A pull request approved, merged or pushed to shows or leaves as the rules say of it now.
- The filter typed and the rules both apply: a row is shown when it passes both. Whatever acts on the rows shown, such as selecting a group by its header, asks the same question of a row as the drawing does, so nothing hidden by a filter is acted on.
- A triage gives Claude every row of the tab, as before, and marks those the rules leave out: Claude can apply the filter typed by itself, but not rules it knows only by name.
- Taking the filters away takes both at once: they are two ways of saying what the tab shows, and keeping them apart would add a step to every clearing.

## Consequences

- A tab can be narrowed while another shows everything; going back to it shows it still narrowed.
- A rule that reads bodies costs a read of the new pull requests' bodies each time a narrowed tab is read again.
- A pull request leaves the rows for as long as its heavy fields are not read for its head, after a push, say, and comes back once they are.
- A new place that acts on the rows shown must ask the same question as the drawing, not test the filter typed alone.
