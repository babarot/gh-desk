# ADR-033: A part of the TUI with state of its own is a package under it

## Status

Accepted

## Context

`internal/tui` is the largest package by far, and nearly all of its state is fields of one `Model`, with methods in every file of the package. The files are split by screen, but nothing tells which part of the TUI owns which state, or which code may change it: inside one package every unexported field can be written from any file.

The conversations with Claude (ADR-031) showed this most. Their state (the session, the turns, the question on its way, the lock and the revision of what is kept) has rules of its own: it is written only under its lock, a question is kept before it is answered, a conversation that could not be written is left alone. Once that state was gathered into one type inside `internal/tui`, `Model` still wrote some of its fields directly, and the type still reset some of the screen's.

## Decision

- A part of the TUI that holds state of its own, with rules of its own, and that needs no terminal is a package under `internal/tui`, named after that part. The conversations with Claude are the first, `internal/tui/chat`.
- Such a package holds the state and the rules: what is kept, the locks, how a question moves along. Its fields are unexported; the TUI reads them through methods and changes them only through the methods the package gives. How the state is drawn, what the keys do, and which conversation is on screen stay in `internal/tui`, which keeps the screen's own state (the text box, the scroll, the screen to go back to) next to the package's type.
- The package is a row of the layer table in `arch_test.go`, like any other. Its layer follows from what it imports, not from its directory: it is below `internal/tui` because the TUI imports it, and the table forbids it Bubble Tea. Go's `internal` directories are not used to restrict who imports it, as the table already says that once.
- It reports what went wrong through an interface it defines, which the TUI implements with the status line, so it does not depend on the TUI.
- Rejected: moving the files that import no Bubble Tea (Markdown, HTML, styles, themes, spinners) out as they are. Most of them are methods that use the `Model`'s styles, and moving them would only export the styles back and forth. Rejected too: directories that only group packages (`internal/core`, `internal/infra`), which would say the layers a second time beside the table.

## Consequences

- The TUI can no longer break the rules of a conversation by writing a field; a change to them is a change to `internal/tui/chat`, and that package cannot reach the terminal.
- A type of the TUI's that has both state and screen is split in two, and the TUI reads the state through methods, which is more code than reading fields.
- The records and the store of a conversation belong to the package, so `internal/app`, which turns what `internal/state` keeps into them, depends on it as well as on `internal/tui`.
- Whether another part of the TUI becomes a package is decided part by part. A part whose state is shared with others (the selection, the progress of a batch) is not one until it is apart from them.
