# ADR-002: Package layers and import rules enforced by a test

## Status

Accepted

## Context

gh-triage will grow parts that are easy to tangle: a TUI, a GitHub client, a runner that approves and merges many pull requests while waiting for checks, a record of pending merges that survives a restart, and a Claude Code subprocess. Once the bulk merge logic calls into the TUI, or the TUI calls GitHub directly, the irreversible parts (approving, merging) can no longer be tested without a terminal and a network, and untangling them later costs more than keeping them apart from the start.

## Decision

Split the code into layered packages, each allowed to import only lower layers and a listed set of outside modules, as `docs/architecture.md` shows:

- Layer 0: `model` (shared types), `logging`, `near` (the closest name, for a did-you-mean)
- Layer 1: `config`, `keymap`, `plan` (pure decisions), `diff`
- Layer 2: `github`, `state`, `claude`, `listcache` (each owning one part of the outside world)
- Layer 3: `runner` (bulk approve and merge; no Bubble Tea)
- Layer 4: `tui`
- Layer 5: `app` (wiring), then `main`

Standard library packages that reach outside the process (`net`, `net/http`, `os/exec`, `syscall`) are allowed only in the packages that own that reach.

`arch_test.go` lists every package with its layer and allowed imports, reads `go list -json ./...`, and fails on any import outside the list, on any package missing from the list, and on a table entry that points upward. It also fails when the table in `docs/architecture.md` differs from it; `go test -run TestArchitectureDoc -update` rewrites the document.

Interfaces are defined by the package that uses them; `app` passes the concrete types. No package keeps state in package variables.

## Consequences

- A new package, a new outside module, or a new use of `os/exec` is a visible change to `arch_test.go` and the document, not an accident.
- `runner` and `plan` can be tested with fakes for GitHub and the pending record.
- Some code is passed down explicitly (the logger, the config values) where a global would be shorter.
- Test files are not checked, so a test may import helpers from any package.
