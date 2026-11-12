# ADR-124: How the code is shaped is in docs/design

## Status

Accepted

## Context

An ADR says what was decided and why, and leaves out how the code does it: the catalogs that grow with each feature, which part holds what, the order things happen in, the default keys (`.claude/rules/adr.md`). Some decisions are about a structure that has many members, as the popups are: the rule is short, but working on one needs to see every popup, what each holds and how they relate to the drawing, and a diagram of it. That had no place. An ADR must not keep it, the code shows it only spread over many files, and the pages of `docs/` are the site's, for users (ADR-054).

`docs/architecture.md` already is such a document for one subject, the package layers, and is not a page of the site.

## Decision

- How a part of the code is shaped (its members, what each holds, how they relate, the order of its steps, diagrams of it) is written in Markdown under `docs/design/`, one file per subject, for whoever changes the code.
- A design document describes the code as it is, and is kept true to it in the change that changes the code. A design document that disagrees with the code is fixed in place, as code comments are; it records no decision and needs no new document to change.
- An ADR keeps the rule and its reasons, and links to the design document for the shape. A design document links to the ADRs that set what it must keep, and does not restate their reasons.
- `docs/design/` is not a page of the site. This replaces, in ADR-054, the sentence "`docs/adr`, `docs/architecture.md` and `docs/site` are not pages" with: `docs/adr`, `docs/design`, `docs/architecture.md` and `docs/site` are not pages. A link to a design document from a page leads to the file on GitHub, as a link to an ADR does.
- Rejected: design documents as pages of the site. The site is for users of gh-desk; how its code is shaped is not what they come for, and pages are held to what reads well there.
- Rejected: keeping the shape in the ADR, as a Context or a section of its own. It grows with each member, and drifts from the code where nothing makes the ADR change with it.
- Rejected: a directory outside `docs/`. The design documents belong beside `docs/architecture.md` and the ADRs they link to.

## Consequences

- A change that adds, removes or reshapes a member of a structure with a design document changes the document in the same change.
- A design document may name functions, types and default values that an ADR may not; it pays for this by having to follow the code.
- [docs/site/build.sh](../site/build.sh) and the site's link rendering leave `docs/design` out of the pages, and a new directory of documents that are not pages must be added to both.
