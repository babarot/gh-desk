# ADR-093: An auto-approve rule is the user's intent written ahead, and approves without a confirm

## Status

Accepted

## Context

Some pull requests are approved without being read: a dependency update of a known kind, from a bot, in a repository whose CI the user trusts. gh-desk still had the user select each one and confirm it, and a repository's auto-merge, which would merge it once approved, waited for that approval.

gh-desk writes nothing without a confirm (ADR-006): the popup reads each pull request again and says what will be done, so that nothing is written that the user did not mean. Its purpose is to keep out writes the user did not intend. A rule the user writes in the config file, saying which pull requests to approve, states that intent before the pull request exists. Confirming each match would ask again what the rule already says.

An approval sent by a rule has no head the user saw. What stands in for it is the head the rule judged. Between the judgment and the write the head can move, a job can start or fail, a person can dismiss the approval, turn the pull request back into a draft or label it to be left alone.

A rule's `checks = ["passing"]` holds for a head with no checks reported yet ([internal/plan](../../internal/plan)). A bot that pushes a new head is followed by its workflow runs only seconds later, which ADR-068 met as a merge that went through before CI started.

What a rule approves could be bounded by gh-desk: only bots, only pull requests whose every commit is the bot's. Who may approve what, and whether a push by someone else needs a new approval, are already the repository's to decide, through its branch protection and rulesets.

## Decision

- `[[auto_approve]]` entries in the config file are rules that approve the pull requests they match, without a confirm popup, while gh-desk runs. This replaces, for these approvals, "A batch is started from one confirm popup, for the selection or the pull request under the cursor." in ADR-006.
- A rule is the user's intent. Which pull requests it approves is the responsibility of the user who wrote it, and of the repository's protections. gh-desk does not require a rule to name its authors or to be limited to bots, and does not check who pushed to a pull request's branch.
- gh-desk is responsible for approving what the rule judged, and nothing else:
  - The approval carries the head the rule judged. This replaces, for these approvals, "the confirmed head" in ADR-006's "Every write carries the confirmed head". A head that moved after the judgment is judged again, as a new head, later.
  - A pull request that is not open, is a draft, is the viewer's own, or is already approved by the viewer is not approved. Neither is one whose latest review by the viewer was dismissed on the head it has now: a person took that approval back. An approval dismissed by a push is on an older head, and the new head is judged again.
  - It waits for the jobs at work on the head before it approves, as an approval may (ADR-069), and stops when one fails. A rule's `passing` includes no checks reported yet, so approving at once could approve a head CI never ran on.
  - A pull request that an `[[exclude]]` rule matches, or cannot tell yet, is not approved. An exclusion is written to leave a pull request alone, whatever it is hidden for (ADR-062), and it is how a rule's "not" is written: auto-approve rules, like every rule, have no operator for it (ADR-065).
  - The draft, the dismissal and the exclusion are judged again right before the approval is sent, after a wait, since they can change while the head stays.
  - A head is tried once a session, so a failure is not repeated at every reading.
- A rule reads its own search, apart from the tabs: what is shown or read in a tab does not change what is approved. A rule may give that search; without one, gh-desk searches for the open pull requests that are not drafts and whose review is requested of the viewer, narrowed by the rule's repositories only when each of them can be written as a search qualifier. A narrowing that drops what a rule's condition would match is not taken. A rule with neither a search nor a condition is reported as a mistake: it would approve every review requested of the user.
- A rule can be set to only record what it would approve, without approving it.
- Rules are read at the automatic refresh and at `r`; with the refresh off, at launch and at `r`. Only the gh-desk that holds the lock on the record of pending merges reads them. This adds auto-approve to what a second gh-desk does not do, in ADR-009's "A second gh-desk is fully usable except for merges that would wait."
- What is approved, or would be, is told in the in-app notifications and logged as every write is (ADR-003, ADR-007).
- The settings are in [internal/config](../../internal/config).
- Rejected: approving through GitHub Actions or a bot's own approving app. The approval would not be the user's, an organization may not let Actions approve, and it would run when the user is not there.
- Rejected: requiring authors, or only bots, and checking that every commit is the bot's. They decide what the user may intend, and the repository already decides who may push and what needs approving again.
- Rejected: approving at once. A head with no checks reported yet would pass.
- Rejected: approving again on the same head after a person dismissed the approval. gh-desk would undo the person's decision at every reading.
- Rejected: judging the pull requests read by the tabs. A new pull request in a tab not read since launch would wait until the tab is opened.
- Rejected: writing the exclusions a rule respects into each rule, by name. It would bring an operator over rules into the file, and a rule that forgot one would approve what is left alone.
- Rejected: a refresh interval of its own for auto-approve. It would be a second clock and a setting nobody would tune.

## Consequences

- An `[[exclude]]` rule now also keeps pull requests from being approved by rules. A user who wants a pull request hidden but approved has no way to say so yet.
- A new condition of a rule must say whether it can be written as a search qualifier without dropping what it would match, before it narrows the default search.
- The record of pending approvals tells an approval made by a rule from one the user confirmed. A pending approval a rule asked for is not resumed: it is forgotten at start, and reading the rules finds it again if it still matches, so a rule taken out of the file stops what it had started.
- How many pull requests a rule's search reads is a constant; a search that matches more is told once a session and asked to be narrowed.
