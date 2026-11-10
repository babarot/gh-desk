# ADR-119: One scope narrows every tab

## Status

Accepted

## Context

The user wanted to look at the pull requests of one owner at a time, as chmouel/gh-news groups its notifications by owner, and to do so on any tab and any query. Several ways were laid out in mocks: a split of each tab by owner (`split = "owner"`), sections nested under an owner, an owner as a tab of its own, and one value shared by every tab. The user looks at one owner across the review, mine and inbox tabs, not at one owner on one tab and another on the next.

Sections (ADR-118) split a tab by rules the user writes; an owner is not known when the file is written, and a section for each would be a list to keep up.

## Decision

- `list.scope` in the config file is `"owner"`, `"repo"` or `"author"`, or left out. While it is set, one value of it, or none (`all`), is chosen at a time, and every tab is narrowed to it: the rows, the counts in the header, and what acts on the rows shown (ADR-027, ADR-062). A selection is kept when the value changes, as when a filter hides it.
- The scope is a narrowing, not a part of a tab: it does not change what a tab reads, the record of the lists read (`lists.json`), or what is excluded. Rules that approve on their own (`[[auto_approve]]`) and the bell are not narrowed, since a pull request out of sight is still one to approve or to hear of.
- The values are those of the pull requests the tabs hold, by name, with `all` first. A value only `[[exclude]]` or the inbox's settled notifications hold is not offered; one only a tab's excluded section holds is, so that the list does not change as rows are judged. The order is by name, not by count or update time, so that stepping through it goes the same way each time.
- The value chosen is kept in `layout.json` (ADR-012) with the scope it is of, and taken at the next launch. It is let go, for that launch only, once every tab is read and none holds it; not before, since a tab not read yet may hold it.
- While a scope is set, the header shows the value chosen in place of the viewer, and who is signed in is said elsewhere. Its width does not change with the value, so that the tabs do not move as it is stepped through. Choosing a value with the mouse writes nothing, as ADR-012 has the mouse do.
- Rejected: a split of each tab (`split = "owner"`). Each tab would have its own value, which the user does not use, and the header would need a line for each.
- Rejected: sections nested under an owner. Folding, selecting and the cursor would each have two levels, and Claude's triage would name both.
- Rejected: an owner as a tab. The number keys would go to owners, which come and go with the data, so a key would not keep its tab.

## Consequences

- A tab's count with a value chosen is what its rows hold, as while a filter narrows it (ADR-083); a search tab cannot count what it did not read.
- Changing `list.scope` takes effect at the next launch, as the settings that are not of the display do.
- A new place that acts on the rows shown, or counts them, narrows by the scope through the same question every such place asks (ADR-062).
