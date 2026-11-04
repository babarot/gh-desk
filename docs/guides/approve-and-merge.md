# Approving and merging

An approval or a merge cannot be taken back on GitHub, so every write goes through a confirm popup first. `A` approves, `M` merges, and `R` writes a review of any kind; each acts on the selection, or on the pull request under the cursor.

## The confirm popup

The popup reads each pull request again and lists what will be done to it, and why the rest is skipped: closed, conflicting, required checks failing, a job failed, no merge method the repository allows. It marks the pull requests that change files in `bulk.attention_paths`, such as workflows, and with `claude.release_notes_check`, the dependency updates whose release notes call for care.

[The bots' updates, selected by a rule, approved and merged](https://babarot.me/gh-desk/?scenario=merge "playground")

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

After an approve, the approved pull requests stay selected, so `M` merges the same set next.

## Following a batch

While a batch goes on, each pull request's row says what it is doing now. `L` opens the log beside the list, as it is written: when each wait started and ended and why, every write with its request ID, and why a pull request was skipped. While it is open it takes the keys: `j`/`k` and the page keys scroll it, the bottom follows the newest again, `f` keeps only the pull request the cursor was on, and `L` or `esc` closes it. On a narrow screen it takes the place of the preview.

While a batch runs, the top of it has a line for each pull request: the steps it has been through, the one it is on with how long so far, and those expected ahead, which can change as gh-desk decides again after each write. A wait says what it waits for. A batch that ends while the log is open stays there, done, until it is closed. The status line counts how the batch stands: done, waiting, skipped, stopped.

The log shows what the log file has, and no more; `--debug` adds every read.

## Waiting survives quitting

There is no background process. What a merge is waiting for is recorded, and the next launch goes on with it, reading each pull request again first. Only the time gh-desk runs counts against the timeouts. A second gh-desk started meanwhile skips merges that would wait.

See [`[merge]`](../reference/configuration.md#merge) and [`[checks]`](../reference/configuration.md#checks) for the settings.
