# ADR-014: Settings are what differs between users

## Status

Accepted

## Context

The config file grew one setting at a time, each added when it was wanted, with no rule for what belongs in it. Some values that decide how gh-triage works inside are constants in the code, others are settings, and nothing said which kind a new value should be. Each setting has a cost besides its line in the file: a default in `config.Default()`, a commented line and its words in `config.Template()`, a check in `config.Load()` reported through `config.Problem`, and tests. A setting that is never changed pays that cost for nothing, and one that is changed without knowing what it guards can break gh-triage for that user only, where it is hard to see.

The values inside gh-triage are of that second kind. How often a waiting pull request is read (`PollFast` and `PollSlow` in `internal/runner`), how many pull requests a bulk approve or merge sends at a time and how many run at once (`bulkBatch` and `bulkInFlight` in `internal/tui/bulk.go`), the same for reading the heavy fields after a list (`heavyBatch` and `heavyInFlight` in `internal/tui/heavy.go`), the timeout of a call to GitHub (`internal/github`), how long a toast stays, the mouse's double click and wheel lines: each was picked against GitHub's rate limits, the load put on GitHub, or how the screen reads. Turning them up could get a user rate limited or put more load on GitHub than intended, and there is hardly a case where someone needs them changed.

## Decision

- A setting is something whose right answer depends on who uses gh-triage: what they look at (the tabs, the inbox), how it looks (theme, the preview pane, the description), what gh-triage may do on GitHub in their name (merge methods, auto-merge, updating branches, the rules of S), how Claude is used (model, tools, language, instructions), the keys, and where their environment keeps things (the cache directory, the editor).
- A value that decides how gh-triage works inside is a constant in the code, not a setting: polling intervals, batch sizes and how many requests run at once, API timeouts, the toast's duration, the mouse's values and the like. A change to one is a change to the code, reviewed as one.
- A new setting is added only when it meets this rule. It comes with all of these, in the same change: its default in `config.Default()`, a commented line with its words in `config.Template()`, a check in `config.Load()` (or by the caller, reported through `config.Problem`) for every value it can take that is wrong, and tests.
- A setting sits in the section a user would look for it in. A value used by more than one feature gets a section of its own rather than one of those features'.

This change applies the rule twice:

- `ui.editor` is added. The editor ctrl+e opens to write a review differs from user to user, and `$VISUAL` and `$EDITOR` are often set for a shell rather than for a TUI (a terminal editor, where a GUI one with `--wait` is wanted here, or the other way round). The order is `ui.editor`, then `$VISUAL`, then `$EDITOR`, and a value may carry arguments, as `"code --wait"`. It is under `[ui]`, with how the TUI looks, since it is about the screen the user works on, and one key does not make a section worth having.
- `claude.cache_dir` becomes `cache.dir`. The directory holds `lists.json` besides Claude's clones and worktrees, so a user who does not use Claude would not look for it under `[claude]`. A file that still has `claude.cache_dir` is told at that line that it moved; the old name is not read.

## Consequences

- A request to make one of the inside values a setting is answered by this ADR. If a case comes up where a user does need one changed, it is decided in a new ADR, with what guards the value written next to the setting.
- Every setting has a default, a line in the written config file, a check and tests. `TestTemplateLoadsAsDefault` and `TestKeysTemplateLoadsUncommented` keep the template and the defaults in step.
- Moving a setting breaks the files that have it under its old name, once, with a message at the line saying where it went. That is preferred to reading both names, which would leave two ways to write one setting.
- ADR-010 and ADR-011 name `claude.cache_dir`, as it was when they were written; the directory is now `cache.dir`.
