# gh-desk

A desk in your terminal for the pull requests waiting on you.

```sh
gh desk
```

It opens the pull requests asked of you, yours, the ones you are involved in, and your notifications, each in a tab, with:

- the requests split by whom they are asked of: you, a team, or bots
- a pane with the description drawn as Markdown, the checks, the comments, and the diff
- rules that select what to approve and merge together, such as the dependency updates whose checks pass
- a popup that says what each approve and merge will do before anything is sent, and follows it through: auto-merge, updating a branch, waiting for checks
- Claude, read-only in a worktree of the pull request, to ask about it, draft a review, or read the release notes of the updates before they are merged
- the lists kept from the last launch, shown at once and then read again

![gh-desk with a made-up organization: a question to Claude about a pull request, and the dependency updates approved and merged together](docs/demo.gif)

Try it in your browser, against a made-up GitHub, at [babarot.me/gh-desk](https://babarot.me/gh-desk/). The [docs](https://babarot.me/gh-desk/docs/) explain the rest: what gh-desk aims for, how to use it, and every setting and key.

## Install

As a [gh](https://cli.github.com/) extension, run as `gh desk`:

```sh
gh extension install babarot/gh-desk
```

Or with Go, run as `gh-desk`:

```sh
go install github.com/babarot/gh-desk@latest
```

Or with [Nix](https://nixos.org/), run as `gh-desk`:

```sh
nix profile install github:babarot/nur-packages#gh-desk
```

gh-desk reads GitHub with the account gh is logged in to (`gh auth login`). Asking Claude needs [Claude Code](https://docs.anthropic.com/en/docs/claude-code) and git; everything else works without them.

## Keys

`?` lists every key, for the screen it is opened from. The line at the bottom shows the keys that matter there. A few to start with:

| Key | |
|---|---|
| `j` `k`, `1`–`9`, `tab` | move, go to a tab |
| `enter` | the actions on the pull request, or on those selected |
| `space`, `S` | select one, select by a rule |
| `A`, `M` | approve, merge (with a confirm first) |
| `v`, `J` `K` | overview or diff, scroll the pane |
| `i` | ask Claude about the pull request |
| `/` | filter |

Keys are bound by the name of their action, and can be changed in the config file.

## Config

The first launch writes `~/.config/gh-desk/config.toml` (or under `$XDG_CONFIG_HOME`), with the rules to start with. Write only what you change: the tabs and their searches, the columns, the rules for selecting, how to merge, the theme, and the keys. A mistake in it is reported with where it is.

```sh
gh desk config reference    # every setting, with what it does and its default
gh desk config check        # read it and print its mistakes, without starting
gh desk config show --diff  # the settings that differ from the defaults
gh desk config edit         # open it in $EDITOR, then check it
```

`gh desk config schema` prints its JSON Schema, which the file names for editors that read it.

## Claude

Claude Code runs on a worktree of the pull request at the head you saw, made from gh-desk's own clone in `~/.cache/gh-desk`; your clones are not touched. It runs in restricted mode with only the tools that read (`Read`, `Grep`, `Glob`, and `WebFetch` if you allow it), with no hooks, settings, MCP servers or slash commands of the pull request's repository. It reaches GitHub only through gh-desk's own tools, which read the pull request's repository and nothing else: its checks and CI logs, after the merge too, and its issues and pull requests. A review it drafts goes into the review popup, for you to read and send.

MCP servers of your own can be given to the conversation in `[claude.mcp]`. With [claude-recall](https://github.com/babarot/claude-recall), Claude searches your past Claude Code sessions while it reviews, held to the pull request's repository:

```toml
[claude.mcp]
builtin = true  # to be written when you give servers of your own

[claude.mcp.servers.recall]
command = "recall"
args    = ["mcp", "--repo", "${GH_DESK_URL}"]  # the pull request's repository
tools   = ["recall_search", "recall_list", "recall_export"]
```

The tools under `tools` run without a prompt, on whatever the pull request's text asks. Do not allow `WebFetch` together with a server that reads your own data.

`gh desk gc` removes the worktrees of pull requests that are merged or closed.

## Files

| | |
|---|---|
| `~/.config/gh-desk/config.toml` | the config file |
| `~/.cache/gh-desk` | the clones and worktrees for Claude, and the lists last read |
| `~/.local/state/gh-desk` | the merges waited for, the conversations with Claude, the log (`gh desk log` prints where) |

Each follows its `XDG_*_HOME` when set.

## Development

```sh
mise run test    # the tests, with the race detector
mise run lint    # golangci-lint
mise run demo    # record docs/demo.gif with VHS in Docker
```

[docs/architecture.md](docs/architecture.md) shows how the packages are layered, and [docs/adr](docs/adr) the decisions behind them.

## License

MIT
