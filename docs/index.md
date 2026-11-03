# gh-desk

A desk in your terminal for the pull requests waiting on you.

gh-desk is a [gh](https://cli.github.com/) extension. It opens the pull requests asked of you, yours, the ones you are involved in, and your notifications, each in a tab. You read a pull request in a pane under the list, select many by a rule, approve and merge them through one popup that says what will happen to each, and ask Claude about the one you are unsure of.

[The playground](https://babarot.me/gh-desk/) runs gh-desk itself in your browser, against a made-up GitHub. The pages here embed it where a screen says more than words: press "Play" on one, and it plays the way to that screen; "try it yourself" opens it on its own page, where you drive.

[gh-desk as it starts](https://babarot.me/gh-desk/ "playground")

## Where to go

- [Getting started](getting-started.md): install it, run it, and the screen you see first
- [Concepts](concepts/README.md): what gh-desk aims for, the [principles](concepts/principles.md) it keeps, and [how it differs from other tools](concepts/compared.md)
- [Guides](guides/README.md): how to use it, from [tabs and groups](guides/tabs.md) to [approving and merging](guides/approve-and-merge.md), [Claude](guides/ask-claude.md) and [the config file](guides/configuration.md)
- [Reference](reference/README.md): every [command](reference/commands.md), [setting](reference/configuration.md) and [key](reference/keys.md), written from the code, so they say what the version on main does

The decisions behind the design are kept as [ADRs](adr/) in the repository.
