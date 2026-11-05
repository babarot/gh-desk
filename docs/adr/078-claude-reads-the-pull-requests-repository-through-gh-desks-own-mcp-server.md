# ADR-078: Claude reads the pull request's repository through gh-desk's own MCP server

## Status

Accepted

## Context

The conversation about a pull request runs Claude Code restricted (ADR-010): it reads the worktree, takes the MCP servers and skills the user configured (ADR-037), and has no tool that runs a command. What it knows of GitHub is what the first question's prompt carried: the description, the latest comments, and the names and states of the checks, as they were then. It has no CI log, nothing of other issues or pull requests, and nothing after a merge, since a merge does not move the head and so tells it nothing new. Asked "the CI failed after I merged it, can you look?", Claude searched past sessions and then guessed from the code, where Claude Code in a terminal would have run `gh run view --log-failed`.

Giving Claude a shell is not an answer. In `-p` the allowed tools run without a prompt, on whatever the pull request's text asks, and that text is written by others: its author, and in a bot's pull request the authors of the release notes it quotes. Restricted mode removes the tools that run commands whole, so `gh run view` alone cannot be let through, and gh's token can write.

The tools only read, so what they guard against is a leak, not a change. A leak needs something to read and a way out. WebFetch and a server of the user's that reaches a service are ways out the configuration can close. One is not: the review Claude drafts, which the user posts on the pull request. A pull request's text can ask Claude to quote what it read in the draft, and a long review is easy to post without noticing.

Measured against GitHub on public repositories: the ID of another repository's check run or job, asked under this repository's path, is answered 404 for the check run, its annotations, the job and its log; a ref with `../` does not reach another repository; two `repo:` qualifiers in a search are ORed. Not measured, but as GitHub documents linking: a pull request may close, and a timeline may reference, an issue of another repository written as `owner/repo#n`, so `closingIssuesReferences` and the timeline can return issues of other repositories, private ones the viewer can see included.

## Decision

- gh-desk has its own MCP server, `gh desk mcp`, a hidden command that Claude Code starts over stdio for the conversation and for Claude Code in the terminal, like a server of the user's (ADR-037): from the cache directory, with its tools allowed without a prompt in `-p` and asked for in the terminal. Its name, `gh-desk`, is reserved; a server of the user's may not take it. The runs that take no tools take no server.
- What the server may read is bounded by who can see the pull request: anything they can already see may end up in the draft without leaking more. So the server reads the pull request's repository and nothing else, and only what reading the repository shows: its checks and their logs, its issues and pull requests with their comments, reviews and review threads. What only the viewer or an admin sees is not read: reviews and review comments not sent yet are left out, and there is no tool for settings, secrets or advisories. Other repositories are out, since their readers are not the pull request's.
- The bound is enforced by the server, not asked of Claude. The repository and the pull request come from the environment gh-desk runs Claude Code in, never from a tool's arguments. A search takes no syntax from Claude: each argument is checked against the values it may have, or quoted as a phrase, so nothing in it reads as a qualifier. Whatever GitHub returns from another repository, in a search or inside another object, is dropped by comparing its repository, ignoring case; references GitHub resolves into other repositories' content, such as the timeline, are not asked for. [internal/mcpserver](../../internal/mcpserver) holds these rules and their tests.
- The tools write nothing. A write goes through the confirm popup and the runner (ADR-043), and Claude's tools do not open another way.
- What the tools return is marked as written by others, and the instructions say so, as for the pull request's text (ADR-010). The instructions say Claude reaches GitHub only through these tools, or that it cannot reach GitHub when the server is off, and that it asks the user rather than guess when what a question needs is out of reach.
- One question may call the tools a bounded number of times, a constant: the server shares gh's GraphQL points with gh-desk's lists, and a pull request's text should not be able to spend them.
- `claude.mcp.builtin` turns the server on or off; the user's servers are `claude.mcp.servers`. Left out, it is on, unless `claude.allowed_tools` has WebFetch or `claude.mcp.servers` has a server: either can carry what the tools read to someone who is not the pull request's reader, so the file must then say true or false, and leaving it out is a mistake in the file. This is the one setting whose default depends on others rather than being a value (ADR-016): a value would open that way out silently for a file written before the server existed. gh-desk does not judge each server of the user's, as it cannot tell which reach out (ADR-037).
- The server logs to gh-desk's log file and never moves it aside, which only the TUI does; its reads are logged at debug, never what they returned.

This replaces, in ADR-010's Consequences, "Claude cannot read outside the worktree": through these tools it reads the pull request's repository on GitHub. In ADR-037's Decision, the user's servers are configured in `claude.mcp.servers` rather than `claude.mcp_servers`, which stays in the table of old names; the rest of that decision stands, and these tools join those a skill may grant. In ADR-016's Decision, "A key left out takes its default" no longer holds for `claude.mcp.builtin`, whose default depends on other settings as above.

## Alternatives

- The bash tool, or `gh` allowed by prefix: restricted mode cannot keep one command, and the token can write.
- The pull request alone as the bound: safe, but the related pull requests, issues and the base branch's CI after a merge stay out of reach, while the repository's readers are the pull request's anyway.
- A tool set that copies gh (any repository, search, `gh api` reads), or GitHub's own MCP server in `claude.mcp.servers`: the bound becomes everything the token sees, and the draft carries another repository's content to this one's readers. Such a catalog also grows with each request.
- The logs put in the prompt by gh-desk: they go into every question, and gh-desk would have to guess what to read, such as the base branch's checks after a merge.
- A server inside the TUI over HTTP: a local port and a token to guard it, and the network outside `internal/github`.
- The server off without a word when WebFetch or a user's server is there: the user would not know why Claude cannot read CI.
- `claude.repo_tools`, `claude.github_tools`, `claude.builtin_mcp_server`, or GitHub as an entry of `claude.allowed_tools`: the first two name an effect that a user's server could also have, the third repeats the neighbour `mcp_servers` it would sit beside, and `allowed_tools` names Claude Code's own tools, passed to `--tools` as they are.

## Consequences

- A user who allows WebFetch or gives a server of their own must write `claude.mcp.builtin` before gh-desk starts.
- A new tool must stay within the repository and within what its readers see, and must drop what GitHub returns from elsewhere; an argument that reaches GitHub's search syntax needs the same checks.
- The tools' results are as much a pull request's text as its description: anything that relies on Claude ignoring instructions in the description has to hold for them too.
- The bound rests on GitHub answering 404 for another repository's IDs under this repository's path, and on the fields asked for; a change in either needs measuring again.

## Not decided here

- Tools for the history and blame of the changed files, another pull request's diff, or the workflows and their past runs.
- Reading repositories the user names beside the pull request's.
