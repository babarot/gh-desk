# ADR-052: The playground plays scenarios, and seeks by replaying them

## Status

Accepted

## Context

The playground of ADR-047 starts on the list, and a visitor finds the rest by pressing keys. The documentation needs to show one screen at a time (the diff, the rules, a merge about to be confirmed, Claude's answer) where it explains it, and to show how gh-desk gets there, not only where it ends.

A screen of gh-desk is the state of a running program against a world that moves: it cannot be saved and restored, or wound back. A recording of what was drawn could be wound back, but only over the steps already played, and it stops being the screen once a visitor presses a key.

The world answers after a pause, so that the spinners are seen, and the lists are read before any key can act on them.

## Decision

- A scenario is a named list of steps in [demo/internal/playground](../../demo/internal/playground). A step names a keymap action and is pressed with that action's first default key, as ADR-004 binds keys; only text typed into a text box is written as it is. A change of the default keys moves the scenarios with it.
- A scenario starts when the world says its first screen can be used, not after a fixed time: its steps act on the lists, and a slower machine reads them later.
- The page shows a scenario in one of two ways. Watching, nothing the visitor types or clicks reaches gh-desk, and a player shows the steps, pauses and goes to a step, as a video's controls do. Driving, gh-desk opens on the screen the scenario ends on and takes the visitor's keys at once. A visitor watching can start driving the screen shown. A frame in another page, such as the documentation, watches.
- Going to a step plays the scenario again in a new world, in the same WebAssembly module: the steps before it are typed fast, with the world answering at once, and what gh-desk draws meanwhile is held and drawn when it gets there. While the drawing is held, the terminal cannot answer gh-desk's questions about itself, and the page answers the question gh-desk's theme depends on, its background, as the terminal would.
- A step's time is of two kinds: waiting for something the world does (an answer, a popup that reads first), which a fast replay still waits for, and a moment to read, which it skips.
- `play/` is the playground on the whole window, a page of its own beside the page about gh-desk; the build writes it from the same page, so the two cannot differ.
- [demo/internal/playground/scenario_test.go](../../demo/internal/playground/scenario_test.go) plays every scenario through the real TUI, wired as the page wires it, and checks that each reaches its screen.
- Rejected: recording what gh-desk draws and replaying the recording. It cannot go to a step not yet played, and what it shows is no longer gh-desk once the visitor drives.
- Rejected: starting the steps after a fixed time. Too short on a slow machine, it acts on lists not read yet; long enough for every machine, it makes every visitor wait.
- Rejected: starting a new module for each seek. Most of the time of a seek was the world's pauses and the steps, not the module, and a new module draws the way there unless the screen is covered.

## Consequences

- A change to the TUI that changes the screen a scenario reaches, or the way there, fails the scenario test until the scenario follows it.
- A step that waits for the world says so, apart from a moment to read, or a seek skips the wait and the next step acts too early.
- Should gh-desk ask the terminal another question as it starts, one its drawing depends on, the page has to answer that one too while a seek holds the drawing.
- The scenarios are named in the documentation's links and frames; renaming or removing one breaks them.
