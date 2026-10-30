# CLAUDE.md

## Language

- This repository will be public. Write everything in it in English: code, comments, UI text, commit messages, docs and ADRs.

## Packages

- Packages are layered, and `arch_test.go` checks every import against the table in `docs/architecture.md`. A new package, outside module, or use of `net`, `net/http`, `os/exec` or `syscall` goes into the table in `arch_test.go` first; then run `go test -run TestArchitectureDoc -update` to rewrite the document. See [ADR-002](docs/adr/002-package-layers-enforced-by-a-test.md)
- Define an interface in the package that uses it; `internal/app` passes the concrete type
- No package variables holding state, and no `slog.SetDefault`. Pass the logger, config values and clients down from `internal/app`

## Logging

- Log through the `*slog.Logger` passed in, with the attribute keys in `internal/logging` (`op`, `batch`, `pr`, `step`, ...). See [ADR-003](docs/adr/003-structured-logging-to-a-file.md)
- Log every write to GitHub at info with its pull request, result and request ID; reads at debug
- Never log tokens, pull request bodies, diffs, comments, review bodies, Claude conversations, or typed text. Name a pull request as `owner/repo#n`, not by its title

## Keys

- Never compare key strings in `internal/tui`. Add the action to `keymap.Defaults` and ask `Matches(action, key)`; key hints and the `?` list come from the keymap. See [ADR-004](docs/adr/004-keys-bound-by-action-name.md)

## Config

- A mistake in the config file is reported where it is, through `config.Problem` and `config.Report`, never ignored. A check that needs another package (a log level, a key) is done by the caller and reported the same way

## Decisions

- Record a decision that sets the direction of the code as an ADR in `docs/adr/`, following `.claude/rules/adr-template.md`

## Test data

- Tests and fixtures must not contain data from any private repository or organization. Use this repository, other public repositories, or made-up data
