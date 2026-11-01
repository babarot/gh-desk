# ADR-016: The config file is shaped by what it controls

## Status

Accepted

## Context

The config file has to say, in places, which comes first: the tabs, the groups of a tab, the list's columns. TOML and YAML define the keys of a table as unordered, and TOML lets one table be written in pieces (dotted keys, or `[a.b]` apart from `[a]`), so the order of a table's keys may not be one order at all. A tool that reads its order from key order, as babarot/enter does from a YAML mapping, holds only as long as the parser keeps it.

A setting is found by the feature or the screen it changes. A table named after a quality rather than a feature ("ui", how it looks) does not say what belongs in it, and a value that more than one feature uses is not found under one of them.

A zero value that means "the default", or "off", cannot be told from one written on purpose, and reads as a value only the comment explains (`limit = 0` for 100, `interval = "0"` for no refresh).

## Decision

Rules for the config file:

- Order is expressed only by arrays (`[...]` and arrays of tables `[[...]]`). The order of the keys in a table means nothing.
- A table is made per feature or per screen, whatever the number of keys it holds. A value used by more than one feature gets a table of its own rather than one of those features'.
- Names are snake_case. A setting holding a list has a plural name. Enum values are lowercase English words. A duration is a Go duration string, and `"off"` turns off what it times. A setting and the key action for the same feature have the same name.
- A zero value has no meaning of its own. A key left out takes its default; `0` or `""` written in the file is read as that value and checked as such, never as "use the default". A setting that can be off says so with a value (`"off"`), not with 0.
- A key that is moved or renamed stays in a table of old names, so that a file that still has it is told at that line where it went. The old name is not read.

Where the settings sit:

- `[[tabs]]` are the tabs (ADR-005), with their groups (ADR-015); `[[select]]` are the rules S offers.
- `[list]` is the list: how often it is read again (`refresh`), when AGE is marked (`stale_days`), the owners left out of the PR column (`home_owners`), and its columns (ADR-017).
- `[preview]` is the pane under the list. `[description]` is how a description is drawn, in the pane and on the PR screen. `[diff]` is how diffs are shown.
- `[checks]` holds `approval`, the checks that pass only after an approval: the CI column and deciding a merge both use them.
- `[review]` is sending a review, from any tab: the editor it is written in (`editor`), and marking the notification done after it (`mark_done`).
- `[merge]` and `[bulk]` are merging and the bulk popup. `[claude]` is asking Claude. `[theme]`, `[log]`, `[cache]` and `[keys]` are the colors, the log file, the cache directory and the keys.
- The inbox's setting to show the notifications whose pull request is merged or closed is `show_settled`, as is its key action: settled covers both. `done`, `hidden` and `stale` are not used for it, since they already mean marking a notification done, putting a row away with `d`, and the AGE mark.

## Consequences

- New settings follow these rules, and a change to a rule is a new ADR.
- A table can hold one key, as `[theme]` and `[cache]` do; it is there for what it is about.
