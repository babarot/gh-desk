# ADR-062: Exclusion rules leave pull requests out of every tab until a key shows them

## Status

Accepted
Partly replaced by ADR-063

## Context

A tab lists what its search returns, and some of it is known not to need the user: dependency updates left alone for a month, pull requests labeled as not ready. `d` puts one away, but only in memory and only one tab at a time, so they come back on the next run. The inbox already leaves some rows out on its own: the notifications whose pull request is merged or closed (settled), until a key shows them (ADR-005).

The rules of S (ADR-026) can already say what a pull request is. A rule that leaves pull requests out differs from one that selects in what an answer it cannot give yet must do: S does not select what a rule cannot tell, since it acts on it, while a row hidden because its checks are not read yet could stay hidden for good if they never are. A rule therefore says Yes, No or Pending (`plan.Rule.Judge` in [internal/plan](../../internal/plan)).

## Decision

- The config file's `[[exclude]]` entries are rules with the conditions of `[[select]]`, every one of which must hold. A pull request one of them matches (Yes) is left out of every tab. The entries are not put together otherwise: there is no expression over them, as ADR-026 has it for S. `packages` is not a condition of an exclusion, since it would read the body of every bot pull request in every tab.
- The inbox's settled notifications are left out as one more exclusion, unless `show_settled = true`; the setting keeps its name and now says that settled ones are not excluded.
- One key action, `show_excluded`, shows and hides what is left out, in any tab, the settled ones in the inbox among them. It was `show_settled`, which the keymap reads as a renamed action (ADR-023). Showing them keeps the cursor on its pull request. The rows shown that way are marked as excluded, and say what excludes them.
- A pull request that a rule cannot tell yet (Pending) is shown, and leaves once the rule says Yes. A rule that waits is never a reason to hide: a read that fails would hide the row for good.
- What decides whether a row is shown is one question, asked by every place that acts on the rows shown (ADR-027): put away, excluded while the tab does not show the excluded, the filter typed and the rules the tab is narrowed to. Selecting a group by its header, S, done on a selection (ADR-044) and the rows given to a triage ask it. A selection made before a pull request was excluded stays, as one the filter typed hides does; approve and merge still list it in their confirm popup.
- An excluded notification's unread mark is not counted in the inbox's count while it is left out, as a settled one was not.
- This replaces, in ADR-005, "The inbox hides those whose pull request is merged or closed (settled) until a key shows them, or from the start with `show_settled = true`." with the second sentence above; in ADR-016, "as is its key action" for `show_settled`, the action being `show_excluded`; and in ADR-044, "in the inbox not left out as settled" with "not excluded, the settled ones of the inbox among them".
- Rejected: marking exclusion rules among the `[[select]]` entries. S lists those entries, and would offer the rules meant to hide as rules to select by.
- Rejected: hiding a row until a rule that waits can tell. The row comes back when the read arrives, but not when it fails.
- Rejected: dropping the excluded from a selection. It would differ from a selection the filter hides, and a selection would shrink without a key.

## Consequences

- A rule left with no condition would match every pull request and leave every tab empty; the config file reports it, as for `[[select]]`.
- A rule on the CI, the size, the review decision or the labels lets a row show until they are read, and again after each push until they are read for the new head.
- A new place that acts on the rows shown must ask the one question; testing the filter and the hidden ones apart from it leaves the excluded out of sight but in reach.
- A new condition of a rule must say whether it is told with the list, after it, or not at all yet, so that Judge can say Pending for it.
