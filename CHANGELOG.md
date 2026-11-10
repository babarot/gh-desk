# Changelog

## [v0.17.0](https://github.com/babarot/gh-desk/compare/v0.16.0...v0.17.0) - 2026-11-10
### New Features
- Turn a pull request either way between draft and ready with D by @babarot in https://github.com/babarot/gh-desk/pull/121
- Show in the list and the pane that a pull request conflicts by @babarot in https://github.com/babarot/gh-desk/pull/127
- Copy the URLs of the pull request or the selection with y by @babarot in https://github.com/babarot/gh-desk/pull/130
### Improvements
- Name the tab that could not be read in its error by @babarot in https://github.com/babarot/gh-desk/pull/128
- Keep the cursor on the pull request the user put it on by @babarot in https://github.com/babarot/gh-desk/pull/129

## [v0.16.0](https://github.com/babarot/gh-desk/compare/v0.15.0...v0.16.0) - 2026-11-09
### New Features
- Let the TUI run offline, take new settings, and draw on a given terminal by @babarot in https://github.com/babarot/gh-desk/pull/117
- Run gh-desk itself on the settings page by @babarot in https://github.com/babarot/gh-desk/pull/119
- Tell, condition by condition, what a rule makes of a pull request by @babarot in https://github.com/babarot/gh-desk/pull/124
- List what a rule on the settings page matches, and where by @babarot in https://github.com/babarot/gh-desk/pull/125
### Bug fixes
- Wait for every wait before judging that reads are shared by @babarot in https://github.com/babarot/gh-desk/pull/120
- Read a condition left empty as none by @babarot in https://github.com/babarot/gh-desk/pull/123
### Refactorings
- Keep the drawing of gh-desk to what the site shows by @babarot in https://github.com/babarot/gh-desk/pull/122

## [v0.15.0](https://github.com/babarot/gh-desk/compare/v0.14.1...v0.15.0) - 2026-11-09
### New Features
- Merge through the merge queue where the base branch has one by @babarot in https://github.com/babarot/gh-desk/pull/113
### Bug fixes
- Read again when a merge's own approval does not show yet by @babarot in https://github.com/babarot/gh-desk/pull/116
- Keep the menu of Add a condition in view on the settings page by @babarot in https://github.com/babarot/gh-desk/pull/115

## [v0.14.1](https://github.com/babarot/gh-desk/compare/v0.14.0...v0.14.1) - 2026-11-07
### Bug fixes
- Update the branch of a merge that falls behind while it waits by @babarot in https://github.com/babarot/gh-desk/pull/105
- Renumber the ADR of the shared read to 105 by @babarot in https://github.com/babarot/gh-desk/pull/108
### Improvements
- Build the configuration guide around the settings page by @babarot in https://github.com/babarot/gh-desk/pull/103
- Share one light read among the waits of a repository by @babarot in https://github.com/babarot/gh-desk/pull/106
- Take the lane of a repository when a merge is ready to write by @babarot in https://github.com/babarot/gh-desk/pull/107
- Give the landing page, the docs and the playground pages of their own by @babarot in https://github.com/babarot/gh-desk/pull/109
- Show the settings panel and the file it writes on the landing page by @babarot in https://github.com/babarot/gh-desk/pull/111
- Switch the settings page between gh-desk and the file it makes by @babarot in https://github.com/babarot/gh-desk/pull/112

## [v0.14.0](https://github.com/babarot/gh-desk/compare/v0.13.0...v0.14.0) - 2026-11-07
### New Features
- Replace the config file from standard input with config apply by @babarot in https://github.com/babarot/gh-desk/pull/100
- Edit the settings on a page with gh desk config edit --ui by @babarot in https://github.com/babarot/gh-desk/pull/101
- Show the lists last read on the settings page by @babarot in https://github.com/babarot/gh-desk/pull/102
### Bug fixes
- Keep the cursor and the mark at the start of the line by @babarot in https://github.com/babarot/gh-desk/pull/98

## [v0.13.0](https://github.com/babarot/gh-desk/compare/v0.12.0...v0.13.0) - 2026-11-06
### New Features
- Keep the questions of a triage and recall them from the text box by @babarot in https://github.com/babarot/gh-desk/pull/95
- Search the questions in place of the text box by @babarot in https://github.com/babarot/gh-desk/pull/96
- Complete what is typed in the text box, and name pull requests in a triage by @babarot in https://github.com/babarot/gh-desk/pull/97
### Improvements
- Let a confirmed action use up its selection by @babarot in https://github.com/babarot/gh-desk/pull/90
- Send a comment through the confirm popup and the runner by @babarot in https://github.com/babarot/gh-desk/pull/92
- Draw a triage as the claude pane draws a conversation by @babarot in https://github.com/babarot/gh-desk/pull/93
- Let a triage go on under the popup, and keep its plan on its tab by @babarot in https://github.com/babarot/gh-desk/pull/94

## [v0.12.0](https://github.com/babarot/gh-desk/compare/v0.11.0...v0.12.0) - 2026-11-06
### New Features
- Write a short config file, and tell of new settings after an update by @babarot in https://github.com/babarot/gh-desk/pull/87
- Approve pull requests that match [[auto_approve]] rules while gh-desk runs by @babarot in https://github.com/babarot/gh-desk/pull/86
### Improvements
- List the arrays of tables in one place, and tell of new settings as one by @babarot in https://github.com/babarot/gh-desk/pull/89

## [v0.11.0](https://github.com/babarot/gh-desk/compare/v0.10.0...v0.11.0) - 2026-11-06
### New Features
- Let Clawd play a scene picked at random while the worktree is made by @babarot in https://github.com/babarot/gh-desk/pull/83
- Ring the terminal's bell when the inbox finds a notification newly unread by @babarot in https://github.com/babarot/gh-desk/pull/85

## [v0.10.0](https://github.com/babarot/gh-desk/compare/v0.9.0...v0.10.0) - 2026-11-06
### New Features
- Show the release notes from the ? list with w by @babarot in https://github.com/babarot/gh-desk/pull/81

## [v0.9.0](https://github.com/babarot/gh-desk/compare/v0.8.0...v0.9.0) - 2026-11-06
### New Features
- Tell of a newer release and of an update in the notifications by @babarot in https://github.com/babarot/gh-desk/pull/79

## [v0.8.0](https://github.com/babarot/gh-desk/compare/v0.7.0...v0.8.0) - 2026-11-06
### New Features
- Give Claude read-only tools over the pull request's repository by @babarot in https://github.com/babarot/gh-desk/pull/70
- Greet an empty claude pane with Claude's mascot and the pull request by @babarot in https://github.com/babarot/gh-desk/pull/71
### Bug fixes
- Draw golden views on a light background and on every theme by @babarot in https://github.com/babarot/gh-desk/pull/78
### Improvements
- Make merges of a repository wait their turn across batches by @babarot in https://github.com/babarot/gh-desk/pull/73
- Count what a narrowed tab's rows hold in its title by @babarot in https://github.com/babarot/gh-desk/pull/74
- Put a review's result away at the automatic refresh after an interval by @babarot in https://github.com/babarot/gh-desk/pull/75
- Keep an opening bracket off a line's end and fit a table's columns to the pane by @babarot in https://github.com/babarot/gh-desk/pull/76
- Compare the colors of a golden view as well as its text by @babarot in https://github.com/babarot/gh-desk/pull/77

## [v0.7.0](https://github.com/babarot/gh-desk/compare/v0.6.0...v0.7.0) - 2026-11-05
### New Features
- Let the filter ask one field with author:, repo: and title: by @babarot in https://github.com/babarot/gh-desk/pull/66
### Improvements
- Let ctrl+w in the filter stop at a slash or a colon by @babarot in https://github.com/babarot/gh-desk/pull/65
- Count only the excluded the filter matches while it filters by @babarot in https://github.com/babarot/gh-desk/pull/67
- Draw the frame of what takes the keys in the focus color by @babarot in https://github.com/babarot/gh-desk/pull/69

## [v0.6.0](https://github.com/babarot/gh-desk/compare/v0.5.1...v0.6.0) - 2026-11-05
### New Features
- Draw shortcodes in a description as their emoji by @babarot in https://github.com/babarot/gh-desk/pull/61
- Draw a title's shortcodes as emoji, and keep the title as written by @babarot in https://github.com/babarot/gh-desk/pull/63

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
