# Architecture

gh-triage is split into packages by layer. A package imports only packages of lower layers, and only the outside modules listed for it. The table below is what `arch_test.go` checks; `go test -run TestArchitectureDoc -update` rewrites it from the test, and the test fails when the two differ.

<!-- layers:start -->
| Layer | Package | Does | May import | Outside modules |
|---|---|---|---|---|
| 0 | `internal/model` | Shared types: pull requests and their names | - | - |
| 0 | `internal/logging` | The slog logger and the log file | - | - |
| 1 | `internal/config` | Reading the config file and reporting its mistakes | `model` | `github.com/pelletier/go-toml/v2` |
| 1 | `internal/keymap` | Keys bound to actions by name | - | - |
| 1 | `internal/plan` | Deciding how to merge each pull request; selection rules | `model` | - |
| 1 | `internal/diff` | Reading patches and the words that changed | - | - |
| 2 | `internal/github` | GitHub's GraphQL and REST APIs | `model`, `logging` | `github.com/cli/go-gh/v2` |
| 2 | `internal/state` | The pending-merge record and its lock; the screen layout kept between launches | `model`, `logging` | - |
| 2 | `internal/claude` | Running claude -p and the pull request worktrees | `model`, `logging` | - |
| 2 | `internal/listcache` | The lists last read, kept on disk for the next launch | `model` | - |
| 3 | `internal/runner` | Bulk approve and merge, auto-merge watching, resuming | `model`, `logging`, `plan` | - |
| 4 | `internal/tui` | The terminal UI | `model`, `logging`, `plan`, `diff`, `keymap`, `runner`, `claude`, `listcache` | `charm.land/bubbletea/v2`, `charm.land/bubbles/v2`, `charm.land/lipgloss/v2`, `github.com/charmbracelet/x/ansi`, `github.com/alecthomas/chroma/v2` |
| 5 | `internal/app` | Putting it together: flags, config, log, and starting the TUI | `model`, `logging`, `config`, `keymap`, `plan`, `diff`, `github`, `state`, `claude`, `listcache`, `runner`, `tui` | - |
| 6 | `main` | Calls app.Run | `app` | - |

Standard library packages that reach outside the process:

| Package | Only in |
|---|---|
| `net` | `internal/github` |
| `net/http` | `internal/github` |
| `os/exec` | `internal/claude`, `internal/github`, `internal/tui` |
| `syscall` | `internal/state` |
<!-- layers:end -->

## Rules

- Each part of the outside world has one package. The network is `internal/github`, the `claude` command and `git` are `internal/claude`, the pending-merge record and its lock and the screen layout kept between launches are `internal/state`, and the terminal is `internal/tui`. The standard library packages that reach outside the process are limited to them in the table above; besides `internal/claude`, `os/exec` is allowed in `internal/github` to open the browser and in `internal/tui` to open `$EDITOR`.
- `internal/runner` does not import Bubble Tea. It reports progress as events of its own type; `internal/tui` turns them into messages. This keeps the bulk approve and merge logic testable without a terminal.
- Interfaces are defined where they are used. `internal/runner` defines the GitHub operations it needs, and `internal/tui` the ones it needs (fetching lists, a diff, a single review, a notification done, the Claude conversation); `internal/app` passes the concrete clients. `internal/github` and `internal/claude` return concrete types.
- `internal/config` returns plain values. It hands `internal/plan`, `internal/keymap` and `internal/tui` the settings as `internal/model` types or basic Go types, and leaves checks that need another package's knowledge (a log level, a key) to that package, reported as `config.Problem` so every mistake is still shown where it is in the file.
- No package keeps state in package variables. The logger, the config and the clients are built in `internal/app` and passed as arguments and fields. `slog.SetDefault` is not used.
- Keys are never written in the TUI's code. The TUI asks `internal/keymap` for an action by name, and draws the key hints and the `?` list from it.

## Logging

The log is JSON lines written with `log/slog` to `$XDG_STATE_HOME/gh-triage/logs/gh-triage.log` (`gh triage log` prints the path), never to the terminal. The level is `info` unless `--debug`, `GH_TRIAGE_LOG_LEVEL` or `[log] level` says otherwise, in that order. The file is moved aside at startup when it is over 10 MB, and three old files are kept.

Every line carries `run`, an ID for the process. Code that starts an operation adds `op`, and bulk approve and merge add `batch`, `pr` and `step`, with `logger.With`, and passes the logger down. Calls to GitHub log `gh_request_id`, `gql_cost`, `gql_remaining` and `duration_ms`.

Every write to GitHub (approve, comment, request changes, merge, enabling auto-merge, updating a branch, marking a notification done) is logged at `info` with its pull request, result and request ID. Reads are logged at `debug`.

Tokens, pull request bodies, diffs, comments, review bodies and Claude conversations are never logged; a pull request is named as `owner/repo#n`, not by its title. A log pasted into an issue must not carry anyone's work.

## Decisions

Decisions that set the direction of the code are recorded as ADRs in [docs/adr](adr/).
