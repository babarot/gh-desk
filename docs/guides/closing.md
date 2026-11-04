# Closing

Close is for the pull requests that will not be merged: a bot's update made obsolete by a later one, a change superseded by another. It has no key of its own, so that a stray key press does not start closing pull requests; choose it from the menu of `enter`, or give it a key under [`[keys]`](../reference/keys.md). Like a merge, it acts on the selection, or on the pull request under the cursor.

## The confirm popup

The popup reads each pull request again and lists what will be done to it, and why the rest is skipped: not open, or not yours to close. Under the list are a check for deleting the head branches and a text box for a comment, both empty when it opens.

- `x` checks or unchecks deleting the head branches. A second click on the check does the same.
- `i` puts the focus in the text box, and `esc` gives it back to the popup. While the box has the focus every key is typed into it, `y` included, and nothing is confirmed; outside it, nothing is typed, and a paste is not taken.
- `y` closes them.

The same comment goes to each pull request, written before it is closed, as GitHub's "Close with comment" does. A comment written in a popup closed without confirming comes back the next time close is opened.

## The branches it keeps

A head branch is deleted only when it can be, and the popup says on each pull request's line why one is kept:

- it is in a fork
- it was already deleted
- you cannot delete it
- an open pull request is based on it: deleting it would close that pull request too
- it was pushed to since it was read

A branch that is kept does not stop the close. A closed pull request can be reopened, and a deleted branch restored, from the pull request on GitHub; a comment cannot be taken back.

## Only the head you saw

Neither the close nor the deletion of a branch takes a commit on GitHub, so gh-desk reads the pull request just before each write and decides again. One whose head moved stops with "head changed · confirm again" before it is closed; after it is closed, its branch is kept. A comment gh-desk sent shows in the pane's comments at once, without reading the pull request again; `r` reads it from GitHub.
