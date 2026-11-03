# ADR-053: Popups take the mouse, and it still writes nothing

## Status

Accepted

## Context

ADR-012 has a popup take no mouse input, so that a click never acts behind what has the focus. The `?` popup was given the mouse later without a decision saying so: its wheel scrolls it, a click folds a section, and a click outside closes it. The other popups took none. A user who opens a pull request with a double click lands in the menu of enter and has to move to the keys to pick an action, the wheel did nothing over a popup that scrolls on a short screen, and the only way out of a popup was esc. One popup taking the mouse and the rest not is a rule the user has to learn popup by popup.

What ADR-012 guards against is a click acting on what is behind the focus. A popup that takes the mouse itself, and swallows what it does not use, keeps that guarantee as well as one that takes none.

## Decision

- Every popup takes the mouse, and mouse input given to a popup never reaches what is behind it. This replaces the words "A popup" in ADR-012's sentence "A popup, the filter being typed and the review being written take no mouse input"; the filter being typed and the text of a review or a comment being written still take none, so that a click does not throw away what was typed.
- In a popup the mouse does what it may elsewhere (ADR-012): it scrolls, moves a cursor, chooses, and closes. A click outside the popup closes it, as esc does.
- A choice in a popup is made by a second click on the same item, so that a single stray click does nothing, as a link waits for a second click (ADR-035).
- Nothing in a popup that writes to GitHub is done with the mouse: the key that confirms a popup which writes stays a key, and a choice that writes at once, with no popup to confirm it, is not made by a click. ADR-012's rule that the mouse does not write to GitHub stands whole.
- A request to Claude on its way is not stopped by a click outside its popup; it is stopped by its key.
- Rejected: choosing on one click. A popup opens with its cursor on an item, and the click that lands on it would choose it before the user saw what it is.
- Rejected: a click only moving the cursor, with enter to choose. It leaves the hand going back to the keys, which is what this decision is for.
- Rejected: keeping the mouse to the menu of enter. It would leave which popups take the mouse to be learned one by one.

## Consequences

- A new popup takes the mouse through [mouse.go](../../internal/tui/mouse.go), in the order keys go to the popups, and has to tell the mouse where it was drawn and which item each of its lines shows.
- A new choice in a popup that writes to GitHub without a confirming popup has to be kept from being made by a click; [mouse_test.go](../../internal/tui/mouse_test.go) checks that marking done from the menu and confirming a merge are not.
- The mouse finds an item from where the popup was last drawn, so a change to how a popup is drawn has to keep that in step, as ADR-012 says of the list.
