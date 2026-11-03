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

## Text input

- Make a text box with `newInput` in `internal/tui`, and return its `Cursor()`, placed on the screen, from `render` so the view gives it to the terminal. Never draw a cursor: an input method shows the text being composed where the terminal's cursor is. `cursor_test.go` checks both

## Config

- A mistake in the config file is reported where it is, through `config.Problem` and `config.Report`, never ignored. A check that needs another package (a log level, a key) is done by the caller and reported the same way

## Decisions

- Record a decision that sets the direction of the code as an ADR in `docs/adr/`, following `.claude/rules/adr.md`. It comes from babarot/github-config, and the Regomark workflow checks `docs/adr` against it
- The lists an ADR names instead of copying are the package table (`arch_test.go`), the settings (`internal/config`), the default keys (`internal/tui/keymap`) and the log attribute names (`internal/logging`). A value left out of an ADR is a constant, as ADR-014 decides

## Docs

- The pages of the docs site are the Markdown files in `docs/`, outside `docs/adr` and `docs/site`, written to read on GitHub as they are: a `# title` first, no front matter, links to other pages by their `.md` file. See [ADR-054](docs/adr/054-the-docs-are-markdown-in-docs-built-by-hugo.md)
- Never edit the pages in `docs/reference` by hand, but its `README.md`; the tests write them from the code (ADR-051)
- A directory's `README.md` is the page of its section on the site, as GitHub shows it with the directory
- Never name a page `claude.md`: on a filesystem that ignores case, Claude Code reads it as its instructions
- `mise run docs:serve` shows the docs with the playground as they are published; `mise run docs:test` is what CI checks

## Test data

- Tests and fixtures must not contain data from any private repository or organization. Use this repository, other public repositories, or made-up data
