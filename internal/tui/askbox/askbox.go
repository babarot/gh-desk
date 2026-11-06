// Package askbox holds what the text boxes that ask Claude share and that is
// not drawn: the questions asked before, and the place among them while the
// user goes back through them.
package askbox

import (
	"slices"
	"strings"
	"time"
)

// Entry is a question asked before.
type Entry struct {
	ID       string
	At       time.Time
	Tab      string
	Question string
	// Reply is the first line of the answer, when one came.
	Reply string
}

// Store keeps the questions between launches.
type Store interface {
	// Load reads the questions, the newest first.
	Load() ([]Entry, error)
	// Add keeps a question as it is sent.
	Add(e Entry) error
	// Reply keeps the first line of the answer to the question id.
	Reply(id, text string) error
}

// Prepend puts e first in list, the newest first, in place of the same
// question asked earlier.
func Prepend(list []Entry, e Entry) []Entry {
	out := []Entry{e}
	for _, o := range list {
		if o.Question != e.Question {
			out = append(out, o)
		}
	}
	return out
}

// Unique leaves the newest of each question in list, the newest first.
func Unique(list []Entry) []Entry {
	var out []Entry
	for _, e := range list {
		if !slices.ContainsFunc(out, func(o Entry) bool { return o.Question == e.Question }) {
			out = append(out, e)
		}
	}
	return out
}

// Hits are the questions of list that hold q, ignoring case.
func Hits(list []Entry, q string) []Entry {
	q = strings.ToLower(q)
	var out []Entry
	for _, e := range list {
		if strings.Contains(strings.ToLower(e.Question), q) {
			out = append(out, e)
		}
	}
	return out
}

// Search is the place among the hits of a search.
type Search struct{ at int }

// At is the place in n hits.
func (s *Search) At(n int) int { return min(s.at, max(n-1, 0)) }

// Next moves to the next of n hits, round to the first.
func (s *Search) Next(n int) {
	if n > 0 {
		s.at = (s.At(n) + 1) % n
	}
}

// Prev moves to the previous of n hits, round to the last.
func (s *Search) Prev(n int) {
	if n > 0 {
		s.at = (s.At(n) + n - 1) % n
	}
}

// Reset goes back to the first hit.
func (s *Search) Reset() { s.at = 0 }

// Shown are the first and the end of the at most room hits shown of n, the
// one at at among them.
func Shown(at, n, room int) (from, to int) {
	from = max(at-room+1, 0)
	return from, min(from+room, n)
}

// Recall is the place among the questions, the newest first, while the user
// goes back through them, and the text that was typed before. The zero value
// is out of them.
type Recall struct {
	in    bool
	at    int
	typed string
}

// Active reports whether the text box holds a question recalled.
func (r *Recall) Active() bool { return r.in }

// At is the place of the question recalled in the list, from 0.
func (r *Recall) At() int { return r.at }

// Older moves to the question before the one recalled, keeping typed when it
// is the first move; ok is false when there is none.
func (r *Recall) Older(list []Entry, typed string) (string, bool) {
	switch {
	case !r.in && len(list) > 0:
		r.in, r.at, r.typed = true, 0, typed
	case r.in && r.at+1 < len(list):
		r.at++
	default:
		return "", false
	}
	return list[r.at].Question, true
}

// Newer moves to the question after the one recalled, or, past the newest,
// out of them, back to the text that was typed. The list may have become
// shorter meanwhile.
func (r *Recall) Newer(list []Entry) string {
	r.at = min(r.at, len(list))
	if r.at == 0 {
		typed := r.typed
		r.Leave()
		return typed
	}
	r.at--
	return list[r.at].Question
}

// Leave goes out of the questions; the text box keeps what it holds.
func (r *Recall) Leave() { *r = Recall{} }
