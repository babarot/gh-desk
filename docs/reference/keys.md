# Keys

<!-- Generated from internal/tui/keymap by TestKeysReferenceIsUpToDate. Do not edit; run go test ./internal/tui -run TestKeysReferenceIsUpToDate -update. -->

`?` lists every key, for the screen it is opened from: that screen's section comes first and unfolded. The line at the bottom of the screen shows the keys that matter there. These are the default keys.

Keys are bound by the name of their action, and each action's keys can be changed under `[keys]` in the [config file](configuration.md): a key or a list of keys, replacing the action's own, or `[]` to turn it off. A key given to an action is taken from an action that has it by default where they meet. `ctrl+c` and `esc` are fixed.

```toml
[keys]
approve = "a"
merge = ["M", "ctrl+m"]
browse = []
```

## Anywhere

| Key | Action | Does |
|---|---|---|
| `q` | `quit` | quit |
| `?` | `help` | keys |
| `r` | `refresh` | refresh |
| `n` | `events` | notifications |
| `ctrl+c` | | quit, from anywhere; cannot be changed |
| `esc` | | back one level, everywhere; cannot be changed |

## Moving

| Key | Action | Does |
|---|---|---|
| `up` `k` | `up` | up |
| `down` `j` | `down` | down |
| `home` `g` | `top` | top |
| `end` `G` | `bottom` | bottom |
| `pgup` `ctrl+u` | `page_up` | page up |
| `pgdown` `ctrl+d` | `page_down` | page down |

## The list and its preview

| Key | Action | Does |
|---|---|---|
| `tab` | `next_tab` | next tab |
| `shift+tab` | `prev_tab` | previous tab |
| `1` | `tab_1` | tab 1 |
| `2` | `tab_2` | tab 2 |
| `3` | `tab_3` | tab 3 |
| `4` | `tab_4` | tab 4 |
| `5` | `tab_5` | tab 5 |
| `6` | `tab_6` | tab 6 |
| `7` | `tab_7` | tab 7 |
| `8` | `tab_8` | tab 8 |
| `9` | `tab_9` | tab 9 |
| `enter` | `menu` | actions |
| `l` `right` | `expand` | unfold group / next pane tab |
| `h` `left` | `collapse` | fold group / previous pane tab |
| `space` | `select` | select |
| `V` | `select_shown` | select shown |
| `S` | `select_rule` | select by rule |
| `u` | `clear_selection` | clear selection |
| `A` | `approve` | approve |
| `M` | `merge` | merge |
| `D` | `ready_for_review` | ready for review |
| (none) | `close` | close |
| `c` | `comment` | comment |
| `R` | `review` | review |
| `C` | `triage` | triage with claude |
| `i` | `ask_claude` | ask claude about it |
| `o` | `browse` | browser |
| `d` | `hide` | hide / done |
| `b` | `fold_bots` | fold bots |
| `a` | `show_excluded` | excluded |
| `/` | `filter` | filter |
| `J` | `preview_down` | scroll pane down |
| `K` | `preview_up` | scroll pane up |
| `]` | `next_file` | next file tab / part |
| `[` | `prev_file` | previous file tab / part |
| `v` | `preview_tab` | overview / diff |
| `z` | `fold` | fold file / part |
| `p` | `preview` | preview pane |
| `+` | `preview_grow` | taller pane |
| `-` | `preview_shrink` | shorter pane |
| `L` | `log` | log |

## A menu or a list popup

| Key | Action | Does |
|---|---|---|
| `enter` | `choose` | choose |
| `space` | `toggle_rule` | check a rule |
| `a` | `add_to_selection` | add to the selection |
| `f` | `filter_by_rule` | show only these |

## A confirm popup

| Key | Action | Does |
|---|---|---|
| `y` `enter` | `confirm` | yes |
| `f` | `include_failing` | include failing |
| `x` | `delete_branch` | delete branches |
| `i` | `write_comment` | write a comment |

## Claude on the list

| Key | Action | Does |
|---|---|---|
| `enter` | `triage_apply` | apply |
| `i` | `triage_more` | ask more |

## The log

| Key | Action | Does |
|---|---|---|
| `f` | `log_this_pr` | this PR only |

## Claude on a pull request

| Key | Action | Does |
|---|---|---|
| `tab` | `fill_suggestion` | fill in the suggestion |
| `ctrl+r` | `draft_review` | draft review |
| `ctrl+o` | `open_claude` | open in claude code |
| `backspace` | `chat_close` | back, claude goes on |

## Writing a review

| Key | Action | Does |
|---|---|---|
| `tab` | `next_kind` | kind |
| `ctrl+s` | `post` | post |
| `ctrl+e` | `editor` | $EDITOR |

## Mouse

| | |
|---|---|
| click | tab, row, pane tab |
| double click | actions |
| click twice | @you, an author, a PR: browser; an item of a popup: choose |
| click outside | a popup: close |
| wheel | scroll |
| drag | the pane's top edge: resize |
