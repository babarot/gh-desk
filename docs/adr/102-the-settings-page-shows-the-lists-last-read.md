# ADR-102: The settings page shows the lists last read, and gh-desk judges its rules

## Status

Accepted

## Context

ADR-100 draws made-up pull requests on the settings page. A rule is hard to write against someone else's pull requests: what a user wants to know is what `[[select]]` or `[[exclude]]` does to their own. The page also matched rules in JavaScript, a second copy of `plan.Rule` that could only drift from it.

gh-desk already keeps what it last read in `lists.json`, for the next launch.

## Decision

- The preview draws the lists in `lists.json`, read once when the command starts, with the time they were read and the account. A `config` command does not reach GitHub, so nothing is read from it, the viewer included.
- The viewer is the one in the file, which is not checked. `listcache.Open` refuses that for good reason, as the TUI acts in the viewer's name. The page acts in nobody's: it only draws and counts, and it says whose lists they are.
- A tab finds its list by its search. A tab with none says it was not read, and is not filled with made-up rows, which would read as a result. Made-up pull requests remain for when nothing was read at all, and the page says so.
- Rules are judged only in Go. The page sends the file as it stands to a `judge` route, under the same token, `Host` and `Origin` as saving, and gets back which rows each rule says yes of, and which it cannot tell. The route writes nothing.
- A rule is judged the way gh-desk uses it. `[[select]]` goes through `plan.Selectable`, as `S` does, since `Judge` alone would count drafts and merged pull requests that `S` leaves out. `[[exclude]]` and `[[auto_approve]]` use `Judge`.
- What cannot be told shows as `?`, and is not counted as a match. The body is not kept with the lists and never goes to the page, so `packages` is always `?`.
- The page gets what it draws and no more: never a body, a comment or a diff.
- Rejected: reading GitHub at start. It would make `config` need a network and a token, for a list that is minutes old at most.
- Rejected: keeping the JavaScript matcher for speed. A count that differs from `S` is worse than one that arrives 200ms later.

## Consequences

- Titles of private repositories are in the page. It is served to this machine only, with `no-store`, and a Content-Security-Policy that sends nothing elsewhere.
- The preview is as old as the last run of gh-desk.
- The count of an `[[auto_approve]]` rule is of the tab shown, not of what it would approve, as the rule searches on its own.
- If the viewer changed since the lists were written, rules on `mine` and the like are judged for the old account until gh-desk runs again. The page names it.
