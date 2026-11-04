# ADR-065: A condition says "not" by its values, never by an operator

## Status

Accepted

## Context

The rules of `[[select]]` and `[[exclude]]` are conditions that must all hold (ADR-026, ADR-062). Rules that leave pull requests out are written to say what is not wanted, and a want often reads as a negation: checks that do not pass, a pull request that is not approved, one that is not open, one that is not a bot's. A condition took one value (`checks = "passing"`) or held only when true (`bot = true`), so none of these could be written. ADR-026 keeps expressions out of the config file, and an operator for "not" over conditions would be the first piece of one.

The review decision has no value for a pull request with none, as in a repository that requires no review, so even listing every other decision would leave those out of "not approved".

## Decision

- A condition with a fixed set of values takes a list and holds for any value in it, so "not passing" is `checks = ["failing", "pending"]`. A single value is a list of one; the file takes no bare string for such a condition.
- The values of such a condition cover every pull request, with one for "none" where the field can be empty, as the review decision of a repository that requires no review is.
- A condition that is true or false asks for the one when true and the other when false. Left out, it does not ask.
- A condition on what is the viewer's holds neither way before the viewer is known.
- Which conditions are which is in [internal/config](../../internal/config).
- Rejected: a `not` table or a `not_` prefix for every key. It is an operator over conditions, which ADR-026 keeps out of the file, and it would give each negation two ways to be written.
- Rejected: taking a bare string beside a list for `checks` and `decision`. It keeps two shapes of one key in the file, its schema and its reference.

## Consequences

- A new condition with a fixed set of values takes a list, and its values must cover every pull request; a new true-or-false condition must mean something when false.
- A config file with such a condition written as a string is reported where it is, as any value of the wrong type.
