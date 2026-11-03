# Principles

These are the rules gh-desk keeps as it grows. Each is recorded, with what was weighed and rejected, as an [ADR](../adr/) in the repository.

## Nothing is written without a confirm

An approval or a merge cannot be taken back on GitHub. Every write, whether it is one pull request or twenty, goes through a confirm popup that reads each pull request again and says what will happen to it, and why the rest is skipped. A plan from Claude ends in the same popup.

## Each write is for the head you saw

What you confirm is the commit that was on the screen. If someone pushes in between, the pull request stops and asks you to confirm again, rather than merging code you have not looked at. The only new head gh-desk takes is one it made itself by updating the branch.

## The repository's rules decide how to merge

gh-desk does not ask you how to merge each pull request. It reads what the repository allows: the merge methods, auto-merge, whether the branch is behind and who may update it, which checks are required. It decides each pull request's next step from that, and decides again before every write.

## Nothing runs behind your back

There is no background process. A merge that waits for checks is recorded, and goes on at the next launch, after reading the pull request again. When gh-desk quits, its waiting stops; GitHub's auto-merge, once turned on, is GitHub's and goes on without it.

## Claude reads, you act

Claude Code runs on a worktree of the pull request in its restricted mode: no settings or hooks of the pull request's repository, file tools that see only the worktree, and no tool that runs a command or changes a file. The pull request's text is given to it as data, not as instructions. What it proposes is applied by you, and what it drafts is sent by you.

## Settings are what differs between people

A setting exists when people would really want it different: their tabs, their rules, their merge methods, their keys and colors. The rest is decided once in gh-desk, so the config file stays short enough to read. Every key is bound to an action by name, and all but `ctrl+c` and `esc` can be changed.

## A mistake is shown where it is

A mistake in the config file is reported at its line, with what was likely meant, never ignored; `gh desk config check` reports them without starting. A setting that does nothing because of a typo looks like a bug in gh-desk, so it is not left silent.
