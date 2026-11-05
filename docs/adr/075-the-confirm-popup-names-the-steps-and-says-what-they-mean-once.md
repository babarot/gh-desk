# ADR-075: The confirm popup names the steps and says what they mean once

## Status

Accepted

## Context

A merge takes several steps on its way: marking a draft ready (ADR-046), approving it (ADR-006), updating the branch, waiting for the jobs (ADR-068), merging, as an admin when it must (ADR-048). The confirm popup said them as two kinds of text joined on one line: the first steps as words joined by arrows, the rest as a sentence ("ready → wait for checks, then merge"). Where one step ended and the next began had to be worked out.

Under each pull request the popup listed notes marked as warnings. Some were facts of that pull request: files marked for attention, a check that is not required and failed. Others said what a step means, and were the same under every pull request that took the step: that a draft stays ready if the merge stops, that an admin merge goes past the rules of the base branch, that auto-merge takes commits pushed later. Five drafts merged at once showed the same sentence five times, and a pull request with nothing wrong with it showed a warning.

## Decision

This replaces, in ADR-048, "The confirm popup says that it merges as an admin, past the rules of the base branch, before anything else it says of the pull request, including when it first waits or is a draft."

- What a merge does to a pull request is said as its steps, each named, joined in the order they are taken. The same words are used wherever gh-desk says how it will merge a pull request, not only in the popup.
- The merge method is named only when it is not the first of the user's order of preference, which is when the repository does not allow that one. The first is what the user expects; naming it on every pull request is the same words repeated, and hides the one that differs. Which method is usual is the user's setting, not a method gh-desk assumes most people use.
- A merge as an admin is named in the steps of each pull request it applies to, including when it first waits or is a draft, so which pull requests go past the rules can be told one by one.
- Under a pull request the popup says only what is true of that pull request.
- What a step means is said once, below the list, when any pull request that goes takes the step: what marking a draft ready leaves behind, that a merge as an admin goes past the rules of the base branch, what auto-merge takes later. ADR-046 and ADR-006 still hold that the popup says these; ADR-048's warning of an admin merge is still said, once below the list instead of under each pull request.

## Alternatives

- Rejected: one step per line under each pull request. It reads well for one pull request, but a batch takes a line per step for each, and the popup no longer fits the screen. Showing one pull request one way and a batch another makes the user read the steps differently from one popup to the next.
- Rejected: keeping what a step means as a note under each pull request. It repeats with the batch, and a warning under a pull request then no longer tells that something about that pull request needs care.

## Consequences

- A new step a merge can take needs a name in the steps and, when it leaves something the user must know, a sentence below the list, not a note of the pull request.
- A note of a pull request must be something that differs from one pull request to the next.
