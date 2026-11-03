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

One tab is the inbox (`kind = "inbox"`): the pull requests in your notifications, with the reason each is there. Your notifications also put an unread mark (●) on the rows of every tab. The inbox hides the notifications whose pull request is merged or closed until `a` shows them. `d` marks one done.

[The inbox](https://babarot.me/gh-desk/?scenario=inbox "playground")

## Narrowing a tab

`/` filters the rows by what you type. The rules of `S` can narrow them too ([Selecting by rules](selecting.md)); a row is shown when it passes both. The columns are chosen with `list.columns`, and a narrow terminal drops SIZE first, then AUTHOR.

See [`[[tabs]]`](../reference/configuration.md#tabs) for every key of a tab.
