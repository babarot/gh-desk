# ADR-084: A review's result is put away by the automatic refresh after an interval

## Status

Accepted

## Context

ADR-007 keeps every result on its row until `r`, because a batch of merges can take minutes while the user looks elsewhere, and a result put away by time or by the automatic refresh may never have been seen.

An approval or another review is sent at once, while the user is looking at the row it was sent from. Sending it takes the pull request out of a search such as `review-requested:@me`, yet the row stayed in the review tab, as approved, until `r`. The tab filled with pull requests that no longer ask anything of the user. The result is kept in the in-app notifications as well (ADR-007), so it can be found after the row is gone.

Putting the result away at the next automatic reading would show it for as little as a moment: the readings come at a fixed interval, and a review sent just before one would be gone seconds later.

## Decision

- This replaces, for the results of reviews, the sentence of ADR-007 "Nothing is put away by time or by the automatic refresh."
- The result of a review sent, an approval included, is put away by an automatic reading that comes at least one interval of the automatic refresh after the result. A reading that comes sooner leaves it, and the next reading asks again. The result is thus shown for at least an interval, whenever it came in the interval.
- Once its result is put away, a row goes as at `r`: it leaves a tab whose reading no longer has its pull request, and sorts as it is (ADR-064).
- Every other result, merges, auto-merge, ready, close, skips, stops and failures, stays until `r` as ADR-007 decides. A failure is not put away, since it asks the user to act.
- When the automatic refresh is off, nothing is put away but by `r`.
- Rejected: putting the results of reviews away at the next automatic reading. It shows a result for anything between a moment and an interval.
- Rejected: a time of its own after which a review's result is put away. It would be a second clock beside the refresh, and a setting nobody would tune; the interval of the refresh is already how long the user leaves the list unread.
- Rejected: reading each pull request again on a timer of its own when its result is due. The readings stay per tab; only whether a result may be put away is decided per pull request.

## Consequences

- A new kind of result must be placed on one side: put away by the automatic refresh after an interval, or kept until `r`.
- How long a review's result is shown depends on the refresh interval: between one and two intervals.
- The guarantee is checked by [fade_test.go](../../internal/tui/fade_test.go).
