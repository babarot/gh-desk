# Configuration

Everything you can change lives in one file, `~/.config/gh-desk/config.toml` (or under `$XDG_CONFIG_HOME`). This page shows how to work with it and the changes most people make first; every setting is listed in the [configuration reference](../reference/configuration.md).

## The file

The first run writes a short file with no settings in it. A setting left out takes its default, so write only what you change, and a default improved in a later version still reaches you.

`gh desk config reference` prints every setting with what it does and its default, from the version you run. Give it a table or a key to see only that:

```console
$ gh desk config reference list.refresh
[list]
# Refresh the shown tab this often, as a Go duration such as "3m" or "90s";
# "off" refreshes only when r is pressed.
# refresh = "3m"
```

```sh
gh desk config reference    # every setting; pipe it to less
gh desk config edit         # open it in your editor, then check it when you close it
gh desk config edit --ui    # edit the settings on a page instead
gh desk config check        # read it and print its mistakes, without starting
gh desk config show --diff  # the settings that differ from the defaults
gh desk config show         # every setting in effect
gh desk config init         # write the file, if there is none
gh desk config apply        # replace the file with one from standard input
```

`gh desk config edit` opens `review.editor`, `$VISUAL` or `$EDITOR`. The first line of the file names its JSON Schema, so an editor that reads TOML with schemas (taplo, or VS Code with Even Better TOML) completes the keys and checks the values as you type; `gh desk config schema` prints it.

A mistake is reported at its line, with what was likely meant:

```console
$ gh desk config check
~/.config/gh-desk/config.toml:42:1: "merge.method" is not a setting; did you mean "merge.methods"?
   |
42 | method = ["squash"]
   | ^~~~~~
```

A warning (something that is likely not what you meant, but does not stop gh-desk) is shown the same way, and gh-desk starts anyway.

## On a page

`gh desk config edit --ui` opens a page in your browser with the settings of the file, and gh-desk drawn above them as they change. Rules, tabs, keys and themes are easier to try there.

Saving on the page checks the file as gh-desk does at start and replaces it, keeping the old file beside the new one as `config.toml.bak`. Settings you did not touch stay as they are, even where they match the default; your comments do not carry over.

A change to anything but how things are drawn is listed in the terminal first, and saved only if you answer `y` there:

```console
the page asks to save:

  claude.command: "claude" -> "my-claude"
  and 1 that change only how things are drawn

save them to ~/.config/gh-desk/config.toml? [y/N]
```

Any other answer leaves the file as it is and the page open. Secrets are not printed: the values of an MCP server's `env` and `headers` show as `(hidden)`. Without a terminal such a change is not saved; the page says so.

Closing the page, with its Close button or by closing the tab, leaves without saving, as does ctrl+c. The page is served on this machine for as long as the command runs. When the file cannot be written, as one from the Nix store, the page says so and shows the new file under New file.

The list on the page is the one gh-desk last read, as of the time it shows, under the account that read it; nothing goes to GitHub. Pick a tab above it to try a rule on that tab. A tab whose search gh-desk has not read yet says so, and before gh-desk has read anything the page shows made-up pull requests and says that too.

A rule is judged by gh-desk's own code, so its count is what `S` would select in that tab. `?` counts the pull requests it cannot tell yet: the description is not kept with the lists, so `packages` is always `?`, as are conditions on what was not read. The count of an `[[auto_approve]]` rule is of that tab; the rule itself searches on its own.

MCP servers are shown and removed there; to add or change one, edit the file.

## After an update

When an update brings settings this install has not had, gh-desk says so in the notifications, one line for each, with the command that explains it. `gh desk config reference` then lists them at its top and marks them, until an update brings others. The file is never changed for you.

A file written by an earlier version, with every setting commented out, goes on working. To make it short, keep only what differs from the defaults; your own comments do not carry over:

```sh
gh desk config show --diff > new.toml
```

## A tab for your repository

Tabs you write replace the default four, so copy the ones you keep. One of them must be the inbox.

```toml
[[tabs]]
name   = "review"
query  = "is:open is:pr review-requested:@me archived:false"
groups = ["you", "team", "other", "bots"]
folded = ["team", "bots"]

[[tabs]]
name  = "infra"
query = "is:open is:pr repo:acme/infra archived:false"
groups = ["you", "other", "bots"]

[[tabs]]
kind = "inbox"
name = "inbox"
```

The query is a GitHub search, as on github.com. See [Tabs and groups](tabs.md) for how groups are decided.

## Rules for what is safe to merge

The rules `S` offers are `[[select]]` entries. A pull request matches a rule when it meets every key the rule gives:

```toml
[[select]]
name      = "safe bumps"
authors   = ["renovate"]
packages  = ["golang.org/x/net", "actions/checkout"]
checks    = ["passing"]
max_lines = 50
```

The first run writes a few rules to start with, live; delete or comment out the ones you do not want. See [Selecting by rules](selecting.md).

## How to merge

```toml
[merge]
methods       = ["squash", "merge"]  # the first the repository allows is used
auto_merge    = true                 # GitHub's auto-merge where the repository allows it
update_branch = "humans"             # update a person's branch when it is behind, not a bot's
```

See [Approving and merging](approve-and-merge.md) for what gh-desk does with these.

## Keys and colors

Every key is bound to an action by name. Give an action a key or a list of keys, or `[]` to turn it off:

```toml
[keys]
merge   = "m"
approve = ["A", "ctrl+a"]
browse  = []

[theme]
dark  = "tokyonight"
light = "github-light"
```

gh-desk picks `theme.dark` or `theme.light` by the terminal's background. A key given to one action is taken from the action that had it by default; two actions you set to the same key are a mistake. The actions and their keys are in the [keys reference](../reference/keys.md).

## Claude

```toml
[claude]
model        = "opus"
language     = "Japanese"
instructions = "Point out changes that have no tests."
```

`language` is the language Claude answers in about a pull request; a review it drafts is written in the language of the pull request. See [Claude](ask-claude.md) for the tools it may use, and for MCP servers and skills.

## When something is wrong

gh-desk writes a log file; `gh desk log` prints where it is, and `L` shows it while gh-desk runs. Set `log.level = "debug"`, or run `gh desk --debug`, to see every read as well as every write.
