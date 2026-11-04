# ADR-067: A comment gh-desk sent is shown without reading again

## Status

Accepted

## Context

The description pane shows a pull request's comments (ADR-013), read once when the pull request is first looked at and kept until `r` (ADR-007). A comment gh-desk sent, with `c` or with close (ADR-066), did not appear there: the pane kept what it had read before, and the user saw the comment only after `r`.

Everything the pane shows of a comment is known when GitHub says it took it: its author is the user, its text is what was sent, and its time is now. Reading the pull request again would show the same, at the cost of a read for each pull request and of the pane's parts going away while it is read.

## Decision

- When GitHub has taken a comment gh-desk sent, the comment is added to what the pane holds of that pull request, as the user's and as of now, and cleaned as a comment read from GitHub is. It is not read again.
- When the pull request is being read at that moment, the read is thrown away and done again: it may have left before the comment was taken, and its answer would hide the comment.
- When the pull request has not been read, nothing is added; the comment comes with the first read.
- `r` reads what the pane shows again as before, so a comment added this way is replaced by GitHub's own.
- A comment that GitHub refused is not added.

## Alternatives

- Rejected: reading the pull request again after each comment. It is a read per pull request for what is already known, and the pane shows that it is reading in place of the description, checks and comments meanwhile.
- Rejected: showing nothing until `r`. The user who just wrote a comment looks for it, and not finding it reads as not sent.

## Consequences

- A comment added this way is drawn from the text that was sent, not from what GitHub made of it; a mention or a reference GitHub expands is shown as typed until `r`.
- Anything else that sends a comment has to add it the same way, or the pane will lag behind for that one.
