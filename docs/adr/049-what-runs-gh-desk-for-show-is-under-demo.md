# ADR-049: What runs gh-desk for show is under demo

## Status

Accepted

## Context

The recordings of ADR-045 and the site of ADR-047 run gh-desk against made-up worlds. Their parts were spread over four places: the two made-up worlds at `internal/demo` and `internal/demo/playground`, beside the packages gh-desk is built from; the recordings' program at `hack/demo`, the only thing under `hack`; their tapes and scripts at `docs/tapes`, among the documents; and the site at `site`. Nothing in the tree said that these were one thing, and the made-up worlds at the top of `internal/` said nothing about whose they were, as ADR-034 found of the keys and the status line.

Go lets a package under an `internal` directory be imported only from the tree rooted at that directory's parent. Under `internal/`, the made-up worlds could be imported from anywhere in gh-desk; only the table in `arch_test.go` kept them out of it.

## Decision

- `internal/` holds what gh-desk is built from. `demo/` holds what runs gh-desk for show and is never built into it: the recordings, the site and the made-up worlds they run against.
- A package that exists for the demo is under `demo/`, as a package that exists for the TUI's screen is under `internal/tui` (ADR-034). The made-up worlds are [demo/internal/recording](../../demo/internal/recording) and [demo/internal/playground](../../demo/internal/playground): under `demo/internal`, so that Go itself refuses an import of them from gh-desk, and the programs under `demo/` may still import them.
- The two worlds stay apart, as ADR-047 decides. Sharing `demo/internal` does not let one import the other; the table in [arch_test.go](../../arch_test.go) holds what each may import, and its layers, as ADR-002 says.
- The made-up worlds are in the main module, and `demo/` has no go.mod of its own, so `go test ./...` builds and tests them with gh-desk and fails to build when an interface they fill gains a method. Only [demo/site](../../demo/site) is a module of its own, for the reason ADR-047 gives, and its module path is under `demo/` so that it may import `demo/internal/playground`.
- Each part of `demo/` is named by what it makes, not by the tool that makes it: `recording`, not `vhs`, and `site`. The recorder can change without the directory's job changing.
- Rejected: keeping the made-up worlds under `internal/`. Go's `internal` is about who may import a package, and `internal/demo` is valid Go, but the directory would say the worlds are a part of gh-desk, and keeping them out of it would rest on the test alone.
- Rejected: the site's world inside the site's module, under `demo/site`. It would leave the main module, and `go test ./...` would no longer build it against the TUI it runs.

## Consequences

- A new program or world for showing gh-desk goes under `demo/`, with its row in the table in `arch_test.go`; a package gh-desk is built from never goes there.
- The table in `arch_test.go` names the packages each may import by their path in the module, as `internal/model` and `demo/internal/recording`, since they are no longer all under `internal/`.
- The site's module path follows its directory. Moving the site out of `demo/` would cut it off from `demo/internal/playground`.
- ADR-045 and ADR-047 link to the parts where they are now; their decisions are unchanged.
