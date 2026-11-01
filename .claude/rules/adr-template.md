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
- Write an ADR for a decision that sets the direction of the code and constrains later changes, in the commit that implements it or the one before. To change a decision, write a new ADR. When it replaces the old decision whole, mark the old one `Superseded`. When it replaces only part of it, name in the new ADR the sentences it replaces, and leave the old one `Accepted` (as ADR-015 does for ADR-005, and ADR-016 for ADR-014 and ADR-015).

# What an ADR holds

An ADR is not a specification. It records what was decided, why, and what was rejected and why, so that a later change knows what it must keep. How the code behaves today is in the code, its tests and the `?` list.

- Write a detail only when it is the decision itself: a guarantee others rely on (nothing written to GitHub without a confirm, a head checked before every write), a safety measure (the flags Claude runs with), a name that users see in the config file or the keys, or an alternative that was rejected. Ask of each sentence: if this changed, would it need a new ADR? If a reviewed code change would be enough, leave it out.
- Leave out how the screen behaves: which click or key does what, layouts, widths, the order of items, the text and format of what is shown. Write the rule that the behavior follows ("the mouse does not write to GitHub"), not the behavior ("a double click opens a row").
- Leave out the values of constants (sizes, thresholds, intervals, durations, batch sizes). ADR-014 makes them code; say that a value is a constant, and why it is not a setting, without giving it.
- Do not copy a list that another place keeps: the package table (`arch_test.go`), the settings (`config`), the default keys (`keymap`), the log attribute names (`internal/logging`). Name the place instead.
- Do not keep a catalog that grows with each feature, such as every setting or every link in the pane.
- Consequences are the costs and the constraints the decision leaves on later work, not a list of what was implemented.

When an ADR states a detail that this section leaves out, remove the detail from it; that is not a change of decision and needs no new ADR. A mismatch between an ADR and the code over such a detail is a sign of one.
