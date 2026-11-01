# ADR-017: The list's columns are chosen; how they fit is not

## Status

Accepted

## Context

The list's columns were fixed in `columns()` (`internal/tui/list.go`): PR, TITLE, AUTHOR, CI, REVIEW, WHY, SIZE and AGE, in that order. Which of them a user looks at differs from user to user, which ADR-014 makes a setting, and ADR-016 says an order is given by an array.

How the columns fit the terminal was fixed in the same place. AUTHOR went below 110 columns and SIZE below 120, whatever else was shown, so on a tab with no WHY the title kept ten columns it did not need. Once the columns can be chosen, fixed widths fit only the set they were picked for: with SIZE left out, AUTHOR still went below 110 though there was room for it.

A row's progress and result (`✓ merged`, `waiting for checks`) were written over WHY and SIZE, or REVIEW and SIZE where there is no WHY. With the columns chosen, all of these can be left out, and then a row would show no result, against ADR-007's results kept on the rows until R.

## Decision

- `list.columns` lists the columns shown, in the order shown. A column left out is not shown. `pr` and `title` are needed, since without them a row does not say which pull request it is; a column named twice is a mistake.
- What decides whether a column listed is shown stays as it was: WHY only on grouped tabs and the inbox, AUTHOR not on a tab whose rows are all the viewer's.
- How the columns fit is not a setting. On a narrow terminal SIZE is left out first, then AUTHOR; no other column is left out, and TITLE narrows instead. They are left out while TITLE would be narrower than 27 columns, the width at which SIZE went before (120 columns with all of them on a grouped tab). The order (`dropFirst`), the 27 (`minTitle`) and each column's width are constants in `internal/tui`: they follow from how the list is drawn, as ADR-014 has such values be.
- A row's progress is written where it was when the columns have the cells it used: WHY and SIZE next to each other, or REVIEW and SIZE where there is no WHY, or WHY (REVIEW) alone where there is no SIZE. Otherwise it is written over every column right of TITLE, and when they leave less than 23 columns (the width of WHY and SIZE), the rest is taken from the end of TITLE. A row shows its result whatever the columns are.

## Consequences

- With the default columns the list looks as before at every width, except that AUTHOR stays down to 107 columns on a grouped tab (110 before), and on a tab with no WHY, SIZE and AUTHOR stay down to 108 and 95.
- A request to make the widths or the order columns go in a setting is answered by this ADR; a case for one is decided in a new ADR.
- A new column is added to `columnDefs` and `config.Columns`, and to `dropFirst` if it may go on a narrow terminal.
