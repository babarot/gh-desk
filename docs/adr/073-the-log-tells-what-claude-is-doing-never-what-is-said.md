# ADR-073: The log tells what Claude is doing, never what is said

## Status

Accepted

## Context

A question to Claude takes from seconds to minutes: the repository may be cloned, the worktree of the pull request is made at its head, and Claude Code then reads files with its tools. The log said only that Claude answered or stopped, at the end; the worktree being ready and each tool Claude used were at debug. While a question ran, the log was silent, and did not tell whether gh-desk was cloning or Claude was reading. Claude also works behind the confirm popup, reading the release notes of bot updates, and its triage plans change what is selected and shown; only failures of the first and nothing of the second were logged at info.

ADR-003 keeps Claude conversations, pull request bodies and typed text out of the log.

## Decision

This adds to what ADR-003 and ADR-057 log at info.

- A question to Claude is logged when it is asked and when it ends, with what it is for (a question about a pull request, a triage, a one-off question such as reading release notes), the pull request when there is one, the model, how many tools Claude used and how long it took.
- The worktree of a pull request is logged when it is ready, with how long it took to make.
- A triage plan applied is logged with how many of its actions were applied and how many were left out; release notes read for the confirm popup are logged with how many pull requests were read and how many call for care.
- What is asked, what Claude answers, the tools' arguments (file paths, patterns) and the release notes themselves are never logged, as ADR-003 says. Each tool used stays at debug, by its name only.
- An error Claude Code returns in the place of an answer is logged as its first line, cut short; the status line shows it whole. It is meant as an error, but comes where Claude's words would.
- Rejected: logging each tool Claude uses at info. A question can use dozens, and their arguments say what is being read.

## Consequences

- A new way of asking Claude is logged the same way, with what it is for, or the log cannot tell it from the others.
