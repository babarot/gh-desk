# ADR-028: The config file is written with rules to start with, which are not defaults

## Status

Accepted

## Context

The config file written on first run holds every setting at its default, commented out, so that a default changed later still applies until the user picks a value (ADR-014). The `[[select]]` rules of S were in it only as commented examples, so S offered nothing but the built-in "approved by me" until the user wrote rules, and the conditions a rule can have were learned from comments.

A rule is not a setting with a default. A default applies when the file says nothing, so rules given as defaults could not be taken away: deleting them from the file would bring them back.

## Decision

This adds to ADR-014; it replaces none of it.

- The template writes a few `[[select]]` rules live, not commented out, as rules to start with. The user deletes or comments out the ones not wanted.
- They are not defaults: `config.Default()` has no rules, and S offers only the rules in the file, besides the built-in "approved by me". A file with no `[[select]]` has no rules.
- The template says so beside them. The rules themselves, and which conditions they show, are the template's, not a decision.

## Consequences

- A rule to start with changed later reaches only files written after the change; files written before keep what they were given, as with anything the user writes.
- The test that reads the template as the defaults leaves the rules out, and checks them apart.
- Another part of the file written live would need the same reasons: something the file must be able to take away, which a default cannot.
