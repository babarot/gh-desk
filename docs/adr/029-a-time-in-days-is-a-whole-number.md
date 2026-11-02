# ADR-029: A time counted in days is a whole number of days

## Status

Accepted

## Context

ADR-016 says that a duration in the config file is a Go duration string. Go durations have no unit longer than an hour, so a time counted in days would be written as `"2160h"` for 90 days, and a unit of its own (`"90d"`, `"3mo"`) would need a parser of its own; a month is not even a fixed length. `list.stale_days` already took a whole number of days, against that sentence, and the `[[select]]` keys `updated_within_days` and `created_within_days` count days as well.

## Decision

This replaces one sentence of ADR-016, "A duration is a Go duration string, and `"off"` turns off what it times", for times counted in days; it stays for the rest.

- A time counted in days is a whole number of days of 24 hours, not a duration string, and its key's name ends in `_days`, so the unit is in the name.
- Where such a time can be off, it says so with `"off"`, as ADR-016 has it. Where leaving the key out already means no limit, as in a `[[select]]` rule, there is no `"off"` and 0 is a mistake.
- A time shorter than a day, or one that is not a whole number of days, stays a Go duration string.

## Consequences

- A setting counted in days cannot be given in hours; one that needs both is a duration string.
- "Three months" is written as a number of days, not as calendar months.
