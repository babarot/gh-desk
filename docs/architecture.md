# Architecture

gh-desk is split into packages by layer. A package imports only packages of lower layers, and only the outside modules listed for it. The table below is what `arch_test.go` checks; `go test -run TestArchitectureDoc -update` rewrites it from the test, and the test fails when the two differ.

<!-- layers:start -->
| Layer | Package | Does | May import | Outside modules |
|---|---|---|---|---|
| 0 | `internal/model` | Shared types: pull requests and their names | - | - |
| 0 | `internal/logging` | The slog logger and the log file | - | - |
| 0 | `internal/version` | The release version, which tagpr updates | - | - |
| 0 | `internal/near` | The closest of a few names to one mistyped, for a did-you-mean | - | - |
| 0 | `internal/release` | Whether a newer release of gh-desk is out, how this install is updated, and what was found and the settings known kept between launches | - | - |
| 0 | `internal/tui/askbox` | What the text boxes that ask Claude share and do not draw: the questions asked before, and the place among them | - | - |
| 0 | `internal/tui/statusline` | What the status line shows: events for a moment, work in progress, the row's result | - | - |
| 1 | `internal/config` | Reading the config file and reporting its mistakes | `internal/near` | `github.com/pelletier/go-toml/v2` |
| 1 | `internal/tui/keymap` | Keys bound to actions by name | `internal/near` | - |
| 2 | `internal/config/ui` | The page of gh desk config ui, one HTML file made from the settings, served on this machine | `internal/config` | - |
| 1 | `internal/plan` | Deciding how to merge each pull request; selection rules | `internal/model` | - |
| 1 | `internal/diff` | Reading patches and the words that changed | - | - |
| 2 | `internal/github` | GitHub's GraphQL and REST APIs | `internal/model`, `internal/logging` | `github.com/cli/go-gh/v2` |
| 2 | `internal/state` | The pending-merge record and its lock; the conversations with Claude and their locks; the questions asked of Claude about the lists; the directories of the running gh-desk; the screen layout kept between launches | `internal/model` | - |
| 2 | `internal/claude` | Running claude -p and the pull request worktrees, and the skills of the plugins given to it | `internal/model`, `internal/logging` | `gopkg.in/yaml.v3` |
| 2 | `internal/listcache` | The lists last read, kept on disk for the next launch | `internal/model` | - |
| 3 | `internal/runner` | Bulk approve and merge, auto-merge watching, resuming | `internal/model`, `internal/logging`, `internal/plan` | - |
| 3 | `internal/mcpserver` | gh-desk's own MCP server: read-only tools over the pull request's repository for Claude | `internal/model`, `internal/logging` | `github.com/modelcontextprotocol/go-sdk` |
| 3 | `internal/tui/chat` | The conversations with Claude about pull requests: what each holds, keeping them and their locks | `internal/model`, `internal/logging`, `internal/claude` | - |
| 4 | `internal/tui` | The terminal UI | `internal/model`, `internal/logging`, `internal/plan`, `internal/diff`, `internal/tui/keymap`, `internal/runner`, `internal/claude`, `internal/listcache`, `internal/tui/statusline`, `internal/tui/chat`, `internal/tui/askbox`, `internal/release` | `charm.land/bubbletea/v2`, `charm.land/bubbles/v2`, `charm.land/lipgloss/v2`, `github.com/charmbracelet/x/ansi`, `golang.org/x/net/html`, `github.com/kyokomi/emoji/v2` |
| 4 | `demo/internal/playground` | A made-up GitHub and Claude that the playground on the site runs against, and the scenarios that take it to a screen; never built into gh-desk | `internal/model`, `internal/claude`, `internal/runner`, `internal/tui/keymap` | - |
| 5 | `internal/app` | Putting it together: commands and flags, config, log, and starting the TUI | `internal/model`, `internal/logging`, `internal/config`, `internal/tui/keymap`, `internal/plan`, `internal/github`, `internal/state`, `internal/claude`, `internal/listcache`, `internal/runner`, `internal/mcpserver`, `internal/tui`, `internal/tui/chat`, `internal/tui/askbox`, `internal/version`, `internal/release`, `internal/config/ui` | `github.com/spf13/cobra`, `github.com/cli/go-gh/v2` |
| 6 | `main` | Calls app.Run | `internal/app` | - |
| 6 | `demo/internal/recording` | A made-up GitHub and Claude that the TUI runs against for the recordings in the README | `internal/model`, `internal/logging`, `internal/config`, `internal/tui/keymap`, `internal/runner`, `internal/claude`, `internal/tui`, `internal/app`, `internal/version` | - |
| 7 | `demo/recording` | Runs the recordings' made-up world for VHS to record; never built into gh-desk | `demo/internal/recording` | - |

Standard library packages that reach outside the process:

| Package | Only in |
|---|---|
| `net` | `internal/config/ui`, `internal/github` |
| `net/http` | `internal/config/ui`, `internal/github` |
| `os/exec` | `demo/internal/playground`, `demo/internal/recording`, `internal/app`, `internal/claude`, `internal/github`, `internal/tui` |
| `syscall` | `internal/state` |
<!-- layers:end -->

## Rules

- Each part of the outside world has one package. The network is `internal/github`, the `claude` command and `git` are `internal/claude`, the pending-merge record and its lock, the conversations with Claude and their locks, and the screen layout kept between launches are `internal/state`, and the terminal is `internal/tui`. The standard library packages that reach outside the process are limited to them in the table above; besides `internal/claude`, `os/exec` is allowed in `internal/github` to open the browser and in `internal/tui` to open `$EDITOR`.
- `internal/runner` does not import Bubble Tea. It reports progress as events of its own type; `internal/tui` turns them into messages. This keeps the bulk approve and merge logic testable without a terminal.
- Interfaces are defined where they are used. `internal/runner` defines the GitHub operations it needs, and `internal/tui` the ones it needs (fetching lists, a diff, a single review, a notification done, the Claude conversation); `internal/app` passes the concrete clients. `internal/github` and `internal/claude` return concrete types.
- `internal/config` returns plain values. It holds the settings as basic Go types, which `internal/app` turns into what `internal/plan`, `internal/tui/keymap` and `internal/tui` take, and leaves checks that need another package's knowledge (a log level, a key) to that package, reported as `config.Problem` so every mistake is still shown where it is in the file.
- No package keeps state in package variables. The logger, the config and the clients are built in `internal/app` and passed as arguments and fields. `slog.SetDefault` is not used.
- Keys are never written in the TUI's code. The TUI asks `internal/tui/keymap` for an action by name, and draws the key hints and the `?` list from it.

## Logging

The log is JSON lines written with `log/slog` to `$XDG_STATE_HOME/gh-desk/logs/gh-desk.log` (`gh desk log` prints the path), never to the terminal. The level is `info` unless `--debug`, `GH_DESK_LOG_LEVEL` or `[log] level` says otherwise, in that order. The file is moved aside at startup when it is over 10 MB, and three old files are kept.

Every line carries `run`, an ID for the process. Code that starts an operation adds `op`, and bulk approve and merge add `batch`, `pr` and `step`, with `logger.With`, and passes the logger down. Calls to GitHub log `gh_request_id`, `gql_cost`, `gql_remaining` and `duration_ms`.

Every write to GitHub (a comment, a review, merge, enabling auto-merge, updating a branch, marking a draft ready for review, marking a notification done) is logged at `info` with its pull request, result and request ID. Reads are logged at `debug`.

Tokens, pull request bodies, diffs, comments, review bodies and Claude conversations are never logged; a pull request is named as `owner/repo#n`, not by its title. A log pasted into an issue must not carry anyone's work.

## Decisions

Decisions that set the direction of the code are recorded as ADRs in [docs/adr](adr/).
