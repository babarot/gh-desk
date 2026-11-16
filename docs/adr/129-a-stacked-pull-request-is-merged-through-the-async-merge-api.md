# ADR-129: A stacked pull request is merged through the async merge API

## Status

Accepted

## Context

GitHub's stacked pull requests are a chain of pull requests in one repository, each based on the head branch of the one below it, the lowest based on the stack's base branch. GitHub refuses to merge a pull request of a stack through the synchronous REST endpoint and the GraphQL mutations gh-desk used (`mergePullRequest`, `enqueuePullRequest`): it answers that the pull request must be merged through the asynchronous merge REST API. Every merge of a stacked pull request failed, and ADR-128 then stopped the pull requests above it.

The asynchronous API takes a merge request and answers at once, with the result, or with an ID to poll while GitHub merges in the background, which can take minutes. It takes the expected head, the method, whether to merge directly or through the merge queue, and whether to bypass the rules the viewer may bypass. Asked for a pull request of a stack, it merges, atomically, that one and every open one below it; the ones above stay open and are retargeted onto the stack's base branch. GitHub documents no auto-merge for a stack. A pull request read through GraphQL says whether it is in a stack and its position there.

GitHub calls the asynchronous API the recommended way to merge any pull request, not only a stacked one.

## Decision

- A pull request that is in a stack is merged, or put in the merge queue, through the asynchronous merge API. Every other pull request is merged as before.
- gh-desk merges a stacked pull request only when it is the lowest open one of its stack, so that a merge writes nothing but the pull request the user confirmed. One higher up is blocked by GitHub until those below are merged, and waits for them or stops as ADR-104 and ADR-128 say. Confirming a whole stack merges it from the bottom, one pull request at a time, each retargeted by GitHub before its turn.
- The request carries the confirmed head, as every merge does (ADR-006), and bypasses the rules exactly when gh-desk would merge as an admin otherwise (ADR-048).
- From the request until GitHub gives its result, the merge holds its repository's lane (ADR-104), as the write it is. Its result is read as the result of a merge written at once: merged, in the merge queue (watched from then on), or failed with what GitHub said.
- The request is GitHub's once sent: a stop by the user (ADR-122) does not cancel it, and gh-desk follows it to its result. The time it follows it is bounded by a constant, since the merge is GitHub's to finish; past it, or when gh-desk quits meanwhile, the record of the pending merge stays, with the ID of the request, and the next launch follows that request to its result, holding the lane. Only when GitHub no longer knows the request, which it keeps for a limited time, is the pull request read again to tell how it ended.
- When GitHub answers that a request asked before is still pending without naming it (gh-desk quit between asking and recording the ID), the merge is asked again after a while, until GitHub takes it or says why it cannot. Asking again is safe because every request carries the confirmed head: it can merge nothing but what the user confirmed, and it brings back the reason a request that failed did not merge.
- A stacked pull request is not put on auto-merge; where auto-merge would be used, it waits as one in a repository without auto-merge.
- Rejected: merging every pull request through the asynchronous API. It is the way GitHub recommends, but it changes the write of every merge gh-desk makes, the errors it reads and how long a merge takes to be known, for no gain on a pull request that is not stacked. It can be decided on its own later.
- Rejected: merging a stack in one request, from the highest pull request confirmed. It writes the pull requests below it whether or not they were confirmed, and what a confirm shows would have to cover them.

## Consequences

- A merge of a stacked pull request is known only after GitHub reports it, which keeps the lane of its repository for that time.
- Merging a stack of n pull requests takes n requests, each waiting for the jobs GitHub runs on the pull request once it is retargeted.
- A new write that merges must say which way it goes for a stacked pull request.
- If GitHub later accepts auto-merge on a stack, this ADR is what keeps it off.
- The record of pending merges carries the ID of a request in flight: a change to that record must keep it, or a merge in flight at quit can no longer be told failed from pending.
- [mergeasync_test.go](../../internal/runner/mergeasync_test.go) checks that a stacked pull request is merged through the asynchronous API and followed to its result, that one higher up a stack is not merged alone, as an admin either, that a stop and a quit leave the request to GitHub, and that a resume follows it by its ID.
