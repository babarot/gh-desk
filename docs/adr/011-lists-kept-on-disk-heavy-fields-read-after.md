# ADR-011: Lists are kept on disk and shown first; heavy fields are read after

## Status

Accepted

## Context

gh-triage read every tab from GitHub at each launch, with no cache. A search of 50 pull requests took about 8 seconds, and a tab of 98 about 18 before it was complete. Measured on 2026-10-30, the search itself took about 1.7 seconds for 50; the time was in the fields GitHub works out for each pull request: `reviewDecision` (about 2 seconds more), the checks' `statusCheckRollup` (1.5 to 2), and `additions`, `deletions` and `changedFiles` (1 to 2). Three searches at once were no slower than one.

gh-news keeps its last notifications in a file, shows them at once and reads them again in the background, and reads each item's details apart from the list. gh-dash keeps nothing on disk; it reads pages of 20 and reads each pull request's details when its row is selected.

## Decision

- The lists last read are kept in `<claude.cache_dir>/lists.json`, mode 0600: for each tab, its search, the total, when it was read and its pull requests; and the inbox with its pull requests. The file belongs to one account on one host (`GH_HOST` or gh's login), and one of another format, host or account is not used, and is replaced whole on the next write. A tab's list is kept by its search string, and a write changes only the lists it names, leaving those of a gh-triage started with another config. Writes go through a temporary file of their own, renamed into place. `--no-cache` turns it off.
- The viewer is read before the screen is shown, as before, and the cache is opened for that viewer: what is shown and what is sent are always of the account gh uses now.
- At launch the cached lists are shown at once, with "as of" and the time they were read, and every tab is read again. A tab with rows gathers the new reading page by page, counting pages by what it gathered, and puts it in place when every page is in, so a list never shrinks to its first page. Rows that still show a result of a batch stay until R (ADR-007), and are not written to the cache. Rows added or changed are marked for two seconds. A reading again that fails leaves the rows, as of when they were read. A tab with no rows (first launch, a failure before) shows each page as it comes, as before.
- A list's search reads only the light fields: who, what, where, the head, who is requested and the latest reviews. The heavy fields (the review decision, the checks' rollup, the line counts, comments, labels) are read after by node ID, ten pull requests a query and five queries at once, the shown tab first. Each result carries its head and is put only in rows of the same pull request at the same head, and only for the tab's latest reading; `PR.HeavyHead` says which head a row's heavy fields are of. A row read again at the same head keeps its heavy fields until they are read again. The inbox reads every field of its pull requests at once, as before.
- A spinner turns on a tab being read, in place of its count, and in the cells whose heavy fields are not read; the screen is drawn every 100 ms only while something is read. `S` waits until the tab's heavy reads are back, and a rule on size or checks does not pick a pull request whose heavy fields are not read for its head.

## Consequences

- With a cache, the lists are on screen about a second after launch; without one, the first rows come in a few seconds and the heavy fields in a few more.
- What is shown can be older than GitHub until the reading again is in. The approve and merge popups read each pull request again before they act (ADR-006), so nothing is approved or merged on what the cache says. A comment or request for changes is sent for the head the row has; after a force push GitHub refuses it, and the error is shown.
- The cache holds the titles and authors of pull requests the account can see, in the user's cache directory, readable only by them. It is never written to the log.
- The notifications are still read in full at each launch; asking with `If-Modified-Since` is left for later.
