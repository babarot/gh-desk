# ADR-003: Structured logging to a file with slog

## Status

Accepted

## Context

gh-desk approves and merges pull requests, often many at once, sometimes after waiting for checks across a restart. These actions cannot be undone, so it must be possible to tell afterwards what was done to which pull request, when, and with what result. Failures (an API error, a merge refused because the head moved, a wait that timed out) must be explainable without reproducing them. The TUI owns the terminal, so nothing can be printed there.

The project is meant to become public. Users will paste logs into issues, so the log must not carry the content of anyone's pull requests.

## Decision

- Log with the standard library's `log/slog`, as JSON lines, to `$XDG_STATE_HOME/gh-desk/logs/gh-desk.log` (default `~/.local/state/gh-desk/logs/`). `gh desk log` prints the path. Nothing is logged to stdout or stderr while the TUI runs.
- Rotate at startup by size, keeping a few old files. No outside library.
- The level is `info` unless `--debug`, `GH_DESK_LOG_LEVEL` or `[log] level` says otherwise, in that order.
- `internal/app` builds the logger, adds `run` (an ID per process) and passes it down. Packages add their own attributes with `logger.With`, and GitHub calls add the request ID and the cost. The attribute names are constants in `internal/logging`. No package variable logger; `slog.SetDefault` is not used.
- Every write to GitHub is logged at `info` with its pull request, result and request ID. Reads and the reasons behind merge decisions are logged at `debug`.
- Never logged: tokens, pull request bodies, diffs, comments, review bodies, Claude conversations. A pull request is named as `owner/repo#n`, not by its title. Typed text is not logged; key presses are logged at `debug` as the action name only.

## Consequences

- A bulk merge can be followed in the log by its `batch` ID, and a GitHub-side question by its request ID.
- Errors shown in the TUI carry the `run` ID so the matching log lines can be found.
- Debugging a decision about a pull request's content needs the pull request itself, not the log.
