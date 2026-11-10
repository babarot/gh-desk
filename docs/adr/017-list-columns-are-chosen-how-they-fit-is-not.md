# ADR-017: The list's columns are chosen; how they fit is not

## Status

Accepted

## Context

The list has eight columns: PR, TITLE, AUTHOR, CI, REVIEW, WHY, SIZE and AGE. Which of them a user looks at differs from user to user, which ADR-014 makes a setting, and ADR-016 says an order is given by an array.

How the columns fit the terminal is another matter. Widths fixed for one set of columns (AUTHOR going below 110 columns, SIZE below 120) do not fit another: with WHY or SIZE left out, a column would go though there is room for it. What fits follows from the columns shown and their widths, which follow from how the list is drawn.

## Decision

- `list.columns` lists the columns shown, in the order shown. A column left out is not shown. `pr` and `title` are needed, since without them a row does not say which pull request it is; a column named twice is a mistake.
- A column listed is still left out on a tab where it says nothing: WHY on a tab where no row has anything for it, AUTHOR on a tab whose rows are all the viewer's.
- How the columns fit is not a setting. On a narrow terminal some columns are left out, in a fixed order, while TITLE would otherwise be narrower than a minimum; the other columns keep their widths and TITLE narrows instead. The order (`dropFirst`), the minimum (`minTitle`) and each column's width are constants in `internal/tui`: they follow from how the list is drawn, as ADR-014 has such values be.
- A row's progress and result are written where ADR-007 places them, which holds whatever the columns are.

## Consequences

- A request to make the widths or the order columns go in a setting is answered by this ADR; a case for one is decided in a new ADR.
- A new column is added to `columnDefs` and `config.Columns`, and to `dropFirst` if it may go on a narrow terminal.
