# ADR-056: The commands are built with cobra

## Status

Accepted

## Context

gh-desk parsed its arguments with the standard library's `flag` and a `switch` over the first word. `flag` stops at the first word that is not a flag, and nothing read what came after it: `gh desk gc --help` ran `gc`, which removes worktrees, instead of printing its help; `gh desk gc --debug` dropped the flag; a stray argument to `gc` or `log` passed unnoticed; and `gh desk config check --help` took `--help` for a path.

These could be fixed by hand, command by command. gh-infra, by the same author, already uses cobra, and with it each command gets its own help, its own check of the arguments, and shell completion.

## Decision

- The commands and flags are a tree of cobra commands in [internal/app](../../internal/app), the only package that imports cobra (ADR-002).
- A flag is written with two dashes. `--config` and `--debug` are taken before or after a command; a flag of one command is refused on another.
- A command says what it takes, so a stray argument is an error and never ignored, and `--help` on any command prints its help and runs nothing.
- The exit code is 0, 1 when a command failed, and 2 when it was called wrongly, as it was with `flag`. Scripts may rely on these.
- Help goes to stdout when it is asked for, and to stderr after a wrong call.
- `gh desk config help` stays, as ADR-051 names it, next to `gh desk help config`.
- The name of the root command stays that of the binary, `gh-desk`, which shell completion runs; the help shows it as `gh desk`.
- Rejected: keeping `flag` and adding the checks by hand. Every command would need the same code for help and arguments, and the next command added could miss it, as `gc` did.

## Consequences

- The one-dash spellings that `flag` took, such as `-debug` and `-config`, are errors.
- cobra is a dependency of two modules, as [demo/site](../../demo/site) builds `internal/app` too.
- A new command is added to the tree; [the commands page](../reference/commands.md) has a section for each one, written by the test of ADR-051.
- A command that fails prints its own message and returns the exit code; any other error from a command is reported as a wrong call.

## Not decided here

- Completion for `gh desk` typed through gh. The `completion` command is cobra's default, and it completes the name `gh-desk`.
