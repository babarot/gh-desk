# Getting started

## Install

As a gh extension, run as `gh desk`:

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

gh-desk reads GitHub with the account gh is logged in to (`gh auth login`); there is no token to set up. Asking Claude needs [Claude Code](https://docs.anthropic.com/en/docs/claude-code), signed in, and git. Everything else works without them.

## Run it

```sh
gh desk
```

The first run writes the config file, `~/.config/gh-desk/config.toml`, with every setting at its default and commented out, then opens the pull requests waiting for you. The lists are kept on disk when gh-desk quits, and the next launch shows them at once while it reads them again.

## The screen

Across the top are the tabs. By default they are four:

| Tab | What it lists |
|---|---|
| review | pull requests whose review is requested from you or a team of yours, split into groups |
| mine | your own pull requests, drafts last |
| involved | pull requests you are involved in, that are neither yours nor asked of you |
| inbox | the pull requests in your notifications, with why each is there |

Under the list is the pane: the pull request under the cursor, its description beside its checks and reviews, or its diff. The line at the bottom shows the keys that matter where you are, and `?` lists them all.

[The screen as gh-desk starts](https://babarot.me/gh-desk/ "playground")

A few keys to start with:

| Key | |
|---|---|
| `j` `k`, `1`–`9`, `tab` | move, go to a tab |
| `enter` | the actions on the pull request, or on those selected |
| `space`, `S` | select one, select by a rule |
| `A`, `M` | approve, merge (with a confirm first) |
| `p`, `v`, `J` `K` | the pane, overview or diff, scroll the pane |
| `i` | ask Claude about the pull request |
| `/` | filter |
| `?` | every key |

Nothing is written to GitHub without a confirm popup that lists what will be done to each pull request.

## Next

- [Tabs and groups](guides/tabs.md), to make the tabs your own
- [Approving and merging](guides/approve-and-merge.md), to know what the popup will do
- [Configuration](guides/configuration.md), to change the rest
- [Concepts](concepts/README.md), for what gh-desk aims for and why it works the way it does
