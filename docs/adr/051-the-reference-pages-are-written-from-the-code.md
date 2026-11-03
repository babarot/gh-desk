# ADR-051: The reference pages are written from the code

## Status

Accepted

## Context

gh-desk has many settings, keys in several scopes, and a few commands. The README tells only the ones to start with; a reader looking for one setting or one key has the config file's comments, `gh desk config schema`, or the `?` popup, none of which can be read without installing gh-desk.

Each of these lists already has one place in the code that defines it: the settings table of [internal/config](../../internal/config), which also writes the template and the JSON Schema; the keymap's defaults in [internal/tui/keymap](../../internal/tui/keymap), which the key hints and the `?` popup are drawn from; and the help that `gh desk -h` and `gh desk config help` print. A reference written by hand would copy them, and fall behind with the next change.

## Decision

- The reference pages of the documentation, [docs/reference](../reference), are written from those places by tests, as [docs/config.schema.json](../config.schema.json) and [docs/architecture.md](../architecture.md) are: each test builds its page and fails when the file differs, and `-update` rewrites it.
  - The configuration page: [internal/config/reference_test.go](../../internal/config/reference_test.go), from the settings table, with every setting's words, kind and default.
  - The keys page: [internal/tui/keys_reference_test.go](../../internal/tui/keys_reference_test.go), from the keymap's defaults, under the titles of the `?` popup's sections.
  - The commands page: [internal/app/commands_reference_test.go](../../internal/app/commands_reference_test.go), from the help the commands print.
- A generated page says that it is generated and how to write it again. Its words are changed where they come from (a setting's words, an action's help, a command's usage), never in the page.
- Only the words around a list are written in the test that writes the page. A page that explains (how tabs, rules or merging work) is written by hand and links to the reference for the list.
- The pages are Markdown that reads on GitHub as it is, so they are useful before any site is built from them.
- Rejected: reference pages written by hand. They copy lists that change with the code, and nothing would tell when they no longer agree.
- Rejected: writing the pages from a command of gh-desk (`gh desk config docs`). It would ship Markdown writing in the binary for the repository's sake, and the file would still need a test to stay current.

## Consequences

- A change to a setting, a default key, or a command's help fails `go test ./...` until the page is written again with `-update`, in the same change.
- The words of a setting or a key now reach three readers: the config file's comments, the `?` popup, and the web. They are written to read well in all three.
- A new list that the documentation should show in full (another command, another kind of config entry) is written by a test in the package that defines it, not by hand.
