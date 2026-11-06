# ADR-092: The config file holds what the user changed, and the manual is a command

## Status

Accepted

## Context

The file written on first run held every setting at its default, commented out, with its words above it: 391 lines by v0.10.0, for 55 settings and 67 keys. It held the `[[select]]` rules live as well, since ADR-028 kept them out of the defaults. It was the user's settings and the manual of every setting at once, and it was written once, so three things went wrong.

A setting added by a later version took its default but never appeared in a file already written, so the only way to learn of it was the release notes. The words and the defaults in the comments stayed as written: a line left commented could show a default no longer in use, and uncommenting it fixed the old value. And what the user had changed was hard to find among what they had not.

## Decision

This replaces the sentences of ADR-014 "The template written on first run ... made from the row" and "Every setting has a default, a line in the written config file", and all of ADR-028.

- The file written on first run holds its schema line and words that point to the manual, and no setting. A file already there is never written to, by first run or by an update.
- The rules of S have defaults, as tabs do. A file with no `[[select]]` has the five default rules; rules written replace all five; `select = []` leaves none. ADR-028 kept rules out of the defaults because a default cannot be taken away, which `select = []` now does, and rules left in the file would be the one thing there the user did not write.
- The manual, `gh desk config reference`, is every setting with its words and its default, made from the table of settings each time, so it is always the running version's. It is all comments under the tables' headers, so read as a file it is the defaults, which a test keeps true. It does not read the config file, so a file with mistakes does not stop it.
- A name narrows it to a table, an array of tables, `keys`, or one key of any of them.
- gh-desk finds what is new to this install by comparing the settings of the running version with the names kept in `release.json`, at every start of a release build. It tells of the new ones in one notification, a table or an array of tables new as a whole by its name, and keeps each by name, with the version it updated from, so that the manual lists and marks them until an update adds others. `release.json` now holds these names besides what was found of the releases.
- The arrays of tables are listed in one place (`arrayTables` in `internal/config`), which the names, the manual and the configuration page read, and a test keeps it in step with their keys.
- A record with no names takes the settings as known and tells nothing: a new install, or the update to the first version that keeps them. Names known stay known, so going back a version and forward again tells of nothing twice. Key actions are left out: they come with their keys and appear in the `?` list.
- Rejected: writing new settings into the file as comments, in place or by a command. A file managed elsewhere can be read-only, and the comments would still go stale.
- Rejected: comparing the file with the table to find new settings. Nearly every setting is missing from a short file.
- Rejected: a `Since` version on each row of the table, written by hand. It makes the author guess the next version, and a wrong guess shows the wrong version and can tell of a setting twice.
- Rejected: the same `Since` written as a placeholder and replaced in the release pull request by a tagpr hook. A release bot would rewrite source files with a failure tagpr v1.17.1 does not report, and the release path would need a check of its own, all for one line of text.
- Rejected: a flag or command that lists the new settings. The marks in the manual do that, and they belong to a command the user already has reason to run.

## Consequences

- Opening the file no longer shows what can be set; its head points to the manual.
- Changing a setting means reading the manual and writing the line, rather than uncommenting it.
- A file written before this goes on working as it is, stale comments included, and keeps the rules it was given. They match the defaults until a default changes, which then does not reach that file.
- Adding one rule to a file takes the five default ones away, as adding one tab does with the tabs. The manual and the guide say so, and the rules are in the manual to copy. `gh desk config show --diff` prints a short one, which a test keeps equal in effect, without the user's comments.
- The update to the first version with this tells of nothing, so what that version adds is left to its release notes.
- A changed default is not told of; it applies by itself and is left to the release notes.
- Each install keeps its own names, so two machines are told once each. A build that is not a release keeps none and tells of nothing; `config reference` reads what a release left in `release.json`, whichever build runs it. A `release.json` lost starts over as a record with no names.
