# ADR-088: Release notes are read from the keys list

## Status

Accepted

## Context

ADR-087 tells of a newer release and of an update in the notifications, which last only for the session and hold one line each. Release notes listed in a notification would be gone once gh-desk quits, and its items, which name no pull request, read as warnings of the config file.

Reading the release notes is not something done from anywhere: it comes after being told of a release, or out of curiosity, while looking at gh-desk itself.

## Decision

- This replaces the sentence of ADR-087 "Only a build the release workflow makes reads gh-desk's releases." Only a release build reads them on its own, to tell of a newer release; any build reads them when the user opens the release notes, and a build that is not a release is told the releases page as how to update.
- The release notes are shown in a box opened with a key from the `?` list, and from there only. The key is an action of a scope of its own, read while the `?` list is open, so it does not take a key from the screens.
- A scope that also reads another scope's action declares it, and the keys are checked against it as against its own: the `?` list reads the list's fold. The check is in [keymap.go](../../internal/tui/keymap/keymap.go).
- The box shows every release read, newest first, the one running marked installed and a newer one not installed with how this install is updated. A build that is not a release marks none, since it names the last release but its code may be newer; the box says what the build is instead. The notes are GitHub's own, shortened, and are read from GitHub, the only place they are kept; gh-desk keeps them for the session.
- They are read at most once in a session: with the reading at start, when it runs, as the `?` list opens when there is a newer release or an update to read about, and otherwise when the box opens. A failed reading says so in the box with where the releases are, and opening it again reads again.
- With release.check off, nothing is told, and the releases are read only when the box is opened: the user asks for them.
- Rejected: release notes as the items of the notifications. They last for the session and draw as warnings.
- Rejected: a key that opens the box from anywhere. It would take a key from every screen for something done rarely.
- Rejected: reading the releases each time the `?` list opens. It is opened often to look up a key.

## Consequences

- A notification that points to the release notes names the keys of both the `?` list and the box.
- A scope that reads another's action must declare it, or a key set in the config file can match both without being reported.
