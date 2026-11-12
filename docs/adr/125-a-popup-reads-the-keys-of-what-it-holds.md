# ADR-125: A popup reads the keys of what it holds

## Status

Accepted

## Context

A popup takes every key while it is open (ADR-079), and one is on the screen at a time (ADR-121). Each popup read its keys in a function of its own, so the same kind of part took different keys in different popups: a list moved on keys in one popup that did nothing in another, the key that opened a popup closed it in some and not in others, and `esc` threw away a review being written at once while elsewhere it only left a text box. `q` closed some popups and did nothing in others, one press away from quitting gh-desk on the screen under them. The hints of a popup were written by hand in each, and said the same keys in different words from the key line, which the keymap makes.

Popups also stood in different places: most from a row near the top, two centered from top to bottom, which moved when their height changed. Their widths came from a formula of their own each, so a popup changed width when it led to another.

Which popups there are, what each holds and how they are drawn is in [docs/design/popups.md](../design/popups.md) (ADR-124).

## Decision

### Keys come from the parts a popup holds

- A popup declares the parts it holds, from a few kinds: a list to choose from, a body to read, a text box, a question that writes to GitHub. A layer shared by every popup reads the keys of those parts, the same in every popup; a popup reads only the keys that are its own.
- While a text box has the focus, every key that types a character goes to it before anything else, so no key of the popup can take a letter being typed (ADR-066). Of the keys that type nothing, the popup's own keys come first, as the popup declares them for the state it is in, so that a popup can keep a key the text box would otherwise take; then an action that claims a nav key (below), which passes the key on when it has nothing to do with it; then the nav keys the text box uses for editing, which go to it. The popup's other parts, a list in it among them, read only what is left, and every key that nothing took goes to the text box, so its own editing keys (deleting, moving along a line) reach it whatever they are.
- While a text box has the focus, a question that writes to GitHub is not read: no key confirms a write from inside a text box, whatever key the question is bound to (ADR-066). A popup declares its parts for the state it is in, and leaves the question out while its text box has the focus.
- A list and a body move on the same actions, nav's (ADR-004): one moves its cursor, the other scrolls. Every popup with either reads all of them.
- One action enters a popup's text box when it does not have the focus, the same in every popup.
- The hints of a popup are made from the parts it declares and its own keys, through the keymap. No popup writes its keys by hand.
- A popup tells its keys in its frame. The key line, while a popup is open, shows none of them except the few the popup marks as its most important actions, which it shows in its frame as well. ADR-024 keeps a key told elsewhere off the key line; this replaces that rule for a popup's marked keys, so that the action that matters most can be found where the eye goes for keys, whatever the popup.
- Rejected: the key line repeating every key of the popup, as it did. It is cut first on a narrow terminal, and it said the popup's keys in other words than the popup did.
- Rejected: leaving each popup to read its keys, with a test that they agree. It checks agreement after the fact, and every new popup would again be written by hand.

### What steps out, and what quits

- `esc` steps out one level in every popup, from the innermost one the popup has at that moment: a list of suggestions or a search opened over a text box (ADR-097, ADR-098), a request on its way (ADR-095), the text box, then the popup itself, which goes back to what it was opened from (the popup it took the place of, ADR-121, or the screen). The popup declares which of these levels it has. What was written in a text box is thrown away only when its popup is closed.
- A popup whose text box has the focus as long as the popup is open, with no keys of its own without it, has no level between the two: `esc` closes it at once.
- A popup is closed by `esc` alone. The key that opened it does not close it: inside the popup, that key is free for the popup's own use, as any key is, and means the same in no other popup by accident.
- Rejected: the key that opened a popup closing it too, as `?` did the `?` list. It held only where no part of the popup read that key first: the menu's `enter` chooses, a text box types its letter, a popup opened by several actions has no one key, and a popup holding what was written would lose it. The user would have to learn popup by popup where it closes, and a key pressed twice would close what it had just opened.
- `quit` quits gh-desk and does nothing else. It is not read while a popup is open: it never closes a popup, and never quits from one. Stepping back with `esc` never quits, however often it is pressed, and the two are kept apart so that closing a popup cannot quit by mistake.
- Rejected: `quit` closing a popup that has no text box with the focus. The same key would close in one place and quit one press later.

### Moving keys are one set, and a part may claim some of them

- nav's up and down hold the keys that terminal programs commonly move by in a list, including those a text box uses for its history; moving is set once, under nav, and works the same in the list, the panes and every popup.
- An action may claim the keys of one nav action for a meaning of its own where both are read: recalling the questions asked before claims the keys of moving up and down. A claim names the two actions, and is declared in [internal/tui/keymap](../../internal/tui/keymap); the keymap's check lets the two share keys and nothing else, and any other nav key used by another action stays a mistake in the config file. This replaces, in ADR-004, the words "`global` and `nav` keys are read everywhere, so they may not be reused" for the nav actions claimed: they may be reused by the action that claims them.
- Rejected: a scope claiming every nav key at once. Any action of that scope could then take a moving key, and which of the two a key does would depend on where the cursor is.
- Rejected: separate actions for moving in a popup. The same keys would be set twice, and a popup would again move on keys the list does not.

### Where a popup stands

- Every popup is drawn from the same row near the top of the screen and grows downward, scrolling past the height it may take. None is centered from top to bottom: a popup whose height changes keeps its title and text box in place, and a popup that takes the place of another opens where it stood.
- A popup is one of two widths, narrow for a short list to choose from and wide for anything to read or write, and popups that lead from one to another are the same width where they can be. The widths are constants, not settings: they only keep popups alike.
- Rejected: centering every popup from top to bottom. It moves whenever its height changes.

## Consequences

- A new popup declares its parts and reads only its own keys. It does not handle moving, `esc` or entering its text box, and does not write its hints.
- An action of a popup must not take a key of nav or of a part the popup holds, unless it is declared as a claim.
- The actions that entered a text box in one popup each give way to one action. As ADR-023 decides for every renamed action, a key set under an old name is a mistake that names the new one, and gh-desk does not start until the config file is changed.
- The notifications, the picker and the `?` list are closed by `esc`, no longer by `quit` or the key that opened them, and throwing away a comment or a review being written takes `esc` twice.

## Not decided here

- The name of the action that enters a text box, and of the scopes the shared layer reads.
- Which keys of which popup are marked for the key line.
- Whether the claude pane and the log viewer, which take the keys but are not popups, read the shared layer.
