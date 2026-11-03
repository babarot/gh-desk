# Claude

With [Claude Code](https://docs.anthropic.com/en/docs/claude-code) installed and signed in, gh-desk asks it two kinds of question. Without it, everything else works.

## About one pull request

`i` asks Claude to review the pull request under the cursor. The conversation takes the place of the pane, about the pull request under the cursor, and is kept per pull request: move to another and back, and it is still there. Type to ask more.

[Claude on a pull request](https://babarot.me/gh-desk/?scenario=claude "playground")

`ctrl+r` drafts a review from the conversation. The draft goes into the review popup for you to read and edit, and is sent through the usual confirm popup. `ctrl+o` hands the conversation to Claude Code in the terminal.

Claude answers in `claude.language`, and follows `claude.instructions` in every review, such as "Point out changes that have no tests."

## About the tab

`C` asks Claude about the tab shown, in your own words, such as "merge the bots with passing checks". Claude answers with a plan of what to filter, fold, select, and at the end approve or merge; it does not act on the screen itself. `enter` applies the plan, and an approve or a merge in it opens the confirm popup with the pull requests Claude named.

[A plan from Claude](https://babarot.me/gh-desk/?scenario=triage "playground")

Claude is given only the rows of the tab shown, so it cannot name a pull request of another tab. A pull request that has left the tab, or whose head moved since Claude saw it, is left out of the plan, and the plan says why.

## What Claude can do

Claude reads a worktree of the pull request at the head you saw, made from gh-desk's own clone in `cache.dir`; your clones are not touched. It runs in Claude Code's restricted mode:

- no settings files are loaded, so a pull request's hooks and permissions never run
- its file tools see only the worktree, not your other files
- its tools are among `Read`, `Grep`, `Glob` and `WebFetch` (`claude.allowed_tools`); none runs a command or writes a file
- the pull request's text is given as data, not as instructions

A plan about the tab is made with no tools at all.

## MCP servers and skills

MCP servers of your own can be given to the conversation in `claude.mcp_servers`, and the skills of your plugins in `claude.plugins`. With [claude-recall](https://github.com/babarot/claude-recall), Claude searches your past Claude Code sessions while it reviews, held to the pull request's repository:

```toml
[claude.mcp_servers.recall]
command = "recall"
args    = ["mcp", "--repo", "${GH_DESK_URL}"]  # the pull request's repository
tools   = ["recall_search", "recall_list", "recall_export"]
```

What the pull request says may call these tools. A server that reads private data, together with `WebFetch`, lets it send the data out, so keep `WebFetch` off with such a server.

The worktrees are kept while the pull request is open, so a conversation can be resumed; `gh desk gc` removes those of pull requests merged or closed.

See [`[claude]`](../reference/configuration.md#claude) for the settings.
