# ADR-010: Claude reads a worktree of its own clone, in restricted mode

## Status

Accepted
Partly replaced by ADR-037

## Context

Claude Code can be asked about a pull request. A review is better when Claude can read the rest of the repository, not only the diff, so it should work in a checkout of the pull request's head. gh-desk runs `claude -p`, which uses the account the user is signed in with; no API key is involved.

Everything in a pull request was written by someone else: its description, comments, diff, and the files of its head. Any of it may carry instructions meant for Claude. A checkout of the head also contains whatever the pull request adds, and Claude Code reads its project files from the working directory: `.claude/settings.json` (hooks, which run commands, and permissions) and `.mcp.json` (MCP servers, which are commands too). Limiting the tools alone does not stop those.

Where the checkout comes from matters too. A worktree added from the user's own clone leaves its entry in that clone, and moving or deleting the clone breaks the worktree.

## Decision

- Every run of Claude Code takes `--restricted`, `--strict-mcp-config` with an empty `--mcp-config`, `--tools` with the configured tools, and `--disable-slash-commands`. Restricted mode reads no user, project or local settings files, so a pull request's hooks and permissions are never loaded; it confines the file tools to the working directory, so Claude cannot read the user's other files (keys, other repositories) even when told to; and it removes the tools that run commands. `claude.allowed_tools` accepts only `Read`, `Grep`, `Glob` and `WebFetch`; anything else is a config error. `-p` runs add `--allowedTools` with the same tools, so they do not stop to ask. Handing the session to Claude Code in the terminal takes the same flags and no `--allowedTools`, so Claude Code asks before each tool.
- `--setting-sources user` is not used: it also skips the project's settings, but leaves the file tools free to read anywhere. Nor is `--bare`: it skips them too, but signs in only with an API key.
- The prompt goes in on stdin, and marks the pull request's text as data, not instructions. Claude's answers are shown with terminal escapes removed, and a draft review is only placed in the review popup: the user reads and edits it, and sends it through the usual confirm popup.
- Git runs on the cache with no hooks (`core.hooksPath=/dev/null`): a relative `core.hooksPath` in the user's config would otherwise run hooks a pull request adds when its head is checked out, before Claude starts.
- Worktrees come from gh-desk's own clones, in `cache.dir`: a blobless clone per repository, and a detached worktree per pull request at the head the user saw. The head is fetched from `refs/pull/<number>/head` of the base repository, so forks need nothing more. When the user has a clone under a ghq root, the new clone borrows its objects with `--reference-if-able` and copies them with `--dissociate`, so it does not depend on that clone afterwards. Nothing is written to the user's clones. Git asks gh for credentials, so private repositories are read with gh's account.
- Worktrees are kept while the pull request is open, so a conversation can be resumed (`claude --resume` needs the same directory). `gh desk gc` removes those of pull requests GitHub reports merged or closed.
- Sessions are kept by Claude Code (no `--no-session-persistence`), so they can be resumed in Claude Code and searched later. Reading release notes in the merge popup is a one-off question with no tools, run in the cache directory, and keeps no session.

## Consequences

- A pull request cannot run anything on the user's machine through Claude Code, and Claude cannot read outside the worktree. What it can still do is answer badly or carry instructions in its answer; the user reads every draft before it is sent.
- Restricted mode does not load CLAUDE.md, the worktree's or the user's, as instructions. Claude can still read the worktree's as a file, as it reads the description and the diff: a pull request can steer the answer through it, but no tool can act on it.
- The user's own Claude Code settings (their hooks, their permission rules) do not apply in gh-desk either.
- The cache holds a clone of each repository asked about. It grows with use; `gc` removes worktrees, not clones.
- Restricted mode is a recent Claude Code flag; an older Claude Code fails on it, which shows as an error in the conversation rather than as an unrestricted run.
