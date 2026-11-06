# ADR-101: The settings page saves the file, and the terminal confirms what is more than drawn

## Status

Accepted

## Context

ADR-099 says a page must not write the config file, since a setting can name a command and a forged request could end in running code, and that the user therefore runs `config apply` in the terminal. That protects less than it says. `config ui | config apply` starts both commands before the page opens, so whatever the page accepts is applied with no further look; a pipe is a transport, not a check. The defense was in what stands in front of the page, and it holds with or without the pipe.

What stands in front:

- The token, which a web site cannot read, and `Host` and `Origin`, which refuse a site that points its name at 127.0.0.1. These keep out web sites open in the same browser. That is what a page on localhost has to fear.
- Nothing keeps out another program run by the same user. It can read the URL where a process's arguments are listed, and it can write the config file anyway, with no page. This is out of scope, and no design here changes it.

What is left is someone with the URL (a copy in shell history, a shared screen, a browser extension) or a page that an injected script drives. Neither is stopped by a check inside the browser, as the browser is what they reach. The terminal is the one place a request to the page cannot answer.

## Decision

- The page is `gh desk config edit --ui`, a way to edit the file, and not a command of its own. It never runs `review.editor`.
- Saving on the page replaces the config file, by the same code as `config apply`: checked as at start, the old file kept as `.bak`, one rename. Standard output and the clipboard no longer carry the file. `config apply` stays, for a file made by anything else; it asks nothing, since the user chose what to pipe.
- A change to anything but how things are drawn is listed in the terminal and saved only on `y` or `yes`. The settings that need no question are marked `Display` in the settings table. A setting is marked only if it can make gh-desk neither run anything, nor write to GitHub, nor read something else, nor put another action under a key. Everything else asks, a new setting and a key nobody knows included, so that forgetting to mark one costs a question and not a hole. A test spells out the list, so that adding to it is a visible change.
- What is listed is worked out in Go from the file as read and the file sent, never told by the page. It includes a setting removed. What is confirmed is what is written, byte for byte.
- The question is put on the terminal of standard input. No terminal, the end of the input, an empty answer or anything but yes refuses, and the page stays open.
- The list is safe to print: keys and values are quoted so that a control character cannot redraw the terminal, and an MCP server's `env`, `headers` and the credentials in its `url` are not shown. Secrets stay out of the scrollback; the names of the keys stay, as they say what changes.
- The file is compared again right before it is replaced, after the question, so that a change made while the terminal asked is not overwritten. This narrows the window; it does not close it.
- A file gh-desk creates, and `.bak`, are for the user alone (0600), as they can hold what an MCP server signs in with.
- The page must not be framed (`frame-ancestors 'none'`).
- Rejected: keeping `ui | apply`. It adds a step and no defense, and it reads as if it did.
- Rejected: a question on the page. What it would stop reaches the page first.
- Rejected: asking for every change. A question asked of the theme teaches `y` as a reflex.
- Rejected: asking only for the settings that name a command. A rule in `[[auto_approve]]`, `exclude` or `checks.approval` changes what is approved and what is seen with no command in it.

## Consequences

- ADR-099's reasoning no longer stands, though `config apply` and its way of replacing the file do.
- A new setting that is only drawn asks until someone marks it, which is the cheap direction.
- The page cannot save such a change where no terminal is attached; `config apply` remains for that.
- A question is one more step where a run of edits touches rules or keys; it is asked once for each save.
