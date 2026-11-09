# ADR-112: A rule on the settings page lists what it matches, and where

## Status

Accepted

## Context

The settings page counts, beside each rule, the pull requests it matches (ADR-102). Writing a rule, the count is not enough: what a user needs to know is whether the pull requests meant are the ones matched. A rule of `[[exclude]]` written a character at a time hides, at each pause, what the pattern typed so far matches, and gh-desk on the page (ADR-109) only shows that rows went, not which or why.

The count was of the tab the page showed. With gh-desk itself on the page, nothing on the page chooses that tab any more, and the count was of the first tab whatever tab gh-desk showed.

Changing gh-desk to show this, such as keeping the excluded rows dimmed while the page of those rules is open, was tried and dropped: the page would show gh-desk acting other than it does.

## Decision

- An open rule on the settings page lists the pull requests it matches in every list last read, and those it cannot tell yet, and says for each what every condition made of it: the values it was judged on, the part of the title the pattern matched, and for what cannot be told, what it waits for. gh-desk on the page is not changed for it.
- What matched is told by the code that judges, not judged again on the page: [internal/plan](../../internal/plan) says it condition by condition beside its verdict, sharing each comparison with the verdict, and a test there checks that the two never disagree. The `judge` route returns it with the rows it already returned.
- A rule is judged per row, as gh-desk judges it: the same pull request in the inbox and in a tab can differ, by the notification's reason or by what was read with each list. The page lists a pull request once and names the lists where it matched.
- The count beside a rule is of the pull requests in every list last read, the same set the list shows.
- A title is shown as the rule reads it, before its shortcodes are drawn as emoji, so that the part marked is the part matched.
- The page now gets, for the rows a rule matches or cannot tell, the labels and the reviewers asked that the rule's conditions met. It still never gets a body, a comment or a diff.
- Rejected: showing it in gh-desk on the page. gh-desk would act there other than it does anywhere else.
- Rejected: matching on the page in JavaScript to say where a rule matched. It drifts from gh-desk, as ADR-102 found.
- Rejected: building the verdict from what each condition says. `Judge` says a rule cannot tell before it looks at the conditions on what is read later, and S, the exclusion and auto-approve rely on that.

This replaces, in ADR-102, "The page gets what it draws and no more: never a body, a comment or a diff." with the sentence on what the page now gets, and "The count of an `[[auto_approve]]` rule is of the tab shown, not of what it would approve, as the rule searches on its own." with the count above, which for `[[auto_approve]]` is still not what it would approve.

## Consequences

- A condition added to a rule must be told by `Explain` as well as judged, through the same comparison, or the page lists it as matched without saying why.
- The list is as old as the lists last read, and for `[[auto_approve]]` it is what the rule matches in them, not what its own search finds.
