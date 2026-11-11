# ADR-118: A tab is split into sections written as rules

## Status

Accepted
Partly replaced by ADR-120

## Context

A search tab could be split into four fixed groups (ADR-015): bots, requested from you, requested from a team, and other. `groups` chose which were shown and in what order, `folded` which were folded at start, and `sort = "drafts_last"` moved the drafts down. A group left out was neither shown nor counted.

The user wanted to see, in the tab of their own pull requests, those GitHub would let them merge now, and to leave those out of the review tab. A rule can ask it (ADR-117), but the groups could not: they say whom a review is asked of, and whether a pull request can be merged is another question. A fixed `ready` group beside you, team and bots would need an order between the two questions and exceptions to it. Groups, the drafts' sort and exclusion each filled a gap the others left, with three mechanisms where one would do; ADR-015 had rejected groups written in the config file until there was a concrete need, and this was one.

## Decision

- A tab, the inbox too, may be split by `sections`: a list of inline tables in the order shown. A section has the conditions of a `[[select]]` rule but `packages`, as an exclusion rule has (ADR-062), an optional `name`, and `show`. Their order is the array's, as ADR-016 has order said.
- A row goes in the first section whose rule says Yes of it. A section with no condition takes all that is left. The sections are judged and shown in the same order: a second order to learn was rejected, and the default review tab writes its order with `bot = false` on the first three.
- What no section takes goes in the section named `other`, where it is written, or in one put at the end with that name. A row is never dropped for meeting no section.
- `show` is one of `"open"`, `"folded"` (folded at start) and `"excluded"`, a string since the three are one choice, and two booleans would allow folded and excluded together. An excluded section leaves its rows out of the tab as an exclusion rule does (ADR-062): counted as excluded (ADR-063), shown with the key that shows the excluded, and never acted on while out of sight.
- A section that cannot tell yet (Pending, as for heavy fields) is passed over, and the row goes in a later one until it can; a row is never left out by a section that waits, as ADR-062 has it for exclusion rules.
- A section with no name has no head: its rows follow the section above, and it cannot be folded. The names of the sections of a tab are not written twice, since folding and Claude's triage name a section by it.
- An excluded section is how a tab leaves pull requests out of itself alone. `[[exclude]]` stays as it was, for every tab, and is the only exclusion auto-approve reads: a section is how a tab shows, not which pull requests are approved without a confirm.
- A row that shows a result is placed in its section by what it was when its batch started, as ADR-064 sorts it.
- `groups`, `folded` and `sort = "drafts_last"` are no longer read. A file that has them is told where, with the `sections` that show what they did, written out; it is not converted silently, since a file read differently from how it is written misleads its reader.
- The default tabs look as they did: the review tab in the four sections of the groups, the team's and the bots' folded; the tab of one's own pull requests in two sections with no name, drafts after the rest.
- Rejected: a fixed `ready` group, a mark in the MERGE column (ADR-114), a word of the filter, a tab of its own for what can be merged, and a sort that puts those first with a line under them. Each answers this one question and leaves the next one, splitting by something else, to a new mechanism.
- Rejected: sections in YAML, or as `[[tabs.sections]]` tables. Moving to YAML would remake where mistakes are reported, the schema, the reference written from the code and the settings page; a list of inline tables reads, one a line, in the order it is judged.

## Consequences

- What earlier ADRs say of a group and its head holds of a section and its head: selecting by the head, folding, counting and the cursor (ADR-027, ADR-044, ADR-062, ADR-083, ADR-115).
- A file written for groups no longer starts until it is rewritten. The rewrite is given in full where the mistake is, so the cost is one edit.
- Splitting by whom a review is asked of is now written in the file; a section's head says what its name says, which the user chose. ADR-015's rule that a head states only a fact of the pull request holds of the default tabs, not of what the user writes.
- A section on a heavy field moves a row once that field is read, as an exclusion rule on one does.
- This replaces ADR-015 whole, and in ADR-005 "A search tab may be split into groups (ADR-015)." with the first rule above.
