# Comparison

gh-desk is not the first terminal tool for GitHub. [gh-dash](https://github.com/dlvhdr/gh-dash) and [gh-news](https://github.com/chmouel/gh-news) are good ones, and gh-desk took ideas from gh-news. They differ in what they are built around, and that decides which one fits a day of work.

## gh-dash: a dashboard of GitHub

gh-dash is a dashboard of pull requests and issues, in sections you define with search filters, per repository if you like. It aims to let you do from the terminal what you would do on GitHub: read a diff, comment, check out, update, merge, and run custom actions for your own workflow.

gh-desk is narrower. It has only pull requests, and is built around the queue of reviews and merges that wait on you:

- A search tab is split by whom the review is asked of, you, a team, or neither, with the bots apart, so that what needs you stands out from what needs a team.
- Your notifications are part of the desk: they mark the pull requests they concern, and the inbox lists them.
- Approving and merging is done for many at once: rules select them, and one confirm popup plans each by the rules of its repository, then follows it through auto-merge, branch updates and waiting for checks.
- Claude Code is part of the desk: a conversation about a pull request in a worktree of its head, and a plan for the tab.

If you want issues, or one tool for everything you do on GitHub, gh-dash fits better.

## gh-news: your notifications

gh-news is a notifications reader. Its unit is the notification: you filter, group, pin, snooze and mark them, and preview the issue or pull request each is about, with the comment that triggered it.

gh-desk starts from the pull request instead. A notification says something happened; it stays after the pull request is merged, so it is not a list of what is left to do. In gh-desk the lists are searches for pull requests, and notifications point at them. gh-desk took two ideas from gh-news: showing the comment that triggered a notification, and a description view in the preview.

If you mostly read notifications, of issues and discussions as well as pull requests, gh-news fits better.

## Using them together

All three install as gh extensions, so you can keep more than one. gh-desk is for the part of the day spent getting pull requests reviewed and merged.
