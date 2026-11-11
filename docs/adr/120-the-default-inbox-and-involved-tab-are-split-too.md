# ADR-120: The default inbox and involved tab are split too

## Status

Accepted

## Context

ADR-118 kept the default tabs as they looked before sections: the review tab and the tab of one's own pull requests split, the involved tab and the inbox not. It was written while sections were new, so that a file with no `[[tabs]]` would not change under its user.

Using sections, the user found the inbox and the involved tab read better split, and that a default split is easy to drop: on the settings page a section is removed with one click, and a file that writes its tabs has them as written. The release that brings sections changes the default look already, so a change of the defaults now costs its users no second change.

## Decision

- The default inbox is split by what a notification asks of the user: `review requests` (reason `review_requested`), `mentions` (reason `mention`), and the rest in other.
- The default involved tab has the bots' pull requests apart and folded, under the rest, which have no head, as the review tab has the bots.
- This replaces, in ADR-118, "The default tabs look as they did: the review tab in the four sections of the groups, the team's and the bots' folded; the tab of one's own pull requests in two sections with no name, drafts after the rest." with: the review tab and the tab of one's own pull requests are as that sentence has them, and the inbox and the involved tab as above.
- It also replaces, in ADR-118, "ADR-015's rule that a head states only a fact of the pull request holds of the default tabs, not of what the user writes." with: a head of a default tab states only a fact of its rows, which in the inbox include the reason of the notification; it holds of the default tabs, not of what the user writes.
- Rejected: splitting the involved tab by a reason. Its rows are what is left once the review requests and one's own pull requests are taken out, and no fact of them says what to do with them.

## Consequences

- A file with no `[[tabs]]` shows heads in the inbox and the involved tab. Dropping them means writing the tabs, which the settings page does when one of their sections is removed.
- The inbox can start with a head; its cursor starts on its first notification, as a search tab's does, also when it is shown from the lists last read. What acts on a section's rows in the inbox reads them from its notifications, not from a search.
