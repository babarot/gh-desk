# ADR-058: The log is kept in memory as it is written, for gh-desk to show it

## Status

Accepted

## Context

gh-desk is to show its log while it runs, so that a batch of merges or a reading of the lists can be followed as it goes. The log is the file ADR-003 writes: JSON lines, at a level, with attributes that never carry the content of a pull request or a conversation. Showing it means reading those records back from somewhere.

The file is not always there to read. The playground runs gh-desk in the browser (ADR-047), where there is no file system and the log was thrown away. The file is also shared: every gh-desk on the machine appends to it, so its lines are not all of this run.

## Decision

- What is written to the log is kept in memory as well, by the same logger and at the same level, record for record. What gh-desk shows as its log is these records and nothing else: no text written for the screen alone.
- The records kept are the latest ones, up to a constant number. It is a constant, not a setting: it bounds memory for a run that lasts all day, and the file keeps the rest.
- A record kept in memory is timed by the clock gh-desk is given, not by the wall clock slog uses, so that what is shown can be tested at a fixed time. The file keeps slog's own time.
- Where there is no file, as in the playground, the records are kept in memory only.
- [`internal/logging`](../../internal/logging) holds the records; the TUI reads them through an interface of its own, given by `internal/app`.
- Rejected: reading the log file back (tailing it). The playground has no file, the file mixes in the lines of other gh-desk processes, and following a file needs its lines parsed and its growth watched.
- Rejected: writing what the screen shows apart from the log. It would be a second account of the same events to keep in step, and it could carry what ADR-003 keeps out of the log.

## Consequences

- Only what is logged can be shown; something worth showing is logged first, at a level the default lets through (ADR-057).
- What ADR-003 never logs is never shown either, since the records are the same.
- The records of earlier runs are in the file only.
- A logger made without the records, such as a test's, shows nothing; code that builds the logger for a run gives it the records.
