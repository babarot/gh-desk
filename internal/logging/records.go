package logging

import (
	"context"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"time"
)

// DefaultRecent is how many records Records keeps: enough to look back
// over a batch and the readings around it, small enough to hold in
// memory for the whole run.
const DefaultRecent = 2000

// Record is a log record as Records keeps it: what the log file has for
// it, with the attributes added by With and those of the call in order.
// Groups are flattened into dotted keys.
type Record struct {
	Time  time.Time
	Level slog.Level
	Msg   string
	Attrs []slog.Attr
}

// Attr is the value of the attribute key as a string, and whether the
// record has it.
func (r Record) Attr(key string) (string, bool) {
	for _, a := range r.Attrs {
		if a.Key == key {
			return a.Value.String(), true
		}
	}
	return "", false
}

// Records keeps the latest records written through a logger made with
// it, so that what the log file has can be read back while gh-desk runs.
// It is safe to use from many goroutines.
type Records struct {
	mu   sync.Mutex
	max  int
	now  func() time.Time
	recs []Record
	// wake is closed when a record is added, and replaced.
	wake chan struct{}
}

// NewRecords keeps up to max records, DefaultRecent when max is not
// positive, timed by now, time.Now when nil. The time is not slog's, so
// that tests can fix it.
func NewRecords(max int, now func() time.Time) *Records {
	if max <= 0 {
		max = DefaultRecent
	}
	if now == nil {
		now = time.Now
	}
	return &Records{max: max, now: now, wake: make(chan struct{})}
}

// All is the records kept, oldest first.
func (r *Records) All() []Record {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.recs)
}

// Changed is closed when the next record is added.
func (r *Records) Changed() <-chan struct{} {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.wake
}

func (r *Records) add(rec Record) {
	r.mu.Lock()
	defer r.mu.Unlock()
	rec.Time = r.now()
	r.recs = append(r.recs, rec)
	if over := len(r.recs) - r.max; over > 0 {
		r.recs = slices.Delete(r.recs, 0, over)
	}
	close(r.wake)
	r.wake = make(chan struct{})
}

// handler adds the records it handles to Records.
type handler struct {
	recs  *Records
	level slog.Leveler
	attrs []slog.Attr
	group string
}

// Handler is a slog handler that keeps what it handles in r, at level and
// above.
func (r *Records) Handler(level slog.Leveler) slog.Handler {
	if level == nil {
		level = slog.LevelInfo
	}
	return &handler{recs: r, level: level}
}

func (h *handler) Enabled(_ context.Context, l slog.Level) bool { return l >= h.level.Level() }

func (h *handler) Handle(_ context.Context, rec slog.Record) error {
	attrs := slices.Clone(h.attrs)
	rec.Attrs(func(a slog.Attr) bool {
		attrs = flatten(attrs, h.group, a)
		return true
	})
	h.recs.add(Record{Level: rec.Level, Msg: rec.Message, Attrs: attrs})
	return nil
}

func (h *handler) WithAttrs(as []slog.Attr) slog.Handler {
	n := *h
	n.attrs = slices.Clone(h.attrs)
	for _, a := range as {
		n.attrs = flatten(n.attrs, h.group, a)
	}
	return &n
}

func (h *handler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	n := *h
	n.group = join(h.group, name)
	return &n
}

// flatten appends a to attrs under group, a group's attributes as dotted
// keys, and drops what slog drops: empty keys and empty groups.
func flatten(attrs []slog.Attr, group string, a slog.Attr) []slog.Attr {
	a.Value = a.Value.Resolve()
	if a.Value.Kind() == slog.KindGroup {
		for _, g := range a.Value.Group() {
			attrs = flatten(attrs, join(group, a.Key), g)
		}
		return attrs
	}
	if a.Key == "" {
		return attrs
	}
	a.Key = join(group, a.Key)
	return append(attrs, a)
}

func join(group, key string) string {
	if group == "" {
		return key
	}
	if key == "" {
		return group
	}
	return strings.Join([]string{group, key}, ".")
}
