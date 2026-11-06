# ADR-096: The questions of a triage are kept, and recalled from the text box

## Status

Accepted

## Context

The same few questions get asked of a triage again and again, such as the bots with passing checks, each morning. Nothing kept them: ADR-031 keeps the conversations about pull requests, but said that a triage is not kept, as its session lasts while gh-desk runs. A question had to be typed anew, and neither text box could bring one back.

## Decision

- The questions of a triage are kept under `triage/<host>` in the state directory, with the tab each was asked on and the first line of its answer, cut short. This replaces, in ADR-031, that a triage is not kept. The rest of a conversation is not: no plan and no session, so a question recalled is a new question, about the tab shown.
- A question is written as it is sent and the line of its answer when that comes, so one cut short by quitting stays, and one stopped can be asked again. The answer's line is written after its question's: reading drops a line of answer with no question.
- The file is lines added to its end. Any write holds a lock on a file of its own, which is never replaced, and opens the history only then: the history is replaced when it is read, so a lock on it, or a file kept open, would miss another gh-desk's write and lose it.
- Reading leaves the newest of each question, at most a fixed number, and writes the file back as that. The number is a constant, as ADR-014 decides. A line of another format is not read, and leaves the file as it is, so an older gh-desk does not lose what a newer one wrote.
- A failure to read or keep them is told, and asking goes on.
- `i` recalls the questions of its pull request, which its conversation keeps already, but for the review it drafts, as what is sent for that is another text than what is shown.
- `history_prev` and `history_next` recall in the text box. The arrows do too, as nav's `up` and `down`, which cannot be set in another scope (ADR-004). They recall only when the key is not typed text, as nav's `up` and `down` are also `k` and `j`.
- `ctrl+p` and `ctrl+n` are keys the text box takes, for moving a line (ADR-040 keeps a conversation's actions on keys it does not take). They recall only on the box's first and last rows, where the box has no line to move to, and move the cursor otherwise.
- What is typed is set aside while going through the questions, and comes back past the newest. Editing a question recalled leaves it as text of the user's.

## Consequences

- What the user asked, and the start of what Claude answered, stay on disk until pushed out by newer questions. Titles and keys of pull requests may be among them, as in the conversations of ADR-031, and the file is kept as those are, readable by the user alone.
- Each read and write takes the lock, so they run away from the screen, and the questions come a moment after the first `C`.
- Two gh-desk see each other's questions at the next launch, not at once.
