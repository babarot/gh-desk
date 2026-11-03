# ADR-047: Merge takes the user's own blocked pull request as an admin

## Status

Accepted

## Context

A repository that requires an approval blocks the pull requests of its owner as well, and GitHub does not let anyone approve their own pull request. In a repository with one maintainer nobody else is there to approve it. GitHub lets an admin merge past the rules of the base branch (`gh pr merge --admin`) when the rules allow it: branch protection that does not include administrators, or a ruleset with the user among its bypass actors. GitHub tells whether the viewer can, per pull request, as `viewerCanMergeAsAdmin`.

ADR-006 has merge wait for a blocked pull request, or put it on auto-merge, until `merge.blocked_timeout` gives it up as blocked by another rule. For the user's own pull request blocked by a required review, that wait never ends in a merge: auto-merge waits for the same review.

An admin merge is the same `mergePullRequest` as any other, with the confirmed head. It goes past every rule of the base branch, required checks included, not only the review.

## Decision

- Merge on the user's own pull request that GitHub calls blocked, and that GitHub says the user can merge as an admin, merges it as an admin. Once the user has said yes, it needs no other step or key.
- It is merged only after its required checks are known to have finished without failing, as gh-desk reads them (ADR-006). Before that it waits for them; an approval check is not waited for, since it waits for the review the merge goes past. Without the checks read, it waits. When more checks are reported than gh-desk read, it is not merged as an admin at all and is decided as ADR-006 says: a check not read may be failing.
- It is not put on auto-merge, even where the repository allows auto-merge: auto-merge would wait for the review. A draft of the user's that the user can merge as an admin waits after it is marked ready (ADR-046), for the same reason, and is decided again.
- The confirm popup says that it merges as an admin, past the rules of the base branch, before anything else it says of the pull request, including when it first waits or is a draft. The merge is logged as an admin merge.
- Another person's pull request, a bot's, and one the user cannot merge as an admin are decided as ADR-006 says: approved when the user has to, then waited for or put on auto-merge.

## Alternatives

- Rejected: merging as an admin whenever GitHub allows it, whoever wrote the pull request. Another person's pull request has the user as its reviewer; going past the review there merges what nobody approved, and the approval the user sends first already settles the review rule.
- Rejected: a setting to turn admin merge on. The user's own pull request has no other way to be merged from gh-desk, so the setting would only be the one value.
- Rejected: a separate key for an admin merge. Which pull requests need one is GitHub's to say, and a batch of mixed pull requests would have to be split by hand.

## Consequences

- An admin merge goes past every rule of the base branch, not only the review: conversation resolution, signed commits and the like. The popup's note is the only warning.
- Which checks are required comes from `isRequired` of the checks reported on the head. A required check that has not reported yet is not seen, so an admin merge right after a push may go before it.
- Whether the user wrote the pull request rests on its author, as for drafts (ADR-046).

## Not decided here

- Whether to merge another person's pull request as an admin when it stays blocked after the user approved it (code owners, a second approval).
