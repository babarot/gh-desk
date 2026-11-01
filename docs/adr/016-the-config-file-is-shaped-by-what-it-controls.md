# ADR-016: The config file is shaped by what it controls

## Status

Accepted

## Context

The config file grew one setting at a time, and its shape followed the order the settings were added in rather than what they control:

- Order is wanted in more places than the file can express. The tabs are ordered by `[[sections]]`, but the inbox is always appended after them (`internal/tui/tui.go`), and `internal/tui` assumes it is the last tab (`inboxTab()` returns `m.tabs[len(m.tabs)-1]`). The groups of a grouped tab are always you, team, other, bots (ADR-015), and the list's columns are fixed in `columns()`.
- The names disagree with each other. The key is `sections`, while the file's comments, the type's comments and the screen call them tabs. The inbox's setting is `hide_closed`, while the key that toggles the same thing is the action `show_closed`, and the TUI keeps it as `showClosed`.
- Tables hold settings that belong elsewhere. `[ui]` holds `approval_checks`, which deciding a merge uses as well as the CI column, against ADR-014's rule that a value used by more than one feature gets a section of its own. `preview.description` also decides how the PR screen draws a description. ADR-014 says one key does not make a section worth having, yet `[refresh]`, `[diff]`, `[bulk]`, `[log]` and `[cache]` each have one key, and `[cache]` was made by ADR-014 itself.
- Zero values carry meaning. A tab's `limit = 0` reads as 100 and `sort = ""` as updated, so writing 0 and leaving the key out cannot be told apart, and `refresh.interval = "0"` turns automatic refresh off, which only the comment says.

The enter tool (babarot/enter) orders its display by the order of the keys of a YAML mapping. That order is not part of either format: YAML and TOML define the keys of a table as unordered, and TOML lets one table be written in pieces (dotted keys, or `[a.b]` apart from `[a]`), so the order of its keys may not be one order at all. It holds only as long as the parser keeps it.

gh-triage has no users yet, so the file can be reshaped as if it had always been this way, without reading the old names.

## Decision

Rules for the config file:

- Order is expressed only by arrays (`[...]` and arrays of tables `[[...]]`). The order of the keys in a table means nothing.
- A table is made per feature or per screen. The number of keys it holds is no reason to make one or not; this replaces ADR-014's sentence that one key does not make a section worth having. ADR-014's rule that a value used by more than one feature gets a section of its own stands.
- Names are snake_case. A setting holding a list has a plural name. Enum values are lowercase English words. A duration is a Go duration string, and `"off"` turns off what it times. A setting and the key action for the same feature have the same name.
- A zero value has no meaning of its own. A key left out takes its default; `0` or `""` written in the file is read as that value and checked as such, never as "use the default". A setting that can be off says so with a value (`"off"`), not with 0.
- A key that is moved or renamed stays in a table of old names, so that a file that still has it is told at that line where it went. The old name is not read (as ADR-014 did for `claude.cache_dir`).

The shape this gives:

- `[[sections]]` becomes `[[tabs]]`, and the inbox is one of its elements, written `kind = "inbox"` (`kind` is `"search"` when left out). The inbox is a kind of tab, not a name: its `name` is free and defaults to `"inbox"`. There is exactly one inbox, since the TUI needs it (`knownPR()` reads it unconditionally, and the unread marks on every tab and marking a notification done come with reading it); none or two is an error. Making the inbox optional would mean reading notifications apart from any tab, which this does not do.
- A tab's `groups` lists the groups to show, in the order shown; writing it turns grouping on, so `group = "requested"` goes. `folded` names groups among `groups`. This replaces ADR-015's sentences that the groups are listed you, team, other, bots and that `group = "requested"` stays the one way to group. ADR-015's groups (what puts a pull request in each, and the labels) stand: `groups` chooses among them and orders them, and defining groups in the file stays rejected.
- The inbox's `hide_closed = true` becomes `show_settled = false`, and the key action `show_closed` becomes `show_settled`. Settled covers both merged and closed pull requests: the inbox hides the notifications whose pull request has either. `done`, `hidden` and `stale` are not used, since they already mean marking a notification done, putting a row away with `d`, and the AGE mark.
- `[ui]` goes, as "how it looks" did not say what belongs in it: `theme` becomes `[theme] name`, `stale_days` and `home_owners` go to `[list]` with `refresh` (from `[refresh] interval`), `editor` goes to `[review]`, and `approval_checks` becomes `[checks] approval`. `[inbox] done_after_review` becomes `[review] mark_done`, since it is what sending a review does, from any tab. `preview.description` becomes `[description] style`.
- `merge.checks_timeout` and `merge.blocked_timeout` stay settings: how long CI runs, and how long another person's approval takes or how soon a block that will not clear is given up on, differ between repositories and teams. `merge.watch_timeout` becomes a constant: the merge is GitHub's, and the value only decides how long gh-triage watches for the result.
- The keys that go to a tab by number are `tab_1` to `tab_9`. Only the numbers of tabs in the file act and are listed under `?`, and there are at most nine tabs.

## Consequences

- Every file written for gh-triage before this change has to be rewritten once; each old key is reported at its line with where it went.
- The inbox can be placed anywhere among the tabs. The places in `internal/tui` that take the last tab for the inbox find it instead.
- New settings follow these rules, and a change to a rule is a new ADR.
- Earlier ADRs name settings as they were when written. ADR-005's `[[sections]]`, `group = "requested"` and `[inbox] done_after_review` are now `[[tabs]]`, `groups` and `[review] mark_done`; ADR-013's `preview.description` is `description.style`; ADR-014's `ui.editor` is `review.editor`; and `watch_timeout`, which ADR-006 and ADR-009 name, is a constant of 24 hours.
