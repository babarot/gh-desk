# ADR-005: Lists come from search queries; notifications only mark them

## Status

Accepted

## Context

There are two places to learn which pull requests need attention:

- GitHub search (`review-requested:@me`, `author:@me`, `involves:@me`) returns pull requests with their state: checks, review decision, who is requested, latest reviews. It matches what is open now.
- Notifications (`GET /notifications`) say that something happened and why (`review_requested`, `mention`, `author`), but carry no state, and stay after the pull request is reviewed, merged or closed. In real use, most unread pull request notifications were about pull requests already merged or closed, so the notification list is not a list of what is left to do. Reading each one's state takes another request per pull request.

GitHub's search is also slow: its response time grows with the number of results asked for (about 3 seconds for 10, 5.6 for 25 and 9 for 50 pull requests with the fields the list needs), while its rate-limit cost stays at 1 to 2 points per page.

## Decision

- Each tab before the inbox is a GitHub search, defined in `[[sections]]` of the config file (`review`, `mine` and `involved` by default). A tab with `group = "requested"` splits its rows into requests to the viewer, requests only to their teams, and pull requests opened by bots, with the last two folded at start.
- Notifications are read for two things only: an unread mark (●) on the rows of every tab, and the inbox tab, which lists them with the reason and hides those whose pull request is merged or closed until asked. The inbox reads the state of its pull requests in one GraphQL query with an alias per pull request.
- A search is read in pages: a first page of 20 so the tab appears in a few seconds, then pages of 50 appended as they arrive, up to the tab's `limit`.
- What only one pull request needs (its body, its checks, its comments, its diff) is read when it is under the cursor or opened, and cached; the diffs of the rows next to the cursor are read ahead.

## Consequences

- A tab shows what is open now, with its state, in one query per page.
- A review sent from gh-triage can mark the pull request's notification done (`[inbox] done_after_review`), so the inbox empties as the work is done.
- The first page of a large tab is visible after a few seconds, and the rest fills in; the count in the tab's title is GitHub's total from the first page.
- Showing whether only approval checks are pending (a separate mark in the CI column) needs each check of each pull request, which the list does not read; it is left to the stage that handles required checks.
