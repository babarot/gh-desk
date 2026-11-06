# Claude

With [Claude Code](https://docs.anthropic.com/en/docs/claude-code) installed and signed in, gh-desk asks it two kinds of question. Without it, everything else works.

## About one pull request

`i` opens a conversation with Claude about the pull request under the cursor. It takes the place of the pane, and is kept per pull request: move to another and back, and it is still there. Until something is asked, the pane shows the pull request and a few questions to start with, a review first; `tab` puts them in the text box one after another, and `enter` asks. Type to ask your own.

[Claude on a pull request](https://babarot.me/gh-desk/?scenario=claude "playground")

`ctrl+x` drafts a review from the conversation. The draft goes into the review popup for you to read and edit, and is sent through the usual confirm popup. `ctrl+o` hands the conversation to Claude Code in the terminal.

Claude answers in `claude.language`, and follows `claude.instructions` in every review, such as "Point out changes that have no tests."

## About the tab

`C` asks Claude about the tab shown, in your own words, such as "merge the bots with passing checks". `tab` fills in the questions it offers. In `C`, `#` and `@` open a menu of the pull requests and authors of the tab, to name one in a question.

As you type in either, the rest of a question you asked or are offered shows faintly after the cursor, and `tab` or `right` fills it in. While Claude answers, `esc` stops it and `backspace` closes the popup with Claude still at work; `C` shows the answer when it comes. `ctrl+l`, pressed twice, starts the conversation over.

In both, `up` and `ctrl+p` bring back the questions asked before, and `down` and `ctrl+n` go forward again, back to what you were typing. `C` keeps its questions between launches, from every tab; a question brought back is asked about the tab shown. `i` goes through the questions of that pull request. `ctrl+r` opens them as a list in place of the text box: type to narrow it, `enter` puts the question in the text box without sending it, and `esc` goes back. Claude answers with a plan of what to filter, fold, select, and at the end approve or merge; it does not act on the screen itself. `enter` applies the plan on the tab it was asked on, and an approve or a merge in it opens the confirm popup with the pull requests Claude named.

[A plan from Claude, applied and merged](https://babarot.me/gh-desk/?scenario=triage "playground")

Claude is given only the rows of the tab shown, so it cannot name a pull request of another tab. A pull request that has left the tab, or whose head moved since Claude saw it, is left out of the plan, and the plan says why.

## What Claude can do

Claude reads a worktree of the pull request at the head you saw, made from gh-desk's own clone in `cache.dir`; your clones are not touched. It runs in Claude Code's restricted mode:

- no settings files are loaded, so a pull request's hooks and permissions never run
- its file tools see only the worktree, not your other files
- its tools are among `Read`, `Grep`, `Glob` and `WebFetch` (`claude.allowed_tools`); none runs a command or writes a file
- it reaches GitHub only through gh-desk's own tools, below
- the pull request's text is given as data, not as instructions

A plan about the tab is made with no tools at all.

## Reading the repository on GitHub

gh-desk gives the conversation tools of its own that read the pull request's repository, and nothing else:

- `checks`: the checks of the pull request's head and, once it is merged, of its merge commit, read when asked; or those of a branch or commit of the repository
- `check_run_log`: what a check run says, its annotations and, for a GitHub Actions job, the lines of its log around errors and its end
- `search_issues` and `read_issue`: the repository's issues and pull requests, their comments, reviews and review threads

So Claude can answer "the CI failed after I merged it, why?" or "was this discussed in an issue?". The tools only read, and only what anyone who can see the pull request can see; whatever GitHub returns from another repository, and your reviews not sent yet, are left out.

They are on unless `claude.mcp.builtin = false`. When `WebFetch` is allowed or you give servers of your own, write `builtin = true` or `false` yourself: the pull request's text could have Claude send what the tools read out through them.

## MCP servers and skills

MCP servers of your own can be given to the conversation in `[claude.mcp]`, and the skills of your plugins in `claude.plugins`. With [claude-recall](https://github.com/babarot/claude-recall), Claude searches your past Claude Code sessions while it reviews, held to the pull request's repository:

```toml
[claude.mcp]
builtin = true

[claude.mcp.servers.recall]
command = "recall"
args    = ["mcp", "--repo", "${GH_DESK_URL}"]  # the pull request's repository
tools   = ["recall_search", "recall_list", "recall_export"]
```

What the pull request says may call these tools. A server that reads private data, together with `WebFetch`, lets it send the data out, so keep `WebFetch` off with such a server.

The worktrees are kept while the pull request is open, so a conversation can be resumed; `gh desk gc` removes those of pull requests merged or closed.

See [`[claude]`](../reference/configuration.md#claude) for the settings.
