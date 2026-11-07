# Commands

<!-- Generated from internal/app by TestCommandsReferenceIsUpToDate. Do not edit; run go test ./internal/app -run TestCommandsReferenceIsUpToDate -update. -->

Installed as a gh extension, gh-desk runs as `gh desk`; installed with Go or Nix, as `gh-desk`. With no command, it opens the pull requests waiting on you.

## gh desk

```
review and merge the pull requests that reach you, from the terminal

Usage:
  gh desk [flags]
  gh desk [command]

Available Commands:
  completion  Generate the autocompletion script for the specified shell
  config      check, show or edit the config file, and print what each setting does
  gc          remove the worktrees of pull requests no longer open
  help        Help about any command
  log         print where the log file is

Flags:
      --config string   the config file (default "~/.config/gh-desk/config.toml")
      --debug           log at debug level
  -h, --help            help for gh desk
      --no-cache        neither show nor keep the lists last read
      --version         print the version

Use "gh desk [command] --help" for more information about a command.
```

## gh desk config

```
Check, show or edit the config file, and print what each setting does.

The config file is the one --config names: by default
$XDG_CONFIG_HOME/gh-desk/config.toml, or ~/.config/gh-desk/config.toml.

Usage:
  gh desk config [flags]
  gh desk config [command]

Available Commands:
  apply       replace the config file with the one read from standard input, if it has no mistakes
  check       read the config file and print its mistakes and warnings, without starting; exits 1 when it has mistakes
  edit        open the config file in review.editor, $VISUAL or $EDITOR, then check it; with --ui, on a page in the browser
  init        write the config file, which holds no settings yet, unless one is there
  reference   print every setting with what it does and its default, or those of a table or a key
  schema      print the JSON Schema of the config file
  show        print the settings in effect, or with --diff only those that differ from the defaults

Flags:
  -h, --help   help for config

Global Flags:
      --config string   the config file (default "~/.config/gh-desk/config.toml")
      --debug           log at debug level

Use "gh desk config [command] --help" for more information about a command.
```

## gh desk config apply

```
Replace the config file with the one read from standard input.

The file is checked as gh-desk checks it at start, and nothing is written
when it has mistakes. The file it replaces is kept beside it, with .bak
added to its name.

Usage:
  gh desk config apply [flags]

Flags:
  -h, --help   help for apply

Global Flags:
      --config string   the config file (default "~/.config/gh-desk/config.toml")
      --debug           log at debug level
```

## gh desk config check

```
read the config file and print its mistakes and warnings, without starting; exits 1 when it has mistakes

Usage:
  gh desk config check [path] [flags]

Flags:
  -h, --help   help for check

Global Flags:
      --config string   the config file (default "~/.config/gh-desk/config.toml")
      --debug           log at debug level
```

## gh desk config edit

```
Open the config file in review.editor, $VISUAL or $EDITOR, then check it.

With --ui, open a page in the browser instead, with the settings of the
file and gh-desk drawn as they change. It is served on this machine for as
long as this runs. The list is the one gh-desk last read, as of the time it
shows, under the account that read it, and nothing goes to GitHub; before
anything was read, the page shows made-up pull requests and says so.

Saving on the page checks the file as gh-desk checks it at start and
replaces it, keeping the old one beside it, with .bak added to its name.
Settings not touched stay as they are, even where they match the default;
comments do not carry over. A file that cannot be written, as one from the
Nix store, is left as it is, and the page shows the new one under New file.

A change to anything but how things are drawn is listed here first, and
saved only if it is answered with y; any other answer leaves the file as it
is and the page open. The values of an MCP server's env and headers show as
(hidden). Without a terminal such a change is not saved, and the page says so.

Closing the page, or ctrl+c, leaves without saving, and writes nothing.

Usage:
  gh desk config edit [flags]

Flags:
  -h, --help   help for edit
      --ui     edit the settings on a page in the browser

Global Flags:
      --config string   the config file (default "~/.config/gh-desk/config.toml")
      --debug           log at debug level
```

## gh desk config init

```
write the config file, which holds no settings yet, unless one is there

Usage:
  gh desk config init [flags]

Flags:
  -h, --help   help for init

Global Flags:
      --config string   the config file (default "~/.config/gh-desk/config.toml")
      --debug           log at debug level
```

## gh desk config reference

```
Print every setting with what it does and its default, as comments, from
this version of gh-desk. The config file is not read.

A name narrows it to a table (merge), an array of tables (select), keys, or
one key of any of them (list.refresh, select.max_lines, keys.approve).

Usage:
  gh desk config reference [table or key] [flags]

Flags:
  -h, --help   help for reference

Global Flags:
      --config string   the config file (default "~/.config/gh-desk/config.toml")
      --debug           log at debug level
```

## gh desk config schema

```
print the JSON Schema of the config file

Usage:
  gh desk config schema [flags]

Flags:
  -h, --help   help for schema

Global Flags:
      --config string   the config file (default "~/.config/gh-desk/config.toml")
      --debug           log at debug level
```

## gh desk config show

```
print the settings in effect, or with --diff only those that differ from the defaults

Usage:
  gh desk config show [flags]

Flags:
      --diff   only the settings that differ from the defaults
  -h, --help   help for show

Global Flags:
      --config string   the config file (default "~/.config/gh-desk/config.toml")
      --debug           log at debug level
```

## gh desk gc

```
remove the worktrees of pull requests no longer open

Usage:
  gh desk gc [flags]

Flags:
  -h, --help   help for gc

Global Flags:
      --config string   the config file (default "~/.config/gh-desk/config.toml")
      --debug           log at debug level
```

## gh desk log

```
print where the log file is

Usage:
  gh desk log [flags]

Flags:
  -h, --help   help for log

Global Flags:
      --config string   the config file (default "~/.config/gh-desk/config.toml")
      --debug           log at debug level
```
