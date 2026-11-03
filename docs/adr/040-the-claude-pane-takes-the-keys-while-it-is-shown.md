# ADR-040: The claude pane takes the keys while it is shown

## Status

Accepted

## Context

ADR-038 put the conversation with Claude in place of the preview pane, with the list's keys working under it and the pane following the cursor, and ADR-039 put the focus in the text box when the pane opens. That left the pane in two states: with the focus in the text box, and without it, where the keys of the conversation (a drafted review, approve, Claude Code in the terminal, going back) shared letters with the list's and meant other things. Only Claude Code in the terminal could not be done elsewhere, and the user had to remember which state the pane was in.

## Decision

- While the claude pane is shown, it takes the keys; the list takes none. The pane stays on the conversation it was opened on.
- While nothing is on its way, the text box has the focus. The conversation's own actions are on keys the text box does not type; esc goes back to the preview.
- While Claude answers, nothing can be sent, so the text box is not shown. esc stops the answer, as in Claude Code; `chat_close` goes back to the preview while Claude goes on. An answer coming in another gh-triage cannot be stopped here, and esc goes back.
- Pointing at the list or the header with the mouse goes back to them.
- This replaces, in ADR-038, "The claude pane shows the conversation about the pull request under the cursor, as the preview shows that pull request. Moving the cursor changes the conversation shown; moving onto a pull request does not begin a conversation about it", and "its keys are read before the list's, so a key bound in both scopes is the conversation's there. The other keys of the list, moving the cursor among them, work as on the list." Its Consequence on keys the list and the conversation share goes with them.
- Rejected: keeping the text box while Claude answers, and going back with backspace in an empty box. Whether backspace erases or goes back would depend on what is in the box.
- Rejected: moving the conversation's actions to the menu of enter. Its actions are run by their keys on the list, where a drafted review's would meet the list's own review.

## Consequences

- To read the conversations about several pull requests, the user goes back to the list between them; the marks on the rows tell which have answers.
- A question cannot be typed while Claude answers.
- The conversation's actions are bound to keys that a text box does not take, and a new one must be too. The keys the list uses to scroll a page are read by the pane, so the text box does not get them.
- A `[keys]` entry for an action of the conversation that is gone is reported as an unknown action (ADR-004).
