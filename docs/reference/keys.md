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
| `up` `ctrl+p` `k` | `up` | up |
| `down` `ctrl+n` `j` | `down` | down |
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
| `l` `right` | `expand` | unfold section / next pane tab |
| `h` `left` | `collapse` | fold section / previous pane tab |
| `space` | `select` | select |
| `V` | `select_shown` | select shown |
| `S` | `select_rule` | select by rule |
| `>` | `next_scope` | next in the scope |
| `<` | `prev_scope` | previous in the scope |
| `O` | `choose_scope` | choose in the scope |
| `u` | `clear_selection` | clear / restore selection |
| `A` | `approve` | approve |
| `M` | `merge` | merge |
| `X` | `stop` | stop |
| `D` | `draft` | draft / ready |
| (none) | `close` | close |
| `c` | `comment` | comment |
| `R` | `review` | review |
| `C` | `triage` | triage with claude |
| `i` | `ask_claude` | ask claude about it |
| `o` | `browse` | browser |
| `y` | `copy_url` | copy url |
| `d` | `hide` | hide / done |
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

## A popup's list or text box

| Key | Action | Does |
|---|---|---|
| `enter` | `choose` | choose |
| `space` | `toggle_rule` | check a rule |
| `a` | `add_to_selection` | add to the selection |
| `f` | `filter_by_rule` | show only these |
| `i` | `write` | write |

## A confirm popup

| Key | Action | Does |
|---|---|---|
| `y` `enter` | `confirm` | yes |
| `f` | `include_failing` | include failing |
| `w` | `wait_for_checks` | wait for checks |
| `x` | `delete_branch` | delete branches |

## Claude on the list

| Key | Action | Does |
|---|---|---|
| `enter` | `triage_apply` | apply |

## The log

| Key | Action | Does |
|---|---|---|
| `f` | `log_this_pr` | this PR only |

## Asking claude

| Key | Action | Does |
|---|---|---|
| `ctrl+x` | `draft_review` | draft review |
| `ctrl+o` | `open_claude` | open in claude code |
| `ctrl+p` | `history_prev` | previous question |
| `ctrl+n` | `history_next` | next question |
| `ctrl+r` | `history_search` | search the questions |
| `backspace` | `chat_close` | back, claude goes on |

## Writing a review

| Key | Action | Does |
|---|---|---|
| `tab` | `next_kind` | kind |
| `ctrl+s` | `post` | post |
| `ctrl+e` | `editor` | $EDITOR |

## A text box

| Key | Action | Does |
|---|---|---|
| `enter` | `accept` | accept |
| `tab` | `complete` | complete |
| `backspace` | `erase` | erase |
| `ctrl+w` | `erase_word` | erase word |

## This list

| Key | Action | Does |
|---|---|---|

## Mouse

| | |
|---|---|
| click | tab, row, pane tab, ✦: events |
| double click | actions |
| click twice | a PR, a user, a branch, a fact: browser; an item of a popup: choose |
| click outside | a popup: close |
| wheel | scroll |
| drag | the pane's top edge: resize |
