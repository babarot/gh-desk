# ADR-046: Merge marks a draft of the user's or of a bot ready first

## Status

Accepted
Partly replaced by ADR-108

## Context

ADR-006 has merge skip a draft. To merge one, the user marked it ready (`ready_for_review`), waited for the list to show it, then merged it: two confirm popups for what the user meant as one thing. A merge already takes the steps it needs on its way (an approval, updating the branch, waiting for checks); being a draft was the one step it refused to take.

A draft says its author does not want it merged yet. When the user is the author, merging it says otherwise. A bot opens drafts by its settings (renovate's `draftPR`, to hold off CI or review), not because a person is still working on them, and the user merging a bot's pull request is who decides. Another person's draft is that person's to mark ready.

Marking a draft ready has effects outside gh-desk that cannot be taken back: GitHub asks for its reviews, code owners among them, and workflows that run on `ready_for_review` start. Its checks start too, so a pull request just marked ready is rarely mergeable at once. GitHub's mutation that marks it ready takes no head to check.

## Decision

- Merge on a draft of the user's or of a bot marks it ready for review, then approves it when the user has to (ADR-006), then merges it as any other. A draft of another person is skipped, and says so.
- The confirm popup says that it will be marked ready, and that it stays ready if the merge stops after that. Marking it ready is not undone when a later step stops.
- Until it is marked ready, a draft is decided as one that waits for its checks: put on auto-merge where the repository allows it, waited for otherwise. Whether it has conflicts is not known until then. After it is marked ready it is read and decided again, as after an approval.
- Marking it ready goes on the head the user saw: the runner reads the pull request just before and stops if the head moved, as for any write.
- Marking it ready is recorded before it is sent, as an approval is. A pull request gh-desk marked ready that is a draft again, in the same run or at the next launch, stops with a reason instead of being marked ready again: someone turned it back, and marking it again would undo them.
- Ready for review on its own stays as it was, for drafts that are only to be marked ready, including another person's.

This replaces "draft" in the list of what `internal/plan` skips in the Decision of ADR-006.

## Alternatives

- Rejected: marking any draft ready. It overrides another person's word that their pull request is not ready, and asks for reviews in their name.
- Rejected: keeping merge from drafts and having the menu offer ready and merge together. The steps a merge takes are the runner's to decide again before each write; a second batch started after the first could not be.

## Consequences

- A merge stopped after marking a draft ready leaves it ready, with its reviews asked for; the popup's note is the only warning.
- A merge of a draft of the user's or of a bot always waits after its first write, so it needs the record of pending merges (ADR-009): locked by another gh-desk, it is skipped before anything is sent.
- Whose draft is marked ready rests on the author of the pull request and whether GitHub calls it a bot. A person's account that acts as a bot without GitHub saying so is treated as a person.
