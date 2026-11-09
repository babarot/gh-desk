# ADR-110: The drawing of gh-desk keeps only what the site shows

## Status

Accepted

## Context

Since ADR-109 the page `gh desk config edit --ui` opens runs gh-desk itself, and the JavaScript drawing of gh-desk (ADR-100) is left only for the site's settings page (ADR-103), which has no gh-desk behind it. That page shows the list, and a scene for two of its pages: the confirm of a merge for Merging, and the keys for Keys. The drawing still had the scenes of the command's page: the menu of enter, the review box, the popup of S, the pane and Claude, and `mise run config-ui:test` compared the menu and the review box with gh-desk. Each of them follows gh-desk by hand or by that comparison, for a page that no longer shows them.

## Decision

- The drawing draws the list and the scenes the site's settings page shows, and nothing else. A scene comes back only with a page of the site that shows it.
- The command's page has no scenes and no switch between them. Each of its pages says which key in gh-desk above shows what the page changes, with the key the file binds, or says what gh-desk there cannot show.
- `mise run config-ui:test` compares the list and the confirm of a merge with gh-desk.
- Rejected: keeping the scenes for a later use. They drift from gh-desk while nothing shows them, and gh-desk itself shows them on the command's page.

This replaces the item of ADR-100's Decision "What is compared is the list, the menu of enter, the review box and the confirm of a merge. The other scenes (the keys, the popup of S, the panes) follow gh-desk by hand, and colors are not compared." ADR-109 replaced it for the command's page; for the drawing that is left, the list and the confirm of a merge are compared, the keys follow gh-desk by hand, and colors are not compared.

## Consequences

- A page added to the site's settings page that should show a scene brings the scene back into the drawing, with a comparison when the scene is one gh-desk draws from the settings.
- A page of the command's settings whose change shows only in a scene names the key that opens it.
