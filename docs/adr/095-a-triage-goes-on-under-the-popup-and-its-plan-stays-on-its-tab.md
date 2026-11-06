# ADR-095: A triage goes on under the popup, and its plan stays on its tab

## Status

Accepted

## Context

A triage (ADR-018) could only be waited for or stopped. `esc` while Claude thought stopped the request, forgot it, and closed the popup at once, while the conversation with Claude about a pull request keeps a stopped question and can be left while Claude goes on (ADR-040). The popup also held the screen for as long as a request took.

Letting the popup close during a request brings three problems the popup had hidden by never closing. Opening it again went straight to the text box, so a second request could be sent over the first. Stopping marked the request as over at once, while Claude Code takes a moment to stop, so a request sent right after would resume a session another process still holds. And a plan is checked against the tab shown, not the tab it was asked on, so a plan answered while the user was elsewhere could be applied to another tab, where a pull request in both would pass every check.

## Decision

- `esc` during a request stops it and keeps it in the conversation, marked as stopped, with the popup open. `chat_close` closes the popup and Claude goes on.
- A request is on its way until its end comes back. While it is stopping nothing can be sent. A session Claude Code had not taken by then is replaced, as a session ID is taken once.
- Opening the popup shows what stands: a request on its way, or a plan that came while it was closed. Only otherwise does it go to the text box. Sending refuses as well while a request is on its way.
- A plan is applied only on the tab its request was about. On another tab the popup names that tab and offers no way to apply. This holds beside ADR-018's checks: they tell whether each action still applies to a tab, not whether it is the tab asked about.
- `chat_restart`, pressed twice, starts the conversation over in a new session, as in the claude pane.

## Consequences

- After a stop, the next request can wait a few seconds for Claude Code to end.
- A plan that came while the popup was closed stays until it is applied, closed or replaced.
- The tab of a plan is kept by its place among the tabs, which do not change while gh-desk runs.
