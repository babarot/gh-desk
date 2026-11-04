# Configuration

<!-- Generated from internal/config by TestReferenceIsUpToDate. Do not edit; run go test ./internal/config -run TestReferenceIsUpToDate -update. -->

The first launch writes `~/.config/gh-desk/config.toml` (or under `$XDG_CONFIG_HOME`) with every setting below at its default, commented out. Uncomment a line to change it. A mistake in the file is reported with where it is, and `gh desk config check` reports them without starting.

```sh
gh desk config check        # read the file and print its mistakes
gh desk config show --diff  # the settings that differ from the defaults
gh desk config edit         # open it in $EDITOR, then check it
gh desk config schema       # the JSON Schema of the file
```

The keys are under `[keys]`; see [Keys](keys.md).

## [[tabs]]

The tabs, in the order shown; the keys 1 to 9 go to them. Tabs given replace the default four, and one of them must be the inbox. At most 9 tabs.

```toml
[[tabs]]
name = "review"
query = "is:open is:pr review-requested:@me archived:false"
groups = ["you", "team", "other", "bots"]
folded = ["team", "bots"]

[[tabs]]
name = "mine"
query = "is:open is:pr author:@me archived:false"
sort = "drafts_last"

[[tabs]]
name = "involved"
query = "is:open is:pr involves:@me -author:@me -review-requested:@me archived:false"

[[tabs]]
kind = "inbox"
name = "inbox"
show_settled = false
```

| Key | Type | |
|---|---|---|
| `kind` | one of `search`, `inbox` | "search" (default), a GitHub search, or "inbox", the pull requests in your notifications. |
| `name` | string | The tab's name; "inbox" for the inbox by default. |
| `query` | string | A search's GitHub search query. |
| `groups` | list of `you`, `team`, `other`, `bots` | The groups a search is split into, in the order shown. |
| `folded` | list of `you`, `team`, `other`, `bots` | The groups folded at start, among groups. |
| `sort` | one of `updated`, `drafts_last` | "updated" (default) or "drafts_last". |
| `limit` | integer, 1 or more | How many pull requests, or notifications, to read (at most 1000; 100 by default). |
| `show_settled` | boolean | The inbox shows the notifications whose pull request is merged or closed, rather than leave them out with the excluded until a shows them. |

## [[select]]

The rules S offers to select pull requests, besides the built-in "approved by me". A pull request matches a rule when it meets every key the rule gives. The file written on first run has these rules live, not commented out, as rules to start with:

```toml
[[select]]
name = "small"
max_lines = 300

[[select]]
name = "dependency bots"
authors = ["renovate", "dependabot"]
checks = "passing"

[[select]]
name = "approved"
approved_by_me = true

[[select]]
name = "recent"
updated_within_days = 90

[[select]]
name = "lock files"
authors = ["renovate"]
title = "(?i)lock file maintenance"
```

| Key | Type | |
|---|---|---|
| `name` | string | The rule's name, as S lists it. |
| `repos` | list of strings | owner/repo globs the repository must match one of; * matches any part of the owner or the name. |
| `authors` | list of strings | The logins the author must be one of; bots without app/. |
| `packages` | list of strings | The dependencies a renovate or dependabot pull request updates, read from its body; every one it updates must be listed. |
| `title` | string | A regular expression the title must match. |
| `max_files` | integer, 1 or more | The most files it may change. |
| `max_lines` | integer, 1 or more | The most lines it may add and delete together: the sum of the two numbers SIZE shows. |
| `min_files` | integer, 1 or more | The least files it may change. |
| `min_lines` | integer, 1 or more | The least lines it may add and delete together. |
| `updated_within_days` | integer, 1 to 36500 | The most days, of 24 hours, since it was last updated: a label or a bot's comment updates it too. An old pull request updated lately matches. |
| `created_within_days` | integer, 1 to 36500 | The most days, of 24 hours, since it was created. |
| `updated_before_days` | integer, 1 to 36500 | The least days, of 24 hours, since it was last updated, as AGE counts them in a search tab. |
| `created_before_days` | integer, 1 to 36500 | The least days, of 24 hours, since it was created. |
| `bot` | boolean | The author must be a bot. |
| `mine` | boolean | It must be your own pull request. |
| `draft` | boolean | It must be a draft; S never selects one, so [[select]] does not take it. |
| `state` | list of `open`, `merged`, `closed` | The states it must be in one of: "open", "merged" or "closed"; S selects open ones only, so [[select]] takes "open" alone. |
| `checks` | one of `passing`, `failing`, `pending` | "passing": passed, or only checks.approval pending; "failing": failed; "pending": still running. |
| `decision` | one of `approved`, `changes_requested`, `review_required` | The review decision REVIEW shows. |
| `labels` | list of strings | Labels one of which it must have. |
| `approved_by_me` | boolean | Your latest review must approve it; a person's pull request matches too, unlike the built-in "approved by me". |
| `reviewed_by_me` | boolean | You must have reviewed it, whatever the review said. |
| `requested` | list of strings | Whom a review must be asked of, one of them: "me", or a team as "@slug". |
| `reasons` | list of `approval_requested`, `assign`, `author`, `ci_activity`, `comment`, `invitation`, `manual`, `member_feature_requested`, `mention`, `review_requested`, `security_advisory_credit`, `security_alert`, `state_change`, `subscribed`, `team_mention` | Reasons of a notification, one of which the row's must have; S has no notification, so [[select]] does not take it. |

## [list]

### list.refresh

duration or "off" · Default: `"3m"`

Refresh the shown tab this often, as a Go duration such as "3m" or "90s"; "off" refreshes only when r is pressed.

### list.stale_days

days or "off" · Default: `30`

Mark AGE when a pull request has not been updated for this many days, or "off".

### list.home_owners

list of strings · Default: `[]`

Owners whose name is left out of the PR column, such as your company.

### list.columns

list of `pr`, `title`, `author`, `ci`, `review`, `why`, `size`, `age` · Default: `["pr", "title", "author", "ci", "review", "why", "size", "age"]`

The columns shown, in the order given; "pr" and "title" are needed. WHY shows on grouped tabs and the inbox only, and AUTHOR not on a tab of your own pull requests. On a narrow terminal SIZE goes first, then AUTHOR.

## [menu]

### menu.actions

list of `ask_claude`, `browse`, `review`, `comment`, `approve`, `merge`, `ready_for_review`, `hide` · Default: `["ask_claude", "browse", "review", "comment", "approve", "merge", "ready_for_review", "hide"]`

The actions the menu of enter offers, in the order given. One left out is not in the menu; its key still runs it from the list.

## [preview]

### preview.height

integer, 0 or more · Default: `14`

Lines of the preview pane under the list; 0 hides it until it is shown with a key. Dragging the pane's top edge, or + and -, changes them, and the lines set that way are kept over this one.

### preview.tab

one of `overview`, `diff` · Default: `"overview"`

The tab the pane opens on: "overview" (default), the description beside a column of facts, or "diff".

## [description]

### description.style

one of `rendered`, `plain` · Default: `"rendered"`

How a description and its comments are drawn in the pane: "rendered" (default) draws headings, lists, code, quotes and tables; "plain" shows the Markdown as written.

## [diff]

### diff.collapse

list of glob patterns · Default: `["*.lock", "go.sum", "pnpm-lock.yaml", "package-lock.json", "yarn.lock"]`

Files shown folded, as their name and line counts, until unfolded: glob patterns, matched against the base name unless they have a slash.

## [checks]

### checks.approval

list of glob patterns · Default: `[]`

Checks that pass only after an approval, such as a policy bot's: glob patterns of check names. While only they are pending, CI shows ◌, and the checks count as passing when deciding a merge.

## [review]

### review.mark_done

boolean · Default: `true`

Mark a pull request's notification done after you review it here, from any tab.

### review.editor

string · Example: `"code --wait"`

The editor ctrl+e opens to write a review, with its arguments, such as "code --wait". Left out, $VISUAL, then $EDITOR.

## [merge]

### merge.methods

list of `merge`, `squash`, `rebase` · Default: `["merge", "squash", "rebase"]`

The order of preference among the methods a repository allows: "merge", "squash", "rebase". The first one the repository allows is used.

### merge.auto_merge

boolean · Default: `true`

Use GitHub's auto-merge where the repository allows it; elsewhere gh-desk waits for the checks and merges when they pass. Commits pushed after auto-merge is turned on are merged too, as GitHub does.

### merge.checks_timeout

duration · Default: `"6h"`

Where gh-desk waits, how long to wait for required checks still running. Only the time gh-desk is running counts.

### merge.blocked_timeout

duration · Default: `"30m"`

Where gh-desk waits, how long a pull request may stay blocked by another rule (a second approval, code owners) after its required checks finished. Only the time gh-desk is running counts.

### merge.update_branch

one of `humans`, `all`, `never` · Default: `"humans"`

Whose branch is updated when it is behind its base: "humans" (default), "all" or "never". Bots rebase their own branches, and some (renovate) stop updating a branch someone else committed to.

## [bulk]

### bulk.attention_paths

list of glob patterns · Default: `[".github/workflows/*", "CODEOWNERS", ".github/CODEOWNERS", "docs/CODEOWNERS"]`

Pull requests changing files that match these glob patterns get a mark in the confirm popup.

## [claude]

C and i ask Claude Code (claude -p, with the account you are signed in with). i, or ask claude about it in the menu of enter, reviews the pull request under the cursor in a worktree of its head. C plans what to show and select in the tab shown, with no tools, and answers in the language you ask in; enter applies the plan, and an approve or a merge in it opens the confirm popup. Claude's own tools only read: no settings files are loaded, its file tools see only the worktree and the skills of plugins, and it has none that run commands. The conversation also takes the MCP servers of mcp_servers and the skills of plugins, which can do whatever they are made to.

### claude.command

string · Default: `"claude"`

The claude command: a name looked up on PATH, or a path to it (~ is your home), without arguments.

### claude.model

string · Default: `"opus"`

The model Claude Code runs, as claude --model takes it.

### claude.allowed_tools

list of strings · Default: `["Read", "Grep", "Glob"]`

The tools Claude may use, among "Read", "Grep", "Glob" and "WebFetch".

### claude.language

string · Default: `"English"`

The language Claude answers in about a pull request, such as "Japanese". A review drafted with D is written in the language of the pull request instead.

### claude.instructions

string · Default: `""`

Appended to the instructions of every review and triage, such as "Point out changes that have no tests."

### claude.release_notes_check

boolean · Default: `true`

Let Claude read the release notes of the bot pull requests in the merge popup, and mark those that need care.

### claude.mcp_servers

table of MCP servers · Example: `{ recall = { command = "recall", args = ["mcp", "--repo", "${GH_DESK_URL}"], tools = ["recall_search", "recall_list", "recall_export"] } }`

MCP servers for the conversation about a pull request, by name, written as Claude Code configures a server (command, args, env; or type, url, headers), with tools: the server's tools that may run without a prompt. A server started with a command runs in the cache, not in the worktree. ${GH_DESK_OWNER}, ${GH_DESK_REPO}, ${GH_DESK_NUMBER} and ${GH_DESK_URL} (the repository's URL) are the pull request; pass secrets as ${VAR} from your environment. What the pull request says may call these tools: a server that reads private data, together with WebFetch, lets it send the data out.

### claude.plugins

list of strings · Example: `["~/.claude/plugins/cache/<marketplace>/<plugin>/<version>"]`

Plugins whose skills the conversation about a pull request takes, as their directories (~ is your home). Only the skills are taken, copied when gh-desk starts, as gh-desk:&lt;name&gt;; the rest of a plugin (hooks, MCP servers, language servers, commands) is not. A skill whose allowed-tools grants a tool outside allowed_tools, Skill and the tools of mcp_servers is a mistake.

## [theme]

### theme.dark

one of `default`, `latte`, `github-dark`, `github-light`, `tokyonight`, `tokyonight-day`, `gruvbox-dark`, `gruvbox-light`, `nord` · Default: `"default"`

The color scheme on a dark terminal, and on one that does not tell its background.

### theme.light

one of `default`, `latte`, `github-dark`, `github-light`, `tokyonight`, `tokyonight-day`, `gruvbox-dark`, `gruvbox-light`, `nord` · Default: `"latte"`

The color scheme on a light terminal. Write the same theme as theme.dark to use it on any terminal.

### theme.spinner

one of `dots`, `dense`, `bounce`, `snake`, `jump`, `fill`, `sand`, `line`, `arc`, `quarter`, `block`, `star`, `off` · Default: `"dots"`

The spinner shown while something is read: "dots" (default) ⠋⠙⠹, "dense" ⣾⣽⣻, "bounce" ⠁⠂⠄, "snake" ⠉⠙⠚, "jump" ⢄⢂⢁, "fill" ⣀⣤⣶, "sand" ⡀⣀⣤, "line" |/-\, "arc" ◜◠◝, "quarter" ◴◷◶, "block" ▖▘▝, "star" ✶✸✹, or "off", a ⋯ that does not move and is left out before a line that says what is going on.

### theme.shimmer

boolean · Default: `true`

Run a light over the text that says what is going on, such as refreshing… or thinking…; the screen is drawn more often while something is read.

## [log]

### log.level

string · Default: `"info"`

How much goes to the log file: "debug", "info" (default), "warn" or "error". --debug and GH_DESK_LOG_LEVEL take precedence. gh desk log prints where the file is.

## [cache]

### cache.dir

string · Example: `"~/.cache/gh-desk"`

Where gh-desk keeps its clones and the worktrees Claude reads, and the lists last read (lists.json, shown at once at the next launch) (~ is your home); by default $XDG_CACHE_HOME/gh-desk, or ~/.cache/gh-desk.
