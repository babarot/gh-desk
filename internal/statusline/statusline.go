// Package statusline decides what the status line under the list shows
// (ADR-025). It keeps the events said on it, each for a time of its kind,
// and when the cursor moved onto the row it is on; the work in progress
// and the row's result are built by the caller from its state and passed
// in, never kept here. Its state can be changed only through its methods,
// so nothing else writes to the line.
package statusline

import (
	"strings"
	"time"
)

// Kind is what the line shows.
type Kind int

const (
	// None is an empty line.
	None Kind = iota
	// Info is a success or other information.
	Info
	// Refusal says why the key pressed cannot act here.
	Refusal
	// Failure says that something went wrong.
	Failure
	// Activity is the work in progress.
	Activity
	// Row is the result of the row under the cursor.
	Row
)

// How long each kind of event stays on the line: information as long as
// a toast, which says the same kind of thing; a failure longer, as it may
// need reading twice. RowFor is how long the result of the row the cursor
// moved onto goes before the work in progress. They are constants, not
// settings (ADR-014).
const (
	InfoFor    = 4 * time.Second
	RefusalFor = 4 * time.Second
	FailureFor = 10 * time.Second
	RowFor     = 4 * time.Second
)

type event struct {
	kind Kind
	text string
	at   time.Time
}

func (e event) end() time.Time {
	switch e.kind {
	case Refusal:
		return e.at.Add(RefusalFor)
	case Failure:
		return e.at.Add(FailureFor)
	}
	return e.at.Add(InfoFor)
}

func (e event) live(now time.Time) bool { return e.text != "" && now.Before(e.end()) }

// Line is the state of the status line. The zero Line is empty.
type Line struct {
	last   event
	failed event
	rowAt  time.Time
}

// Say tells of something that happened, such as a write that went through.
// Only what already happened is said; work in progress is passed to Shown.
func (l *Line) Say(now time.Time, text string) { l.tell(Info, now, text) }

// Refuse tells why the key pressed cannot act here. It goes at the next
// key.
func (l *Line) Refuse(now time.Time, text string) { l.tell(Refusal, now, text) }

// Fail tells of something that went wrong.
func (l *Line) Fail(now time.Time, text string) { l.tell(Failure, now, text) }

func (l *Line) tell(k Kind, now time.Time, text string) {
	e := event{kind: k, text: text, at: now}
	l.last = e
	if k == Failure {
		l.failed = e
	}
}

// KeyPressed puts away a refusal, which answers the key before.
func (l *Line) KeyPressed() {
	if l.last.kind == Refusal {
		l.last = event{}
	}
}

// MovedOnto marks that the cursor has just moved onto a row; hasResult
// tells whether the row has a result to show.
func (l *Line) MovedOnto(now time.Time, hasResult bool) {
	l.rowAt = time.Time{}
	if hasResult {
		l.rowAt = now
	}
}

// Shown is what the line shows at now, given the work in progress in the
// order it is shown and the result of the row under the cursor, empty when
// it has none. In order: the newest event still in its time, information
// giving way to a failure still in its time; the row's result for a moment
// after the cursor moved onto it; the work in progress, all of it; the
// row's result.
func (l *Line) Shown(now time.Time, activities []string, row string) (Kind, string) {
	if l.last.live(now) && l.last.kind != Info {
		return l.last.kind, l.last.text
	}
	if l.failed.live(now) {
		return Failure, l.failed.text
	}
	if l.last.live(now) {
		return Info, l.last.text
	}
	if row != "" && !l.rowAt.IsZero() && now.Before(l.rowAt.Add(RowFor)) {
		return Row, row
	}
	if len(activities) > 0 {
		return Activity, strings.Join(activities, " · ")
	}
	if row != "" {
		return Row, row
	}
	return None, ""
}

// Due is the next moment after now at which what the line shows may
// change by time alone, zero when none is coming.
func (l *Line) Due(now time.Time) time.Time {
	var due time.Time
	ends := []time.Time{l.last.end(), l.failed.end()}
	if !l.rowAt.IsZero() {
		ends = append(ends, l.rowAt.Add(RowFor))
	}
	for _, t := range ends {
		if t.After(now) && (due.IsZero() || t.Before(due)) {
			due = t
		}
	}
	return due
}
