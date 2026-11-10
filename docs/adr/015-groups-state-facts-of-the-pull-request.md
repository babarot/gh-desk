# ADR-015: Groups state facts of the pull request

## Status

Superseded by ADR-118

## Context

A search tab can split its rows into groups. Which group a pull request falls in is decided by `plan.GroupOf` from the pull request alone: every search reads the same fields, among them whether the author is a bot and whom a review is requested from, users and teams apart. The tab's query is never looked at, so grouping works on any tab.

A label that assumes the tab's query is wrong on another query. "via team" for every pull request that is neither a bot's nor requested from the viewer reads right on `review-requested:@me`, the default review tab: there, a pull request not requested from the viewer in person is requested from one of their teams. On a tab of `author:@me`, nearly every row would be "via team", since one's own pull requests are not requested from oneself, though no team was asked for anything.

Tabs and their queries are meant to stay free to write, and the grouping should be understood without documentation explaining which queries it suits.

## Decision

- A group's label does not assume anything about the tab's query. It states only a fact of the pull request, which holds whatever the query is.
- There are four groups, decided in this order, the first that holds:
  1. bots: the author is a bot.
  2. you: a review is requested from the viewer in person.
  3. team: a review is requested from at least one team. Whether the viewer is on that team is not known from an arbitrary query, so it is not looked at, and the label is "requested from a team".
  4. other: none of the above; the group that takes the rest.
- Groups do not overlap, so a label says that its fact holds, not that the others do not: a bot's pull request is under bots even when it is requested from the viewer.
- A tab's `groups` lists the groups it is split into, in the order shown; writing it splits the tab. A group left out is not shown, and nothing in it is selected or acted on from that tab. `folded` names the groups folded at start, among those.
- A group with no rows is not shown, its head included. This holds under a filter too, so that what the list shows does not depend on how the rows were narrowed.
- Groups defined in the config file (their conditions and labels) were rejected: their conditions, the order they are tried in and where the rest go are much to learn and hard to explain in the config file's comments, and ADR-014 keeps settings to what differs between users. They will be considered when there is a concrete need, such as splitting by team.

## Consequences

- On the review tab, "requested from a team" says only that some team was asked, which is what can be told on any query; that the viewer is on it is left unsaid. This is the trade-off the rule asks for.
- A grouped tab where only one group has rows shows that group's head alone, which still lets it be folded. A tab of one's own pull requests is under other only.
- Only the first ten review requests of a pull request are read (`reviewRequests(first: 10)` in `internal/github`). A pull request whose team request comes after ten others is put under other, which breaks the rule. It is rare enough that the limit is kept; this is the one known exception.
- A grouped tab can show no rows at all, when a filter leaves none; it shows "nothing here", as a tab that is not grouped does.
