# ADR-045: The demo runs the TUI against a made-up GitHub, outside gh-desk

## Status

Accepted

## Context

The README needs a recording of gh-desk at work: the lists, a pull request's diff, a question to Claude, and a merge of several pull requests. A recording of real pull requests shows private data, changes each time it is made, and needs a GitHub login and Claude in the recording's container. gh-infra records its demos with a mock of the `gh` command, but gh-desk does not run `gh`: it reads GitHub through go-gh in the process and runs `claude -p` for Claude.

## Decision

- The recordings run the real TUI, the real runner and the real config file a first launch writes, against a made-up GitHub and a made-up Claude in [demo/internal/recording](../../demo/internal/recording). The TUI's `Source` and `Claude` interfaces and the runner's `GitHub` interface are where they are put in.
- The demo is its own program, [demo/recording](../../demo/recording), never built into gh-desk. gh-desk has no switch, flag or variable that points it at the made-up GitHub, so nothing a user runs can show made-up data or skip GitHub.
- The demo's data is made up: no real organization, repository or person, as for tests.
- The made-up GitHub keeps the reviews and merges made, and answers a little slower than at once, so that the screen moves as it would against GitHub.
- `internal/app` hands the demo how the config file becomes the TUI's settings, so that the demo shows what a user's config does instead of a copy of it.
- The tapes and the scripts that record them are in [demo/recording/tapes](../../demo/recording/tapes); `mise run demo` records them with VHS in Docker.

Rejected:

- A mock of GitHub's API over HTTP, with gh-desk pointed at it: it needs a way into the shipped binary to change the host, and answers to every GraphQL query gh-desk makes.
- Recording against a GitHub account kept for the demo: the recording would change each time, and Claude's answers with it.

## Consequences

- A method added to the TUI's `Source` or `Claude`, or to the runner's `GitHub`, needs one in the made-up GitHub or Claude too; [demo/internal/recording/demo_test.go](../../demo/internal/recording/demo_test.go) fails to build until it has one.
- What the demo shows of GitHub is only as true as the made-up GitHub: a change in how gh-desk reads GitHub does not show in the recording until the demo follows it.
- The made-up Claude answers the questions the tapes ask, whatever was asked.
