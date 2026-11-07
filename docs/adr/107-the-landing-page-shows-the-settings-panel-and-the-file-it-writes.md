# ADR-107: The landing page shows the settings panel and the file it writes

## Status

Accepted

## Context

ADR-106 has the landing page embed the settings playground under the playground, which made two emulated terminals on one page, and the second showed only the pages ADR-103 keeps to. What it is there to show on the landing page is that the file need not be written by hand, and the list following the settings says that less directly than the file does.

ADR-103 leaves the pages of the rules out because a rule nothing judged reads as one that matches nothing. That reason is about the count, not the page.

## Decision

This replaces, on the landing page only, two parts of ADR-103: "It shows only what needs no gh-desk and what most people change first", and "Rejected: the pages of the rules without their counts". It also replaces, of ADR-106, "It keeps TOML only for what the settings page cannot do".

- The landing page runs one terminal, the playground.
- It shows the settings panel as the command draws it, with every page, and beside it the file those settings would write, apart from the panel. The file is the result on this page, in place of the list.
- The panel shows no count for a rule. A rule is judged nowhere here, and nothing reads as its result, so the pages of the rules are shown.
- The panel saves nothing, as ADR-103 asks of the page on the site.
- The configuration guide and `play/settings/` keep the list and the pages of ADR-103.
- Rejected: judging the rules with `gh-desk.wasm` in the panel. The landing page has no need of a count once the file is the result.
- Rejected: the file inside the panel's frame. It reads as part of the page the command opens, which has no such thing.

## Consequences

- The panel tells the page around it the file through a message, so a page embedding it in this form has to listen for one.
- The panel turns to its narrow form at the width the command's page does, so the landing page gives it more room than that, past its column if need be.
- The panel on the landing page differs from the command's page in what it cannot do: it has no way to close and no count on a rule. Anything else that needs gh-desk behind it has to stay out of this form, as ADR-103 says of the page on the site.
