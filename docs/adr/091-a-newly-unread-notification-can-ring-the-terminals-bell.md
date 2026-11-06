# ADR-091: A newly unread notification can ring the terminal's bell

## Status

Accepted

## Context

gh-desk reads the inbox again on every automatic refresh and on `r`, whichever tab is shown, so it learns of new notifications while it runs. Nothing said so unless the inbox's count in the header was looked at: gh-desk left open in a pane out of sight gave no sign of new work.

A terminal has its own way to call for attention: the bell (BEL). Each terminal decides what it does with it, a sound, a flash, a mark on its tab or window, or nothing, and users already set that up for their shells and editors.

## Decision

- `list.bell` rings the terminal's bell when a reading of the inbox finds a notification newly unread: one that was not unread in the reading before it, or one whose time moved on since. It is off by default: a program that makes a sound on its own surprises those who did not ask for it.
- The bell is the terminal's BEL written to the screen, and nothing else: no sound file, no desktop notification, no command run. What it looks or sounds like is left to the terminal.
- What gh-desk last read is what a reading is compared with, not what is shown: a notification put away here while its mark of done is on its way is not new when a reading still returns it.
- The first reading from GitHub in a run only sets what later readings are compared with. What came while gh-desk was not running is in the inbox's count when it starts, and a bell then says nothing that is not already on the screen.
- A notification the inbox excludes, by a rule of `[[exclude]]` or as merged or closed (ADR-062), rings no bell, even while the excluded are shown: the exclusion is the user saying it does not call for attention. The bell waits until the pull requests of a reading are read, since whether one is excluded rests on them.
- A reading rings the bell at most once, however many notifications it finds newly unread.
- Rejected: a desktop notification. It needs an outside command or API for each platform, and a terminal that shows the bell as a notification already does it.
- Rejected: ringing for every tab's new pull requests. The inbox is what GitHub says calls for the user; a search tab's rows change with every push.

## Consequences

- The bell asks of an exclusion what the inbox's rows ask: a new way to exclude a notification that the rows learn of elsewhere would still ring it.
- The bell rings only as often as the inbox is read, so `list.refresh` set to `off` leaves it to `r`.
- [bell_test.go](../../internal/tui/bell_test.go) checks when the bell rings and when it does not.

## Not decided here

- Telling what is new in the in-app notifications as well as by the bell.
