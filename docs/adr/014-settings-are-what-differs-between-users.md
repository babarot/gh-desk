# ADR-014: Settings are what differs between users

## Status

Accepted
Partly replaced by ADR-092

## Context

Some values that decide how gh-desk works are constants in the code, others are settings, and a rule is needed for which kind a new value should be. Each setting has a cost besides its line in the file: a row in the table of settings (`internal/config/settings.go`) with its words and the values it takes, its default in `config.Default()`, a check for every value it can take that is wrong, and tests. A setting that is never changed pays that cost for nothing, and one that is changed without knowing what it guards can break gh-desk for that user only, where it is hard to see.

The values inside gh-desk are of the second kind. How often a waiting pull request is read (`PollFast` and `PollSlow` in `internal/runner`), how many pull requests a bulk approve or merge sends at a time and how many run at once (`bulkBatch` and `bulkInFlight` in `internal/tui/bulk.go`), the same for reading the heavy fields after a list (`heavyBatch` and `heavyInFlight` in `internal/tui/heavy.go`), the timeout of a call to GitHub (`internal/github`), how long a toast stays, the mouse's double click and wheel lines: each was picked against GitHub's rate limits, the load put on GitHub, or how the screen reads. Turning them up could get a user rate limited or put more load on GitHub than intended, and there is hardly a case where someone needs them changed.

## Decision

- A setting is something whose right answer depends on who uses gh-desk: what they look at (the tabs, the inbox, the list's columns), how it looks (theme, the preview pane, the description), what gh-desk may do on GitHub in their name (merge methods, auto-merge, updating branches, how long to wait for checks and rules, the rules of S), how Claude is used (model, tools, language, instructions), the keys, and where their environment keeps things (the cache directory, the editor).
- A value that decides how gh-desk works inside is a constant in the code, not a setting: polling intervals, batch sizes and how many requests run at once, API timeouts, how long auto-merge is watched, the toast's duration, the mouse's values, the list's column widths and the like. A change to one is a change to the code, reviewed as one.
- A new setting is added only when it meets this rule. It comes with all of these, in the same change: its row in the table of settings (key, kind, words), its default in `config.Default()`, a check for every value it can take that is wrong (from its kind, or in `config.Load()`, or by the caller, reported through `config.Problem`), and tests. The template written on first run, the JSON Schema and the checks of ranges and choices are made from the row.
- Where a setting sits in the file follows ADR-016.

Two settings show the rule:

- `review.editor` is the editor ctrl+e opens to write a review. It differs from user to user, and `$VISUAL` and `$EDITOR` are often set for a shell rather than for a TUI (a terminal editor, where a GUI one with `--wait` is wanted here, or the other way round). The order is `review.editor`, then `$VISUAL`, then `$EDITOR`, and a value may carry arguments, as `"code --wait"`.
- `cache.dir` is where gh-desk keeps what it can read again. It holds `lists.json` besides Claude's clones and worktrees, so it has a table of its own rather than one under `[claude]`, where a user who does not use Claude would not look for it.

## Consequences

- A request to make one of the inside values a setting is answered by this ADR. If a case comes up where a user does need one changed, it is decided in a new ADR, with what guards the value written next to the setting.
- Every setting has a default, a line in the written config file, a check and tests. The tests of `internal/config` keep the table of settings, the decoded file, the template and the schema in step.
