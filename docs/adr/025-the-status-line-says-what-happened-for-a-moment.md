# ADR-025: The status line says what happened for a moment, and what is going on while it lasts

## Status

Accepted

## Context

The status line is one line under the list. Anything wrote a string into it, which stayed until something else wrote another. Work in progress was written the same way, so a line such as "approving 3 pull requests" stayed after the batch ended, and a message written meanwhile hid that the work was still going on. A few kinds of work in progress were built from the state instead, but an old error, kept until the next message, hid them as well, and each of them gave way to the other messages by a rule of its own. Moving the cursor wrote the result of the row under it over whatever was said, an error included. Failures that arrived while the user was not looking were lost at the next message, although ADR-007 keeps what may be missed in the notifications.

## Decision

This adds to ADR-007; it replaces none of it.

- The status line is a place for the moment. What must be found again goes into the notifications (ADR-007), whether or not the status line also says it.
- What the status line shows is of three kinds, each kept apart in the code:
  - An event: something that happened, said once. It is a success or other information, a refusal (the key pressed cannot act here, and why), or a failure. A refusal is drawn apart from a failure, which alone means that something went wrong.
  - Work in progress: built from the state on every frame, never written. It shows while the work lasts and goes when it ends; it may count its progress.
  - The result of the row under the cursor, built from the row.
- An event lasts a time that depends on its kind, a constant rather than a setting (ADR-014); a refusal also goes at the next key. Work in progress has no time: it ends with the work. An event that ends lets what is under it show again.
- The line is taken by, in order: the newest event still in its time, except that information gives way to a failure still in its time so that a success arriving after it does not hide it; the result of the row the cursor has just moved onto, for as long as an event lasts, since moving the cursor is the user's doing; work in progress, every piece of it at once; then the row's result for as long as the cursor stays on it.
- When work in progress ends, the status line says so only when nothing else on the screen does. A failure that may arrive while the user is not looking is also kept in the notifications. A failure to read a list again is kept only when the reading before it went through, so a network that stays down does not fill the notifications as the clock reads the lists again.
- Work the clock starts, such as reading the lists again, is not shown as work in progress.
- What the line shows is decided in a package of its own, `internal/statusline`, whose state only its methods change. The compiler, not review, keeps anything else from writing to the line; an AST check in a test, tied to a field's name, was rejected as easy to slip past.

## Consequences

- Nothing on the status line stays to look current after it is over, and nothing written there hides that work is still going on.
- New work in progress is added where the others are listed, not written into the line; a message written into the line must be something that already happened.
- A message that the user must not miss cannot rely on the status line alone.
