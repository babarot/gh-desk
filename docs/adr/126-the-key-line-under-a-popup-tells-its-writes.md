# ADR-126: The key line under a popup tells its writes

## Status

Accepted

## Context

ADR-125 has a popup tell its keys in its frame, and the key line show none of them while the popup is open, except the few the popup marks as its most important. It left which keys those are undecided. Before it, the key line under a popup repeated most of the popup's keys in other words, and was the first thing cut on a narrow terminal.

## Decision

- A popup marks for the key line the keys that write to GitHub or lead on to a write: the question it asks to confirm, sending what was written on to the confirm popup, and applying a plan that opens one. No other key is marked; moving, choosing, and the popup's other keys are told in its frame alone.
- The key line under a popup always says what `esc` does there, in the same words as the popup's frame, so the way out is found where the eye goes for keys.
- The hints in the frame and on the key line come from the same declaration of the popup's parts and keys ([internal/tui/popupkeys.go](../../internal/tui/popupkeys.go)), so the two never say a key in different words.
- Rejected: marking the key that chooses in a popup with a list as well. It is the key every popup with a list has, told in the frame beside the list, and the key line would grow back toward repeating the popup.
- Rejected: marking no key, leaving only `esc`. A write is the one thing a user should not have to look for, and it is what a popup is opened for.

## Consequences

- A new popup that writes, or opens one that does, marks that key; one that does not marks none.
- ADR-125's "Which keys of which popup are marked for the key line" is decided here.
