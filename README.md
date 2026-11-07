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

[babarot.me/gh-desk](https://babarot.me/gh-desk/) shows what it does, and [the playground](https://babarot.me/gh-desk/play/) runs it in your browser, against a made-up GitHub. The [docs](https://babarot.me/gh-desk/docs/) explain the rest: what gh-desk aims for, how to use it, and every setting and key.

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

Keys are bound by the name of their action, and can be changed on the settings page.

## Make it yours

There is no file to write. This opens a page in your browser, with gh-desk drawn above the settings and following each change before you save it:

```sh
gh desk config edit --ui
```

On it you can add a tab for your repository, write the rules for what is safe to merge and see what they catch among your own pull requests, and choose how to merge, what Claude is told, the keys and the theme. [The settings playground](https://babarot.me/gh-desk/play/settings/) shows the page with made-up pull requests.

Everything on the page can also be written by hand in `~/.config/gh-desk/config.toml`, which suits a file kept in your dotfiles. The [configuration guide](https://babarot.me/gh-desk/docs/guides/configuration/) shows how.

## Claude

Claude Code runs on a worktree of the pull request at the head you saw, made from gh-desk's own clone in `~/.cache/gh-desk`; your clones are not touched. It runs in restricted mode with only the tools that read (`Read`, `Grep`, `Glob`, and `WebFetch` if you allow it), with no hooks, settings, MCP servers or slash commands of the pull request's repository. It reaches GitHub only through gh-desk's own tools, which read the pull request's repository and nothing else: its checks and CI logs, after the merge too, and its issues and pull requests. A review it drafts goes into the review popup, for you to read and send.

MCP servers of your own and the skills of your plugins can be given to the conversation, as can [claude-recall](https://github.com/babarot/claude-recall) to search your past Claude Code sessions; [the guide](https://babarot.me/gh-desk/docs/guides/ask-claude/#mcp-servers-and-skills) shows how. Do not allow `WebFetch` together with a server that reads your own data.

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
