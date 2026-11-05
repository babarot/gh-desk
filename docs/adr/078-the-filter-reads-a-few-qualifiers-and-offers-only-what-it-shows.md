# ADR-078: The filter reads a few qualifiers and offers only what it shows

## Status

Accepted

## Context

The filter typed with `/` looked for its text in the key, the title and the author of a pull request joined into one string. A name meant as an author also matched every title that held it, and a text could match across the end of one field and the start of the next. There was no way to ask one field alone, and nothing on the screen would have told the user that there was.

Titles of pull requests often start with a word and a colon (`fix:`, `feat:`, `chore(deps):`), so a filter that reads any `word:` as a field would stop finding them.

## Decision

- The filter is words split by spaces, every one of which must hold, in any order. A word that starts with one of a fixed set of qualifiers and a colon asks that field alone; any other word is looked for in the key, the title or the author, each on its own, never across them. The qualifiers are kept in one list in [internal/tui/filter.go](../../internal/tui/filter.go), which matching, drawing and completion all read, so a word is never colored or completed as a qualifier that the filter does not read as one.
- Only that set is read. A word with any other name before a colon is looked for as it is, so the words titles start with are still found.
- `author:` asks for a login that starts with the value, a bot's without `app/`; the other qualifiers ask for a field that contains it, a title as written or as drawn (ADR-077). A qualifier with no value yet asks nothing, so the rows do not all go while it is being typed.
- The filter line shows which words it reads as qualifiers, and while the filter is typed it offers them and the values that may follow. What it offers is only shown: nothing is filled in without the complete action, as a word of a title may start like a qualifier.
- The values offered for a qualifier come from the rows the tab shows under the rest of the filter, asked by the same question that decides whether a row is shown (ADR-027, ADR-062). A completion never names a pull request that is put away, excluded or narrowed out, and never leaves no rows.
- Claude's triage is told the same words ([internal/claude/triage.go](../../internal/claude/triage.go)), as it sets the filter by its text (ADR-027).

## Alternatives

- Rejected: reading any `word:` as a qualifier, and an unknown one as an error. It would stop finding the titles that start with such a word.
- Rejected: keeping the words that are not qualifiers as one phrase, matched as before. Its matches across fields are what the filter had wrong, and it reads unlike the qualifiers beside it.
- Rejected: matching the qualifiers with the rules of `S` (`plan.Rule`). A rule selects only open pull requests that are not drafts, and asks for an author's login in full; the filter shows every row the tab holds and is matched as it is typed.
- Rejected: listing the values to choose under the filter line. It changes how many lines the list takes and where the mouse finds its rows; the values go round in place instead.

## Consequences

- A new qualifier goes into the one list, and the triage instructions must tell Claude of it.
- A new place that offers values of the rows must ask the question that decides whether a row is shown, not read the tab's pull requests directly.

## Not decided here

- Qualifiers on fields read later than the list, such as labels, checks or size. A row whose field is not read yet would have to be shown or hidden before it is known, which is what the rules of `S` already handle.
- Negating a word.
