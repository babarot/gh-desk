# Approving and merging

An approval or a merge cannot be taken back on GitHub, so every write goes through a confirm popup first, but the approvals of the rules you write to approve, below. `A` approves, `M` merges, `R` writes a review of any kind, and `c` comments on the conversation; each acts on the selection, or on the pull request under the cursor.

## The confirm popup

The popup reads each pull request again and lists what will be done to it, and why the rest is skipped: closed, conflicting, required checks failing, a job failed, no merge method the repository allows. It marks the pull requests that change files in `bulk.attention_paths`, such as workflows, and with `claude.release_notes_check`, the dependency updates whose release notes call for care.

[The bots' updates, selected by a rule, approved and merged](https://babarot.me/gh-desk/play/?scenario=merge "playground")

## Only the head you saw

Each write is for the commit shown when you confirmed. If someone pushes in between, the pull request stops with "head changed · confirm again" rather than merging code you have not looked at. The only new head gh-desk takes is one it made itself by updating the branch.

Marking a draft ready is the one write GitHub takes no commit for: gh-desk reads the pull request just before and stops if the head moved, but a push in the moment between is not caught.

## How each is merged

For each pull request, gh-desk decides the steps from what is true when it acts. Marking a draft ready and approving go first, one after the other; then it reads the pull request again and decides the rest, and decides again after each write that follows:

- wait for the jobs at work on the pull request, required or not: its check runs and GitHub Actions workflow runs, for up to `checks.wait_timeout`. A job that fails stops the merge. Right after you confirm, after a draft is marked ready, and after its branch is updated, gh-desk also waits a little for workflows that have not shown yet
- approve, when your approval is needed, once the jobs passed
- merge now, when nothing blocks it
- turn on GitHub's auto-merge, where the repository allows it and `merge.auto_merge` is on, once no job is at work; the popup says that commits pushed later are merged too, as GitHub does
- wait for the rest, where auto-merge is not available: a required check, a second approval, code owners
- update the branch when it is behind its base, as `merge.update_branch` says: by default a person's branch, not a bot's, since bots rebase their own

Commit statuses, such as a policy bot's, are not jobs: gh-desk waits for them only where GitHub blocks the merge on them.

The method is the first of `merge.methods` the repository allows, and the popup shows it. A draft of yours or of a bot is marked ready for review first. Your own pull request that GitHub lets you merge as an admin is merged as one, after its required checks pass, and the popup says so first.

## Approving once the checks pass

When some of the pull requests to approve have jobs at work, the approve popup waits for them by default: each is approved once its check runs and workflow runs have finished, and one whose job failed is skipped, unless failing ones are included with `f`. `w` approves at once instead. An approval with a text, written with `R`, waits the same way; a comment or a request for changes is sent at once.

A confirmed action uses up its selection. To merge what you just approved, press `u` to bring the selection back and then `M`; `M` alone also approves what needs it, once the jobs passed.

When a batch ends with pull requests failed or stopped, the notification says so and `.` opens the same action again for those alone, with its text. `.` is kept until it is used or another action fails, and skipped pull requests are not included.

## Approving by rules

Some pull requests you would approve without reading them, such as a bot's patch updates. An `[[auto_approve]]` rule in the config file approves what it matches while gh-desk runs, without a popup: the rule is what you would have confirmed. Together with the repository's auto-merge, they are merged without you.

```toml
[[auto_approve]]
name = "renovate patches"
authors = ["renovate"]
title = '^chore\(deps\): update .* to v?\d+\.\d+\.\d+$'
checks = ["passing"]
```

A rule takes the conditions of `[[select]]`. It reads its own search, whatever the tabs show: the open pull requests that are not drafts and whose review is asked of you, in the rule's `repos` when each of them can be searched, or the `query` you give it. Write a `query` for pull requests that do not ask you for a review, such as `is:pr is:open author:app/renovate org:acme`, and prefer one that a pull request leaves once approved. A rule with neither a query nor a condition is a mistake: it would approve every review asked of you.

What a rule approves is yours to say, and who may approve what is the repository's: gh-desk does not limit a rule to bots, nor check who pushed to the branch. It approves only what the rule judged:

- on the head the rule judged, after the jobs at work on it have finished; a job that fails stops it. `checks = ["passing"]` holds for a head with no checks reported yet too, which is why it waits
- not a draft, not your own, not one you approved, and not one whose approval a person dismissed on the head it has now
- not one an `[[exclude]]` rule matches, which is how to say "not this label": exclusions leave a pull request alone, in the tabs and from the rules. They are asked again right before the approval is sent
- once for each head while gh-desk runs, so a new push is judged again

Each approval is told in the notifications (`n`) with the rule that asked for it, and logged. `dry_run = true` only tells what a rule would approve. Rules are read with the automatic refresh, and at `r`; with `list.refresh = "off"`, at launch and at `r`. Only the gh-desk that holds the record of pending merges approves by rules, and another one says so once.

## Following a batch

While a batch goes on, each pull request's row says what it is doing now. `L` opens the log beside the list, as it is written: when each wait started and ended and why, the jobs it waits on as each ends and begins, what GitHub's auto-merge waits on, every write with its request ID, and why a pull request was skipped, by the popup or later. While it is open it takes the keys: `j`/`k` and the page keys scroll it, the bottom follows the newest again, `f` keeps only the pull request the cursor was on, and `L` or `esc` closes it. On a narrow screen it takes the place of the preview.

While a batch runs, the top of it has a line for each pull request: the steps it has been through, the one it is on with how long so far, and those expected ahead, which can change as gh-desk decides again after each write. A wait says what it waits for. A batch that ends while the log is open stays there, done, until it is closed. The status line counts how the batch stands: done, waiting, skipped, stopped.

The log shows what the log file has, and no more; `--debug` adds every read.

## Waiting survives quitting

There is no background process. What a merge or an approval is waiting for is recorded, and the next launch goes on with it, reading each pull request again first. Only the time gh-desk runs counts against the timeouts. A second gh-desk started meanwhile skips merges and approvals that would wait.

See [`[merge]`](../reference/configuration.md#merge) and [`[checks]`](../reference/configuration.md#checks) for the settings.
