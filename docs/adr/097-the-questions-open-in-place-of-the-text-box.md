# ADR-097: The questions open in place of the text box

## Status

Accepted

## Context

Going back through the questions one by one (ADR-096) works for the last few. To find an older one, the user needs to search them. A popup of its own would be a box over a box in the popup of a triage, and the claude pane has no room to spare. The key people know for this, `ctrl+r`, was `draft_review`.

## Decision

- `history_search` opens a search in place of the text box: in the popup of a triage the foot becomes the search, and in the claude pane the box to ask in does, and takes room from the conversation above. No box is drawn over another, so it reads the same in `C` and `i`, and neither has a frame of its own to tell the focus (ADR-079).
- The search has a text box of its own, made as any other, so an input method works in it. It finds a text anywhere in a question, ignoring case, as a question in Japanese has no prefix worth typing.
- The hits show when each was asked, the tab of a triage's, and the first line of the answer to the one chosen, which is what tells two like questions apart.
- `accept` puts the question in the text box and sends nothing: a question is asked again about the lists as they are now, and sending costs a request. `esc` leaves the text box as it was.
- `history_search` is `ctrl+r`, and `draft_review` moves to `ctrl+x`. `ctrl+x` is a key the text box does not take (ADR-040), which `ctrl+g` is not, as it selects all, and `ctrl+s` would be, but a hand used to posting with it would start a draft, which costs a request.

## Consequences

- `ctrl+r` no longer drafts a review. A `[keys]` entry for `draft_review` keeps working, and one set to `ctrl+r` takes the key from `history_search`.
- The claude pane shows at most a few hits, as it is low, and a line or so of the conversation above.
