# ADR-111: A condition left empty sets none

## Status

Accepted

## Context

A rule matches the pull requests that meet every condition it sets, and a rule that sets none matches every pull request: the file reports it, and the settings page stops it before gh-desk takes it (ADR-109). A list left empty, such as `authors = []`, already set no condition.

A title did not follow. Adding a title to a rule on the settings page gave it an empty one before anything was typed, and an empty regular expression matches every title: a rule of `[[exclude]]` hid the whole list the moment its title was added. The page counted the empty title, and an empty list, as a condition, and the file read `title = ""` as one without a word.

## Decision

- A condition left empty sets none: a list with no item, and a title of no characters. The file reads it as not written, and a rule whose conditions are all empty is reported as one that matches every pull request.
- The settings page judges a rule the same way, and gives gh-desk no file with such a rule, so that gh-desk keeps the rules applied before while one is being written.
- Rejected: reading an empty title as a pattern that matches every title, as a regular expression does. No one writes `title = ""` to mean every title; a rule that means every pull request has no reason to exist.

## Consequences

- A condition that gains a value that can be empty must count the empty one as none, in [internal/config](../../internal/config) and in the page's check ([internal/config/ui/check.mjs](../../internal/config/ui/check.mjs) checks the page), or a rule of it alone matches every pull request again.
