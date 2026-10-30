# ADR-001: Build a new gh extension instead of extending gh-news

## Status

Accepted

## Context

The pull requests that reach a developer arrive in three ways that do not line up: review requests (to the developer and to every team they are on), their own pull requests, and GitHub notifications. In a busy organization most review requests go to teams, and most of the rest are opened by bots (renovate, dependabot, release bots). Notifications stay after the pull request is reviewed or merged, so the notification list is not a list of what is left to do.

[gh-news](https://github.com/chmouel/gh-news) is a good notifications TUI and was considered as a base, through a fork with patches. Two things it is not built for were needed:

- Showing each pull request's state (checks, reviews, whether it can be merged) next to the notifications, and acting on many pull requests at once: select bot pull requests, approve them, and merge them, following each repository's rules.
- Asking Claude Code about a pull request in a conversation inside the TUI, and turning the answer into a review the developer edits and posts. gh-news's custom actions run a command; they cannot take its output back, let the user edit it, and post it.

[gh-dash](https://github.com/dlvhdr/gh-dash) covers query-based sections of pull requests, but not notifications or bulk merging with waiting for checks.

## Decision

Write a new gh extension, gh-triage, in Go with Bubble Tea v2 and go-gh v2. Lists come from GitHub search queries; notifications mark the rows they concern and have a tab of their own.

## Consequences

- gh-triage owns its whole flow (listing, reviewing, bulk merging, Claude), so the parts can be designed together, at the cost of writing what gh-news and gh-dash already have (a list, a preview, key bindings).
- gh-news's useful ideas are taken over deliberately: showing the comment that triggered a notification, and a description view in the preview pane.
- The patches kept for gh-news are not carried over.
