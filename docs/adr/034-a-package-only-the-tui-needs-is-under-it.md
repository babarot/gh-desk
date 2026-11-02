# ADR-034: A package only the TUI needs is under it

## Status

Accepted

## Context

ADR-033 put the conversations with Claude in `internal/tui/chat`, a package under the TUI. Two packages were already apart from the TUI and needed only by it: the status line (ADR-025) and the keys bound to actions (ADR-004). They sat at the top of `internal/`, beside packages that have nothing to do with a screen, so the directory said nothing about whose they were.

The direction of imports does not settle where a package goes. Every package in the layer table imports in one direction, and the table, not the directory, enforces it (ADR-002, ADR-033). What a directory can still say is what a package is a part of.

## Decision

- A package that exists for the TUI's screen is under `internal/tui`, whatever else imports it. The status line is `internal/tui/statusline` and the keys are `internal/tui/keymap`. `internal/app` importing the keymap to build it from the config does not make it a package of its own, as it imports `internal/tui/chat` too.
- A package that means something apart from a screen stays at the top of `internal/`, even when only the TUI imports it today. `internal/diff` reads patches anyone can write, which is not a part of the screen. `internal/near` is used by `internal/config` as well. `internal/listcache` keeps lists on disk, beside `internal/state`.
- Its layer is still set by the table in `arch_test.go`, not by its directory, as ADR-033 says. A package under the TUI may be in a low layer.
- Rejected: placing a package by whether its imports are one way, which every package in the table already is.

## Consequences

- When a package under the TUI comes to be needed apart from the screen, it moves out to the top of `internal/`; when one at the top comes to exist only for the screen, it moves under the TUI.
- ADR-004, ADR-023 and ADR-025 name `internal/keymap` and `internal/statusline` as they were when they were written; the packages are now `internal/tui/keymap` and `internal/tui/statusline`.
