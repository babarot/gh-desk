# ADR-099: The config file is replaced by a command the user runs

## Status

Accepted
Partly replaced by ADR-101

## Context

A tool that makes a whole config file, such as a settings page in the browser, needs a way to put it in place. If that tool wrote the file itself, the browser could write it, and a setting can name a command to run (`claude.command`, `review.editor`, `claude.mcp.servers`), so a forged request could end in running code. Pasting the file in an editor needs no such thing, but a mistake is found only once the file is saved, and gh-desk does not start with mistakes in it.

## Decision

- `gh desk config apply` reads a whole config file from standard input and replaces the config file with it. It is not a patch: what is piped is what the file becomes. The user runs it, in their terminal; nothing in the browser writes the file.
- The file is checked as gh-desk checks it at start, through the same function, since `config.Parse` alone leaves out the keys, `claude.allowed_tools` and the plugins. With a mistake, nothing is written.
- The new file is written beside the config file and checked there, the old one is copied to `config.toml.bak`, and one rename puts the new one in place, so the config file is there at every moment. Moving the old one aside first would leave none if the next step failed, and gh-desk would then start on its defaults.
- A link stays a link: what it points to is replaced. Where that cannot be written, as in the Nix store, the error says so and nothing changes.
- Empty input changes nothing and is no mistake: a tool that ended without saving gives nothing, and an empty clipboard must not empty the file.
- Rejected: a patch of what changed. It would meet the conflicts of a three-way merge, and what is written could not be read before it is written.

## Consequences

- Comments, the order of the keys and alignment do not survive, as the file is replaced whole; the old file keeps them in `.bak`.
- `.bak` holds the one file before; a second apply replaces it.
- Warnings do not stop `apply`, as they do not stop gh-desk.
