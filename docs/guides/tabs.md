# Tabs and groups

## A tab is a search

Each tab is a GitHub search, written in `[[tabs]]` of the config file, in the order shown. The keys `1` to `9` go to them, so there are at most nine. Tabs you write replace the default four.

```toml
[[tabs]]
name   = "review"
query  = "is:open is:pr review-requested:@me archived:false"
groups = ["you", "team", "bots"]
folded = ["bots"]

[[tabs]]
name  = "infra"
query = "is:open is:pr repo:acme/infra archived:false"

[[tabs]]
kind = "inbox"
name = "inbox"
```

A tab is read in pages: a small first page so it appears in a few seconds, then larger ones up to its `limit`. What only one pull request needs, such as its description and its diff, is read when it is under the cursor.

## Groups

A search tab can be split into groups with `groups`. A group states a fact of the pull request, whatever the query is, and the first that holds wins:

1. `bots`: the author is a bot
2. `you`: a review is requested from you in person
3. `team`: a review is requested from a team
4. `other`: none of the above

The groups are shown in the order you write them. A group left out is not shown, and nothing in it is selected or acted on from that tab. `folded` names the groups folded at start; `h` and `l` fold and unfold the one under the cursor, and `b` folds the bots.

## The inbox

One tab is the inbox (`kind = "inbox"`): the pull requests in your notifications, with the reason each is there. Your notifications also put an unread mark (●) on the rows of every tab. The inbox leaves out the notifications whose pull request is merged or closed, as it leaves out what your exclusion rules match, until `a` shows them; `show_settled = true` keeps them as rows. `d` marks one done. With `list.bell = true`, a notification newly unread while gh-desk runs rings the terminal's bell, unless the inbox excludes it.

[The inbox](https://babarot.me/gh-desk/?scenario=inbox "playground")

## Leaving pull requests out

Rules in `[[exclude]]` leave the pull requests they match out of every tab, the inbox too. They take the keys of `[[select]]` but `packages`, and a pull request is left out when it meets every key of one of them. `name` may be left out; a rule without one is called by its conditions. There is no `not`: a key that takes a list matches one of its values, so `checks = ["failing", "pending"]` is checks not passing, and a key that is `true` or `false` asks for the one or the other, so `bot = false` is a person's pull request:

```toml
[[exclude]]
name                = "old upgrades"
title               = "^Upgrade"
updated_before_days = 30

[[exclude]]
labels = ["wip", "do-not-merge"]
```

A tab says how many it leaves out on the last line of its list, and its count in the tab's title leaves them out. While `/` filters the rows, that line counts only those the filter matches, still out of sight. `a` shows them, dimmed and marked ⊘, and hides them again; the status line and the pane say which rule leaves out the row under the cursor. What is left out is never selected, by a group's header or by `S`, nor put away by `d`. A rule is matched again at every draw, so a pull request leaves or comes back as it changes. One whose CI, size, review or labels are not read yet stays until they are, its PR column shimmering, or marked `?` with `theme.shimmer = false`, and the cursor stays on its pull request when rows above it leave.

## Narrowing a tab

`/` filters the rows by what you type. Each word, split by spaces, must be found in the pull request's key, title or author, in any order and ignoring case, so `retries alice` shows alice's pull requests about retries. A title is drawn with its shortcodes, such as `:sparkles:`, as emoji, and either finds it. A word can ask one field alone:

| Word | Shows the pull requests |
|---|---|
| `author:alice` | whose author's login starts with `alice`; a bot is named without `app/`, as `author:dependabot` |
| `repo:acme/api` | whose owner/repo contains `acme/api` |
| `title:retries` | whose title contains `retries` |

Only these are read: a word such as `fix:` is looked for as it is. The filter line colors the words it reads this way. While you type, an empty filter shows them faint, and the rest of one follows faint once a word starts it; `tab` fills it in. After `author:` or `repo:`, the logins or repositories of the rows the rest of the filter leaves follow faint, the most common first; `tab` fills one in, and `tab` again goes to the next.

The rules of `S` can narrow the rows too ([Selecting by rules](selecting.md)); a row is shown when it passes both. While the rows are narrowed, a tab's count in the header is in the filter's color and counts only what its rows hold, the unread among them in the inbox; the filter narrows every tab, so the header tells which tabs hold what you typed. The columns are chosen with `list.columns`, and a narrow terminal drops SIZE first, then AUTHOR.

See [`[[tabs]]`](../reference/configuration.md#tabs) for every key of a tab.
