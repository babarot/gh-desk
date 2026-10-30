---
paths:
  - "docs/adr/**"
---

# ADR Structure

ADRs (Architecture Decision Records) follow this template:

```
# ADR-NNN: {Title}

## Status

Accepted | Deprecated | Superseded

## Context
## Decision
## Consequences
```

Rules:
- Filename: `{NNN}-{kebab-case}.md` (e.g., `001-new-extension-not-a-gh-news-fork.md`)
- H1: Must start with `ADR-NNN:` and the number must match the filename prefix
- Required H2s: Status, Context, Decision, Consequences
- Status value: Must be exactly `Accepted`, `Deprecated`, or `Superseded`
- Additional H2s (e.g., `## History`) are allowed
- Write an ADR for a decision that sets the direction of the code and constrains later changes, in the commit that implements it or the one before. To change a decision, write a new ADR and mark the old one `Superseded`.
