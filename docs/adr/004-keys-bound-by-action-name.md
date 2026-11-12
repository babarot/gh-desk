# ADR-004: Keys are bound by action name and configurable

## Status

Accepted
Partly replaced by ADR-023
Partly replaced by ADR-125

## Context

gh-desk has over fifty actions across its screens (the list, the preview pane, the pull request screen, confirm popups, the Claude screen, review writing). Several defaults are near each other and easy to confuse (`c` comments, `C` asks Claude), and irreversible actions use capital letters (`A` approve, `M` merge, `R` review). Users will want to change some of them, and a key written into a view or an update function cannot be changed without editing code; it also tends to drift from the key hints and the help screen.

## Decision

- `internal/keymap` defines every action by name with its scope and default keys (`Defaults`). The TUI asks whether a key press matches an action (`Matches("approve", key)`), never compares key strings itself, and builds its key hints and its `?` list from the keymap.
- `[keys]` in the config file sets an action's keys by name: a key, a list, or `[]` to turn it off. A key set for an action is taken from any action that has it by default where they meet.
- Scopes decide where keys meet: `global` and `nav` keys are read everywhere, so they may not be reused; other scopes only share their own keys.
- Every mistake is reported where it is in the config file, and the TUI does not start: an unknown action, a key no key press is read as (`shift+a` instead of `A`, `ctrl-s` instead of `ctrl+s`), a fixed key (`ctrl+c`, `esc`), or two actions set to one key where they meet.
- The `[keys]` part of the config file written on first run is generated from `Defaults`, so it always lists every action with its current keys.

## Consequences

- Adding an action means adding it to `Defaults`; a test fails if its default keys conflict with another action's.
- The key hints and help are always true to the bindings.
- Scopes have to be kept accurate: an action placed in the wrong scope could let two keys meet without a conflict being reported.
