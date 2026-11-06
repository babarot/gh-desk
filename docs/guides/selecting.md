# Selecting by rules

Approve, merge, review, hide and done act on the selection when there is one, and on the pull request under the cursor when there is none. A key never acts on the cursor's pull request while something is selected: it acts on the selection, or refuses and says why.

`space` selects one, `V` selects every row shown, and `u` clears the selection. With nothing selected, `u` brings the last selection back, without what left the list or was put away since.

Confirming an action takes its pull requests out of the selection, however it ends; closing the popup without confirming takes nothing out. `.` tries again what failed or stopped, without touching the selection.

## Rules

A rule picks pull requests by facts gh-desk reads: who opened them and in which repository, what a dependency update bumps, their size, age and labels, how they are reviewed, and how their checks are. Rules are written in `[[select]]` of the config file, and a pull request matches a rule when it meets every key the rule gives:

```toml
[[select]]
name      = "safe bumps"
authors   = ["renovate"]
packages  = ["golang.org/x/net", "actions/checkout"]
checks    = ["passing"]
max_lines = 50
```

Five rules are there by default: `small`, `dependency bots`, `approved`, `recent` and `lock files`. Rules you write replace all five, as tabs do; copy the ones you want to keep from `gh desk config reference select`, or write `select = []` at the top of the file to have none. Besides them, the built-in "approved by me" picks the bots' pull requests you approved that are still open.

## The popup of S

`S` opens the rules. In the popup:

| Key | |
|---|---|
| `enter` | select what the rule under the cursor matches, replacing the selection |
| `space` | check a rule; with rules checked, `enter` selects what matches all of them |
| `a` | add what matches to the selection instead of replacing it |
| `f` | show only these rows in the tab, leaving the selection as it is |

[The rules of S](https://babarot.me/gh-desk/?scenario=rules "playground")

Checking rules is AND; adding to the selection is OR, so (A and B) or C is chosen in two steps. A combination you use again is better written as one rule.

The rows a tab is narrowed to with `f` are kept per tab, and matched again at every draw: a pull request approved, merged or pushed to shows or leaves as the rules say of it now. `esc` takes the narrowing away, together with the filter typed.

See [`[[select]]`](../reference/configuration.md#select) for every key of a rule.
