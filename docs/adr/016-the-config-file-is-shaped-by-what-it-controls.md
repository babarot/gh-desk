# ADR-016: The config file is shaped by what it controls

## Status

Accepted
Partly replaced by ADR-029
Partly replaced by ADR-062

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

Where each setting sits is in the table of settings in `internal/config` (ADR-014). Three choices follow from the rules above and are kept:

- `[checks]` holds `approval`, the checks that pass only after an approval: the CI column and deciding a merge both use them, so neither `[list]` nor `[merge]` has it.
- `[review]` is sending a review from any tab, not a part of the inbox, though marking the notification done after it (`mark_done`) concerns notifications.
- The inbox's setting to show the notifications whose pull request is merged or closed is `show_settled`, as is its key action: settled covers both. `done`, `hidden` and `stale` are not used for it, since they already mean marking a notification done, putting a row away with `d`, and the AGE mark.

## Consequences

- New settings follow these rules, and a change to a rule is a new ADR.
- A table can hold one key, as `[theme]` and `[cache]` do; it is there for what it is about.
