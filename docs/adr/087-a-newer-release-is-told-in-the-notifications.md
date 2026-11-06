# ADR-087: A newer release is told in the notifications

## Status

Accepted
Partly replaced by ADR-088

## Context

gh-desk had no way to say that a newer release was out. gh tells of a newer version of an extension, but on stderr after the extension exits, so a TUI left open for a day says nothing, and a Nix install is not an extension gh updates. claude-recall, another Bubble Tea program, shows a newer release on its status line for as long as it is known.

Here the status line is for the moment (ADR-025): what must be found again goes into the in-app notifications (ADR-007).

## Decision

- Only a build the release workflow makes reads gh-desk's releases. It is marked at link time; a build with `go install` or from a clone never reads them.
- A release build reads its releases at start, in the background, at most once in a constant time, kept with what was found in the state directory. The first frame never waits for the reading, and a reading that fails is told only in the log, since the release is not the user's work.
- A newer release is told once, as a notification, with how this install is updated, decided from where its executable really is: an executable in the Nix store is updated with Nix, even when gh runs it as an extension. While a newer release is known, the `?` list says so under the version.
- The first launch after an update tells of the update in a notification.
- `release.check` turns the reading and the telling off, and so does gh's `GH_NO_EXTENSION_UPDATE_NOTIFIER`: gh-desk is a gh extension and keeps gh's way of turning this off. The version that ran is kept whether or not it tells, so turning it on later does not tell of an update long past.
- The releases are read from github.com, where gh-desk is released, with gh's login when it is to github.com and without one when gh is logged in to another host only.
- Rejected: the status line for as long as a newer release is known, as claude-recall does. It is not a place for what lasts (ADR-025).
- Rejected: gh-desk replacing its own executable. Each way gh-desk is installed has its own way to update it.

## Consequences

- A new way to install gh-desk needs its place in how an install is updated, or it is told the releases page.
- The release workflow must keep marking its builds; a release built without the mark never tells of a newer one.
- [release_test.go](../../internal/tui/release_test.go) checks what is told and when.

## Not decided here

- How the release notes are read inside gh-desk. ADR-088 decides it.
