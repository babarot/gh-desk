# ADR-036: A conversation with Claude starts with what the user asks

## Status

Accepted

## Context

A new conversation about a pull request (ADR-010, ADR-031) asked Claude for a review as soon as it was opened. A review is the slowest and costliest question there is, and it was asked even when the user opened the conversation to ask something narrower, such as whether a change breaks its callers. The review was also what told Claude about the pull request: its description, comments, checks and diff, marked as data, went in with that first question only.

## Decision

- Opening a conversation with nothing asked asks nothing. The first question is the user's, whatever it is.
- A review is a question like any other, asked by an action of its own (`chat_review`), which works at any point in a conversation.
- What GitHub knows about the pull request goes before the first question of every session, as data and not as instructions (ADR-010), whatever that question is. It no longer carries a request for a review.
- The worktree is made as a conversation with nothing asked is opened, so the first question does not wait for a clone. It is made under the conversation's lock, as for a question, because the worktree is moved only under that lock (ADR-031). A question asked meanwhile takes the lock on, and waits for the worktree.
- Starting over (`chat_restart`) leaves a conversation with nothing asked, and frees the lock.
- This replaces, in ADR-031, "starts a new one with a review" in the sentence on `chat_restart`, "losing it costs a new review" (losing it costs a new session), and "its next question starts a new session with a review" (its next question starts a new session, with the pull request before it).
- Rejected: handing the worktree to Claude Code in the terminal instead. Claude would not know the description or the diff, and what was said there would not be kept in gh-desk, nor turned into a review to post.
- Rejected: a setting for the automatic review. One key gives the same review, and the setting would not be something that differs between users (ADR-014).

## Consequences

- A review takes a key it did not take before.
- Opening a conversation clones the repository and checks out the pull request even when nothing is then asked. The worktree goes when the pull request is closed (`gh desk gc`); the clone stays, as before.
- While the worktree is made, another gh-desk cannot ask about that pull request, and is told that Claude is working on it.
