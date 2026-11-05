# Changelog

## [v0.5.1](https://github.com/babarot/gh-desk/compare/v0.5.0...v0.5.1) - 2026-11-05
### Improvements
- Log why the popup skipped, rechecks of mergeability, and what auto-merge waits on by @babarot in https://github.com/babarot/gh-desk/pull/54
- Log clues to why reads slow down, rows are missing, or lists are old by @babarot in https://github.com/babarot/gh-desk/pull/55
- Log what Claude is doing, never what is said by @babarot in https://github.com/babarot/gh-desk/pull/58
- Move a mark of its own on the progress pane's lanes by @babarot in https://github.com/babarot/gh-desk/pull/59
- Name the steps of a merge and say what they mean once by @babarot in https://github.com/babarot/gh-desk/pull/60
- Name the attention patterns a pull request matches, not each file by @babarot in https://github.com/babarot/gh-desk/pull/62

## [v0.5.0](https://github.com/babarot/gh-desk/compare/v0.4.0...v0.5.0) - 2026-11-05
### New Features
- Close pull requests with a comment and their branches by @babarot in https://github.com/babarot/gh-desk/pull/46
- Let an approval wait for the jobs at work by @babarot in https://github.com/babarot/gh-desk/pull/51
- Let an approval with a text wait for the jobs too by @babarot in https://github.com/babarot/gh-desk/pull/52
### Bug fixes
- Say "merging" for a merge batch, whatever steps it takes by @babarot in https://github.com/babarot/gh-desk/pull/43
- Show every note of a pull request in the confirm popup by @babarot in https://github.com/babarot/gh-desk/pull/44
- Wait for the jobs at work before merging, required or not by @babarot in https://github.com/babarot/gh-desk/pull/48
### Improvements
- Show a comment sent with c in the pane at once by @babarot in https://github.com/babarot/gh-desk/pull/47
- Open more of what the screen names with clicks by @babarot in https://github.com/babarot/gh-desk/pull/49
- Keep Claude's answers and a sent comment in the notifications by @babarot in https://github.com/babarot/gh-desk/pull/50
- Log the jobs a wait waits on as they end and begin by @babarot in https://github.com/babarot/gh-desk/pull/53

## [v0.4.0](https://github.com/babarot/gh-desk/compare/v0.3.0...v0.4.0) - 2026-11-04
### New Features
- Judge rules in three ways and give them more conditions by @babarot in https://github.com/babarot/gh-desk/pull/35
- Leave pull requests out of every tab by exclusion rules by @babarot in https://github.com/babarot/gh-desk/pull/37
- Tell the rows an exclusion rule waits on, and keep the cursor on them by @babarot in https://github.com/babarot/gh-desk/pull/38
- Let a rule ask for the state of a pull request by @babarot in https://github.com/babarot/gh-desk/pull/41
- Say "not" in a rule by its values by @babarot in https://github.com/babarot/gh-desk/pull/42
### Improvements
- Keep the excluded count in sight, leave it out of the tab's count, and let a rule go unnamed by @babarot in https://github.com/babarot/gh-desk/pull/39
- Keep a row where it was while it shows a result by @babarot in https://github.com/babarot/gh-desk/pull/40

## [v0.3.0](https://github.com/babarot/gh-desk/compare/v0.2.0...v0.3.0) - 2026-11-04
### New Features
- Show the log while gh-desk runs by @babarot in https://github.com/babarot/gh-desk/pull/33
- Show each pull request's progress above the log by @babarot in https://github.com/babarot/gh-desk/pull/34
### Improvements
- Build the commands with cobra by @babarot in https://github.com/babarot/gh-desk/pull/28
- Log waits, readings of the lists, and failures shown by @babarot in https://github.com/babarot/gh-desk/pull/30
- Keep the log in memory as it is written by @babarot in https://github.com/babarot/gh-desk/pull/31
- Say the runner's steps and a forecast of those ahead by @babarot in https://github.com/babarot/gh-desk/pull/32

## [v0.2.0](https://github.com/babarot/gh-desk/compare/v0.1.1...v0.2.0) - 2026-11-03
### New Features
- Write the reference pages of the docs from the code by @babarot in https://github.com/babarot/gh-desk/pull/22
- Play scenarios in the playground, and seek by replaying them by @babarot in https://github.com/babarot/gh-desk/pull/23
- Let popups take the mouse by @babarot in https://github.com/babarot/gh-desk/pull/26

## [v0.1.1](https://github.com/babarot/gh-desk/compare/v0.1.0...v0.1.1) - 2026-11-03
### Improvements
- Print the version with --version, not a version command by @babarot in https://github.com/babarot/gh-desk/pull/20

## [v0.1.0](https://github.com/babarot/gh-desk/commits/v0.1.0) - 2026-11-03
### New Features
- Mark a draft of your own or of a bot ready on its way to a merge by @babarot in https://github.com/babarot/gh-desk/pull/8
- Add a playground site that runs gh-desk in the browser by @babarot in https://github.com/babarot/gh-desk/pull/11
- Merge your own blocked pull request as an admin by @babarot in https://github.com/babarot/gh-desk/pull/12
### Bug fixes
- Fix the flaky TestMergeDraftReadiesFirst by @babarot in https://github.com/babarot/gh-desk/pull/10
### Improvements
- Turn one spinner, not dozens, while something is read by @babarot in https://github.com/babarot/gh-desk/pull/17
### Others
- Update the release and pull request setup by @babarot in https://github.com/babarot/gh-desk/pull/1
