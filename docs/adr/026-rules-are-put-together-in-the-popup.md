# ADR-026: Rules are put together in the popup of S, not written as expressions

## Status

Accepted

## Context

S selects the pull requests a rule matches, so that A and M act on them together. A rule is a `[[select]]` in the config file, and a pull request matches it when it meets every condition the rule sets. The popup chose one rule at a time and replaced the selection with what it matched, so two rules could not be put together: "approved by me" and "small" had to become one more rule in the file, written for that one combination, and "renovate or dependabot" could not be had at all.

Two ways were weighed. Rules could be written as expressions in the config file (`A and (B or C)`), which says anything but needs a grammar, its errors reported where they are (the config rules in CLAUDE.md), and an edit of the file for each new combination. Or the popup could put the rules already written together while it is open, which needs no grammar and keeps each rule small.

## Decision

- Rules are put together in the popup of S, as the user goes, and not in the config file. The config file has no expressions over rules; a rule stays a set of conditions that must all hold.
- The popup makes one set of pull requests from the rules chosen: those the user has checked, which must all match (AND), or with none checked the rule under the cursor. Choosing replaces the selection with that set.
- OR is had by adding a set to the selection instead of replacing it, so (A AND B) OR C is chosen in two steps. Within one rule, a condition that takes a list allows any value in it, as the authors and the packages do.
- The checks last while the popup is open. A combination that is used again is written as one rule in the config file, which the keys `approved_by_me` and `max_lines` make possible for the usual ones.
- A rule that reads the heavy fields does not match a pull request whose heavy fields are not read for its head, whichever rules it is put together with (`Rule.NeedsHeavy`).

## Consequences

- Any combination of the rules written is one popup away, but a combination is not kept; one used often has to be written as a rule.
- A shape that needs OR inside AND, such as A AND (B OR C), is made by writing B OR C into one rule where a condition takes a list, or not at all. Adding expressions later is a new ADR.
- What the popup makes is a set of pull requests; a new use of it (another way to act on the set) belongs in the popup with the others.
