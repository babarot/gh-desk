# ADR-022: The menu of enter offers the actions chosen

## Status

Accepted

## Context

Enter on a pull request opens a menu of what can be done to it, and the menu opens with its first action under the cursor. Which actions a user reaches for there, and so which should come first, differs from user to user: one reads a pull request with Claude or the browser before anything else, another approves and merges what the list already told enough about. ADR-014 makes what differs between users a setting, and ADR-016 says an order is given by an array.

ADR-016 also says a setting and the key action for the same feature have the same name. The key action that opens the menu is `open`, and a table named `[open]` would not say what it holds.

## Decision

- `menu.actions` lists the actions the menu offers, in the order shown. An action left out is not in the menu but keeps its key on the list: the menu is a way to reach the actions, not a gate on them. An action named twice, or none, is a mistake.
- The table is `[menu]`, after the screen it configures, not `[open]` after the key action. This is an exception to ADR-016's rule that a setting and its key action share a name: the rule is there so that one name is learned for one feature, and `open` does not name the menu.
- The actions the setting takes are those the TUI can offer; `config.MenuActions` and the TUI's list are kept the same by a test, as the themes are.
- The default order is code, decided by how a pull request is usually handled, and changes with a reviewed code change.

## Consequences

- A new action in the menu is added to the TUI's list and to `config.MenuActions` together; a user's `menu.actions` written before it does not gain it until they add it.
- More settings of the menu go under `[menu]`.
