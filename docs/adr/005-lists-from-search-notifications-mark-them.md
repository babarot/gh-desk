# ADR-005: Lists come from search queries; notifications only mark them

## Status

Accepted
Partly replaced by ADR-062

## Context

There are two places to learn which pull requests need attention:

- GitHub search (`review-requested:@me`, `author:@me`, `involves:@me`) returns pull requests with their state: checks, review decision, who is requested, latest reviews. It matches what is open now.
- Notifications (`GET /notifications`) say that something happened and why (`review_requested`, `mention`, `author`), but carry no state, and stay after the pull request is reviewed, merged or closed. In real use, most unread pull request notifications were about pull requests already merged or closed, so the notification list is not a list of what is left to do. Reading each one's state takes another request per pull request.

GitHub's search is also slow: its response time grows with the number of results asked for (about 3 seconds for 10, 5.6 for 25 and 9 for 50 pull requests with the fields the list needs), while its rate-limit cost stays at 1 to 2 points per page.

## Decision

- The tabs are `[[tabs]]` of the config file, in the order shown. A tab is a GitHub search, or the inbox (`kind = "inbox"`), which may be anywhere among them; its name is free and is `inbox` by default. There is exactly one inbox: reading notifications comes with it, and every tab needs them (below). The default tabs are `review`, `mine`, `involved` and the inbox. A search tab may be split into groups (ADR-015).
- Notifications are read for two things only: an unread mark (●) on the rows of every tab, and the inbox, which lists them with the reason. The inbox hides those whose pull request is merged or closed (settled) until a key shows them, or from the start with `show_settled = true`.
- The keys 1 to 9 go to the tabs, so there are at most nine.
- A tab with no rows reads its search in pages: a small first page so the tab appears in a few seconds, then larger pages added as they arrive, up to the tab's `limit`. A tab with rows is read again as ADR-011 has it.
- What only one pull request needs (its body, its comments, its diff) is read when it is under the cursor or opened, and cached; the diffs of the rows next to the cursor are read ahead. The single checks are read for every row whose checks are pending, in one query, once the list's checks are known, so the CI column can tell the rows waiting only for an approval check (◌).
- The search reads only the light fields of each pull request; the heavy ones (checks, review decision, line counts) are read after it (ADR-011).

## Consequences

- A tab shows what is open now, with its state, in one query per page.
- A review sent from gh-desk can mark the pull request's notification done (`review.mark_done`), so the inbox empties as the work is done.
- The first page of a large tab is visible after a few seconds, and the rest fills in; the count in the tab's title is GitHub's total from the first page.
- Showing whether only approval checks are pending (◌ in the CI column) needs each check of each pull request with pending checks, one more query per list.
- The inbox cannot be left out of the tabs: that would need notifications read apart from any tab, which they are not.
