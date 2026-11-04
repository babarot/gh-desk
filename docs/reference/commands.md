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
  config      check, show or edit the config file
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
Check, show or edit the config file.

The config file is the one --config names: by default
$XDG_CONFIG_HOME/gh-desk/config.toml, or ~/.config/gh-desk/config.toml.

Usage:
  gh desk config [flags]
  gh desk config [command]

Available Commands:
  check       read the config file and print its mistakes and warnings, without starting; exits 1 when it has mistakes
  edit        open the config file in review.editor, $VISUAL or $EDITOR, then check it
  init        write the config file with every setting at its default, unless one is there
  schema      print the JSON Schema of the config file
  show        print the settings in effect, or with --diff only those that differ from the defaults

Flags:
  -h, --help   help for config

Global Flags:
      --config string   the config file (default "~/.config/gh-desk/config.toml")
      --debug           log at debug level

Use "gh desk config [command] --help" for more information about a command.
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
open the config file in review.editor, $VISUAL or $EDITOR, then check it

Usage:
  gh desk config edit [flags]

Flags:
  -h, --help   help for edit

Global Flags:
      --config string   the config file (default "~/.config/gh-desk/config.toml")
      --debug           log at debug level
```

## gh desk config init

```
write the config file with every setting at its default, unless one is there

Usage:
  gh desk config init [flags]

Flags:
  -h, --help   help for init

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
