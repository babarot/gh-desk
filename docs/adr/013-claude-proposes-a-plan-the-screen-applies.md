# ADR-013: Claude proposes a plan; the screen applies it

## Status

Accepted

## Context

Asking Claude about one pull request (ADR-010) helps once a pull request is open. In front of the lists, the question is a different one: which of these to look at, select, approve or merge, under conditions that take several keys and some judgment to apply ("the bots' updates whose checks pass and whose diffs are small"). Claude can judge such conditions from what the lists already hold, but letting it drive the screen raises three questions: how it acts on the screen, what keeps it from acting on what it should not, and what happens to a write.

Claude Code can call tools while it answers, and gh-triage could offer screen operations as MCP tools. An MCP server, though, is a process Claude Code starts, so moving the TUI from it needs a channel back into the TUI; and tools run while Claude is still thinking, so the screen would move under the user before the user has seen what Claude means to do. The titles, authors and labels in the lists are written by others, so whatever Claude proposes may have been steered by them.

## Decision

- `C` on the list opens a triage popup; `C` on a pull request's screen keeps the conversation of ADR-010 about that pull request.
- A triage runs `claude -p` with no tools, under the restrictions of ADR-010, in the cache directory, in a session kept for as long as gh-triage runs. Each request carries the lists as the screen shows them: every tab's rows, with each pull request's key and head, and the cursor, filter and pane. A field not read yet for the head shown (checks, review decision, size, labels) is given as `unknown`, not as an empty value, so a pull request that has not been read cannot pass for a small one.
- Claude answers, through `--json-schema`, with a reply and a plan of actions from a fixed set: `tab`, `filter`, `fold`, `unfold`, `select`, `clear_selection`, `cursor`, `pane`, `approve`, `merge`. It does not act on the screen itself. What the set cannot do (sorting, commenting, anything the lists do not show) Claude says in its reply.
- The TUI checks the plan against the lists each time it shows it and again when it applies it, action by action, on the tab and filter the actions before it leave. A pull request Claude was not given, that has left the lists, or whose head has moved since Claude was given it is left out, and the plan shows which and why. The plan is applied only when the user presses `triage_apply` (enter).
- `approve` and `merge` act only on the pull requests they name, never on the selection or the cursor, and only as a plan's last action. They open the usual confirm popup with the pull requests as Claude saw them, so one pushed to since is caught there as moved; nothing is sent until the user confirms. The popup says that Claude picked them, for which request. Approving your own pull request is left out; merging it is not.
- A selection records who made it, and rows Claude selected are drawn on a background of their own.
- Reading GitHub beyond the lists (searching, a pull request's description or files) would be offered to Claude as read-only MCP tools, and is not part of this decision.

## Consequences

- A request is as slow as a run of `claude -p`, several seconds at least; moving around the lists stays faster with keys. The triage is for requests that combine conditions or need judgment.
- Claude can only propose what the action set covers. A new kind of action needs its own check in the TUI, and an action that writes needs the confirm popup.
- Claude sees only what the lists hold; it cannot answer about a pull request's files or description until read-only tools are added.
- Text in the lists can still steer Claude's plan. It cannot write anything by itself: the user reads the plan before applying it, and confirms any write in the confirm popup.
- Each request sends up to 300 rows of the lists; a longer list is cut, and Claude is told so.
