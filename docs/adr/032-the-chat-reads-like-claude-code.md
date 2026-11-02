# ADR-032: The chat reads like Claude Code

## Status

Accepted

## Context

The chat with Claude about a pull request marked each turn with a short colored word, "you" or "claude", and listed every tool Claude used, with its full path, above the answer. A long conversation was hard to scan: the turns ran into each other, the tools pushed the answer off the screen, and a question showed only its first line.

The users of gh-triage use Claude Code, and `O` opens the same conversation there. Claude Code already has a way of drawing a conversation that they read every day.

## Decision

- The chat draws a conversation the way Claude Code does: a question set apart on a band of its own, Claude's answer led by its mark. A user who reads one reads the other. Labels with the speaker's name, a rule between turns, and a colored bar beside each turn were tried in a mock and rejected: each is a third way to read a conversation, next to Claude Code's and GitHub's.
- The chat tells what Claude did while it works, and once it has answered, only how much: the full record is in Claude Code, which `O` opens. A key to unfold the tools of an answer in the chat was rejected, as it would keep a second, smaller viewer of what Claude Code already shows.
- The text of the chat is wrapped at a width that is a constant (ADR-014), however wide the terminal, as lines past it are slow to read. It is not a setting: the reason for it does not change with the user.
- The chat says nothing that the key line says (ADR-024).

## Consequences

- A change to how Claude Code draws a conversation is a reason to look at the chat again; the chat does not follow it on its own.
- What the chat shows of a tool is what `claude` reports of it as it runs; a detail it does not report is seen in Claude Code.
