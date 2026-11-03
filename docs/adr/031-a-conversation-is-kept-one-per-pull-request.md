# ADR-031: A conversation with Claude is kept, one per pull request, shared by every gh-desk

## Status

Accepted
Partly replaced by ADR-036

## Context

A conversation with Claude about a pull request (ADR-010) lived only in the memory of the gh-desk that started it. Quitting gh-desk, by mistake or to come back later, lost it, and so did opening the same pull request in another gh-desk: the next question started a new session with a full review, the slowest and costliest question there is. Claude Code itself keeps the session, and the worktree it resumes from stays while the pull request is open (ADR-010), but gh-desk forgot the session's ID, so it could not go back to it, nor hand it to Claude Code in the terminal.

Several gh-desk often run at once, in different terminals. Two of them resuming one session at the same time would fork Claude Code's conversation, and two of them moving one pull request's worktree to different heads would change the files under a question being answered. A pull request's head also moves after a conversation about it.

Claude Code's session file is its own format, which mixes the conversation with the requests it sent; it is not something gh-desk can read to show the conversation.

## Decision

- Each conversation is kept in a file of gh-desk's own, one per pull request, under the state directory (`internal/state`), separated by GitHub host, readable only by the user. It holds what the screen shows (the questions as the user saw them, the answers, the tools used) and what resuming needs: the session, whether Claude Code has it, the worktree it started in, and the head of the last question. It is state, not cache: losing it costs a new review.
- One conversation per pull request, shared by every gh-desk. A lock of its own, `flock(2)` as in ADR-009, is held for as long as Claude Code works on the session: from a question to its answer, and while the session is open in Claude Code in the terminal. Another gh-desk that wants to ask meanwhile is refused and shows that the conversation is being answered elsewhere; it reads the conversation again as it opens it, before it asks, and while it shows it and the lock is held elsewhere. That reading only reads; nothing is written on a timer (unlike the lock taken over by ADR-021). How often it reads is a constant, not a setting: it is a small file.
- The conversation is written only under its lock, with a revision that rises at every successful write and never goes down while the file is there. A gh-desk replaces what it holds when the file's revision is newer or its session is another, and drops what it holds when the file it had written is gone. Starting over writes a new conversation over the old one instead of removing it, so another gh-desk cannot take its older copy for the newer.
- A lock file is never removed, by gh-desk or by `gh desk gc`: a lock belongs to the file, and a file made again would let two processes hold it.
- A session ID is made when a question starts the session, never kept for a session Claude Code has not answered in: Claude Code may have taken it before the question was cut short.
- When the head has moved since the last question, the conversation goes on in the same session. The question carries a note that the head moved and the pull request's whole diff at the new head (not the change between heads: a head pushed over may be gone, and Claude cannot run git). The worktree is put at the head shown before every question, under the lock.
- `chat_restart` drops the conversation from gh-desk and starts a new one with a review; Claude Code keeps the old session.
- `gh desk gc` removes the conversation of a pull request no longer open, with its worktree, after taking its lock without waiting; one that a gh-desk is working on is left for the next run.
- A conversation that cannot be written is said once and goes on in memory, detached from the file for the rest of the run, so it cannot write over what another gh-desk wrote meanwhile.
- Quitting interrupts Claude Code rather than killing it, and waits for it a moment, so it closes the session it keeps. How long is a constant, not a setting: it only keeps quitting from hanging.

## Consequences

- A conversation survives quitting and is the same in every gh-desk, at the cost of a file per pull request asked about, holding Claude's answers about code the user can read. It is never written to the log.
- While one gh-desk has Claude answering, or has the session open in Claude Code, no other can ask about that pull request.
- A conversation started under another `cache.dir` cannot be resumed (Claude Code resumes a session only where it started); its next question starts a new session with a review, under the conversation shown.
- What was said in Claude Code in the terminal is in the session, not in the file, and is not shown in gh-desk.
- The triage of ADR-018 is not kept; its session still lasts as long as gh-desk runs.
- Lock files of pull requests asked about stay in the state directory; they are small.
