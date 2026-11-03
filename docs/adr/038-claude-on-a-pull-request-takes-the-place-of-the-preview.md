# ADR-038: Claude on a pull request takes the place of the preview

## Status

Accepted

## Context

The conversation with Claude about a pull request (ADR-031, ADR-032, ADR-036) was a screen of its own: opening it replaced the list and the preview pane. While it was open the user could not see the other pull requests, nor move to one, and had to leave the conversation to look at the list again, though Claude went on answering in the background.

Three other places for it were tried in mock-ups and rejected:

- A tab of the preview pane, beside the overview and the diff. The preview shows what GitHub knows about the pull request and is only read; the conversation is a different kind of thing, which takes typed questions, runs in the background, tells when it is answered and writes to GitHub. As a tab it would also hide behind the diff, and keys would mean different things in different tabs of one pane.
- A pane of its own beside the preview. Both lose width, the overview and the diff would have to be laid out again for less of it, and the user would move the focus between panes.
- A popup over the list, as Claude on the list is (ADR-018). It covers the list much as the screen did.

## Decision

- The conversation is shown in a claude pane that takes the place of the preview pane: the same place and the same height, so that a height set for one, with the mouse or with keys, is the other's. The list stays above it.
- The claude pane shows the conversation about the pull request under the cursor, as the preview shows that pull request. Moving the cursor changes the conversation shown; moving onto a pull request does not begin a conversation about it, as beginning one makes a worktree (ADR-036).
- While the claude pane is shown, its keys are read before the list's, so a key bound in both scopes is the conversation's there. The other keys of the list, moving the cursor among them, work as on the list.
- esc stops what Claude is answering, as it does in Claude Code and in Claude on the list (ADR-018). With nothing on its way, esc goes back to the preview, as it steps back one level everywhere else. A key of its own (`chat_close`) always goes back to the preview. Going back does not stop Claude.
- An answer stopped by the user is kept in the conversation as stopped, not as a failure. Claude Code is interrupted as on quitting, and a session that never answered is made anew at the next question, as after a failure.

## Consequences

- The conversation has the preview pane's height, which is less than the screen it had. A long answer is scrolled, or the pane made taller.
- The overview and the diff cannot be seen beside the conversation; going back to the preview and opening the claude pane again keeps the conversation.
- A key that the list and the conversation share means the conversation's action while the claude pane is shown. A new action of the conversation must be checked against the list's keys for what it hides.
- esc has two meanings in the claude pane, which the key line tells: stop while Claude answers, and back otherwise.
