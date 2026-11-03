# ADR-043: A review goes through the confirm popup and the runner

## Status

Accepted

## Context

A review reached GitHub by three ways. Approve (A) went through the confirm popup and the runner of ADR-006: each pull request read again, skipped with its reason, failing checks asked about, and read again before the write. A review written with R, and one Claude drafted, was sent from its own popup straight to GitHub, on the head the list had read but with none of those checks, so approving with R could approve what A would have stopped. R took one pull request, so a short approving message could not be sent to many. A was, in what it does, R with approve and no body, yet it was the one that checked more.

## Decision

- Every review gh-desk sends goes through the confirm popup and the runner, as approve and merge do (ADR-006). Writing it and sending it are two steps: the text is written first, then the popup reads each pull request again and lists what will be sent and why the rest is skipped.
- R writes a review of the selection or of the pull request under the cursor, of any kind. A is R with approve and no body: it opens the popup at once.
- `internal/plan` decides whether each pull request takes the review, by its kind; the runner decides again before it writes, and stops one whose head moved.
- A review is not recorded for the next launch. ADR-009 says this of approve batches, since an approval is a single write that ends at once; this extends it to every review, and adds that the record would hold its body on disk. One cut off by quitting is told as not known to have reached GitHub.
- After a review sent to every pull request, approved ones stay selected, so `M` merges them next; the others leave the selection. When any was not sent, only those are left selected and the text is kept for the next review, so nothing is sent twice.
- A draft from Claude is a review of its conversation's pull request alone, whatever is selected when it comes.
- This replaces, in ADR-042, "Review and asking Claude refuse: a review and a conversation are about one pull request read on its own." with "Asking Claude refuses: a conversation is about one pull request read on its own."
- Rejected: a message field in A's popup, leaving R as it was. It gives approve with a message to many, but leaves R's own review unchecked.
- Rejected: sending each review straight from R when only one pull request is chosen. One pull request is not safer than many: the head can move and checks can fail as well.

## Consequences

- A single review waits for one more read before it is sent.
- A new kind of write to a pull request goes through the popup and the runner too, or says in its ADR why not.
- The runner holds a review's body in memory only.
