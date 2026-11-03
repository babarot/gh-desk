# Commands

<!-- Generated from internal/app by TestCommandsReferenceIsUpToDate. Do not edit; run go test ./internal/app -run TestCommandsReferenceIsUpToDate -update. -->

Installed as a gh extension, gh-desk runs as `gh desk`; installed with Go or Nix, as `gh-desk`. With no command, it opens the pull requests waiting on you.

## gh desk

```
Usage: gh desk [flags] [command]

Commands:
  config     check, show or edit the config file (gh desk config help)
  gc         remove the worktrees of pull requests no longer open
  log        print where the log file is

Flags:
  -config string
    	the config file (default "~/.config/gh-desk/config.toml")
  -debug
    	log at debug level
  -no-cache
    	neither show nor keep the lists last read
  -version
    	print the version
```

## gh desk config

```
Usage: gh desk config <command>

Commands:
  check [path]    read the config file and print its mistakes and warnings,
                  without starting; exits 1 when it has mistakes
  show [--diff]   print the settings in effect, or with --diff only those that
                  differ from the defaults
  init            write the config file with every setting at its default,
                  unless one is there
  edit            open the config file in review.editor, $VISUAL or $EDITOR,
                  then check it
  schema          print the JSON Schema of the config file

The config file is the one --config names: by default
$XDG_CONFIG_HOME/gh-desk/config.toml, or ~/.config/gh-desk/config.toml.
```
