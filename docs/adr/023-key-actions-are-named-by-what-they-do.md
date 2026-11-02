# ADR-023: Key actions are named by what they do

## Status

Accepted

## Context

ADR-004 binds keys by action name, and the names are what a user writes under `[keys]` and `menu.actions`. They grew one feature at a time, and some no longer say what they do: one says only that something is opened, one that something is toggled, one names Claude without saying which of the two ways to ask it. Some name the same thing two ways: the preview pane is `[preview]` in the config file but `pane` in its actions, and the follow-up question to Claude is named after its scope in one screen and not in the other.

ADR-004's Context says irreversible actions use capital letters. The keys do not follow that: comment is lowercase and writes to GitHub after it is written, while several capitals only move or show things. What every write does follow is that it passes through a confirm popup, or a compose screen and then a confirm popup, before anything is sent.

A confirm popup can be answered only with its own key. `confirm` shares a scope with `choose`, the enter of the menu and the other popups that pick from a list, so enter cannot also confirm without the two meeting, though no popup reads both.

## Decision

- An action is named by what it does, with its object when that is not plain from the scope.
- An action and the setting for the same feature have the same name, as ADR-016 says; the actions of the preview pane are named `preview`.
- An action is prefixed with its scope's name only when another scope has an action of the same meaning, and actions of the same meaning have the same default key.
- No key writes to GitHub on its own: a write passes through a confirm popup, or a compose screen and a confirm popup. A capital letter marks a heavy action as a hint, not as a rule. This replaces the sentence of ADR-004's Context on capital letters.
- One exception is kept: hiding a row of the inbox marks its notification done at once. Emptying the inbox is done many times in a row, and marking done touches only the user's notification, not the pull request.
- Confirming and choosing are separate scopes, since no popup reads both. The confirm popups take enter as well as their own key, so a popup that shows what will be sent is answered with the same key as the rest of the screen. A confirm popup that reads what it will show before it can be answered, as those of approve and merge do, takes no confirm until it has.
- An action that is renamed keeps its old name in a table in `internal/keymap`. A `[keys]` entry under an old name is an error at its line that names the new one; the old name is not read, as ADR-016 has for settings.

## Consequences

- A new action is named by these rules, and a new write needs a confirm popup; another exception like the inbox's is decided in a new ADR.
- Renaming an action costs every user who set its keys one edit, which the error at the line tells them.
- With enter on both, choosing a write in the menu and confirming it are two presses of the same key; what keeps it a confirmation is that the popup shows what will be sent before it takes a key.
