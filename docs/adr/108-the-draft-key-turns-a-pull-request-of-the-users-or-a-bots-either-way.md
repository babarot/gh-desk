# ADR-108: The draft key turns a pull request of the user's or of a bot either way

## Status

Accepted

## Context

The key of ready for review marked a draft ready, and on a pull request that was not a draft it did nothing but say "not a draft". Turning a pull request back into a draft, when the user finds it was opened too early, took the browser.

ADR-046 left ready for review on its own as it was, for any draft, including another person's. A draft says its author does not want it reviewed yet; marking another person's draft ready asks for reviews in their name, which is why ADR-046 has merge leave such a draft. Turning another person's pull request into a draft overrides them the same way.

GitHub's mutations for both ways take no head to check.

## Decision

- One action, `draft`, turns a pull request either way: a draft is marked ready for review, a pull request that is ready is turned back into a draft. It replaces `ready_for_review`; the old name in `[keys]` or `menu.actions` is a mistake that names the new one, as for other renamed actions (ADR-023).
- It acts only on a pull request of the user's or of a bot, in both ways. Another person's is skipped and says why; on a selection the rest still go.
- Which way is decided from what the list shows, before the popup: when the pull requests it would act on are all drafts it marks them ready, when none is it turns them into drafts, and when they are both it refuses and says so. It does not turn each its own way: a user who meant one way would find some turned the other.
- Another person's pull request is left out before the way is decided, so it neither decides the way nor makes a selection mixed.
- The confirm popup names the way. The runner reads each pull request just before it writes, skips one that is already the way it would turn it, and stops if the head moved, as for any write.

This replaces "Ready for review on its own stays as it was, for drafts that are only to be marked ready, including another person's." in the Decision of ADR-046.

## Alternatives

- Rejected: a second action and key for turning into a draft. Each would only refuse on what the other acts on, and the key that did nothing on a pull request that is ready is the one the user reaches for.
- Rejected: keeping the name `ready_for_review` for both ways. The name is what a config file and the log say; it would name only one of them.
- Rejected: turning each pull request of a mixed selection its own way. A selection is one intent; flipping each hides which way each went until the list is read again.

## Consequences

- A user who marked other people's drafts ready with this key can no longer; ready for review on another person's draft is for its author.
- A config file that names `ready_for_review` is reported until it is renamed.
- Whose pull request it is rests on its author and whether GitHub calls it a bot, as in ADR-046.
