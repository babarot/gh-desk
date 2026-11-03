# Concepts

gh-desk is a desk for the pull requests that reach you: the ones waiting for your review, your own, and the ones your notifications point at, in one place, with what you need to read them and land them.

## The problem

Pull requests reach you in three ways that do not line up.

- Review requests, to you and to every team you are on. In a busy organization most go to teams, so the requests addressed to you in person are a few among many.
- Your own pull requests, waiting for someone else.
- Notifications, which say something happened. They stay after the pull request is reviewed or merged, so the notification list is not a list of what is left to do.

On top of that, many of the pull requests are opened by bots: dependency updates, lock file maintenance, release pull requests. Each is small, and there are many; merging them one by one in a browser is most of the work, and the part least worth doing by hand.

## What gh-desk aims for

- The pull request is what you work on. A tab is a GitHub search for pull requests; notifications mark the pull requests they concern, and the inbox lists them with why each is there.
- You see whom a review is asked of: you, a team, or neither, with the bots apart. What needs you is not hidden among what needs a team.
- You read a pull request without leaving the list: its description, checks, reviews and diff are in the pane under it.
- You land many at once, safely. Rules select what can go together, and one confirm popup says what will happen to each pull request, by the rules of its repository, before anything is sent.
- Claude helps you read, and you decide. Claude Code reviews a pull request with tools that only read, and what it proposes, you apply; what it drafts, you send.

gh-desk does not try to be all of GitHub in the terminal. It has no issues, no repository browsing, and no editing of code; it is for the pull requests that wait on you.

## Read on

- [Principles](principles.md): the rules gh-desk keeps, and why
- [Comparison](compared.md): how gh-desk differs from gh-dash and gh-news, and which fits when
- [Guides](../guides/README.md): how to use it
