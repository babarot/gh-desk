# Configuration

Everything you can change lives in one file, `~/.config/gh-desk/config.toml` (or under `$XDG_CONFIG_HOME`). The easiest way to change it is on a page:

```sh
gh desk config edit --ui
```

## On a page

The command opens a page in your browser, with gh-desk drawn above the settings and following each change before you save it. The list is your own, as gh-desk last read it; nothing goes to GitHub.

[The settings page, with made-up pull requests](https://babarot.me/gh-desk/settings/ "settings")

That is the page itself, with made-up pull requests. Try a theme, or change the columns or a key, and See the file shows what would be written. Nothing is saved here. Rules are only on the page the command opens, where gh-desk judges them on your pull requests.

To keep your changes, choose Review changes, then Save and finish. A change to anything but how things are drawn is confirmed in your terminal, so answer `y` there. The file is then replaced and the old one kept as `config.toml.bak`; your comments do not carry over. Closing the page leaves the file as it is.

[`gh desk config edit`](../reference/commands.md#gh-desk-config-edit) has what happens in every case.

## What to change first

Each of these is a page on the left of the settings.

| To | Open | Read more |
|---|---|---|
| Add a tab for your repository | Tabs | [Tabs and groups](tabs.md) |
| Choose what is safe to merge together | Selection rules | [Selecting by rules](selecting.md) |
| Decide how to merge | Merging | [Approving and merging](approve-and-merge.md) |
| Change a key | Keys | [Keys](../reference/keys.md) |
| Change the colors | Appearance | |
| Tell Claude how to answer | Claude | [Claude](ask-claude.md) |

## In the file

Everything on the page can be written by hand, which suits a file kept in your dotfiles. A setting left out takes its default, so write only what you change:

```toml
[merge]
methods = ["squash", "merge"]

[keys]
merge = "m"

[theme]
dark = "tokyonight"
```

Every setting is in the [configuration reference](../reference/configuration.md), and in `gh desk config reference`, which is of the version you run.

```sh
gh desk config edit         # open it in your editor, then check it when you close it
gh desk config check        # read it and print its mistakes, without starting
gh desk config show --diff  # the settings that differ from the defaults
gh desk config reference    # every setting; give a table or a key to see only that
```

The first line of the file names its JSON Schema, so an editor that reads TOML with schemas (taplo, or VS Code with Even Better TOML) completes the keys and checks the values as you type. The [commands reference](../reference/commands.md#gh-desk-config) has the rest of `gh desk config`.

## After an update

When an update brings settings this install has not had, gh-desk says so in the notifications, and `gh desk config reference` lists them at its top. The file is never changed for you.

A file written by an earlier version, with every setting commented out, goes on working. `gh desk config show --diff > new.toml` makes a short one, without your comments.

## When something is wrong

gh-desk writes a log file; `gh desk log` prints where it is, and `L` shows it while gh-desk runs. Set `log.level = "debug"`, or run `gh desk --debug`, to see every read as well as every write.
