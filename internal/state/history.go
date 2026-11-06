package state

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"time"
)

// historyVersion is the format of a line of the history; a line of another
// format is not read.
const historyVersion = 1

// historyLimit is how many questions the history keeps.
const historyLimit = 500

// History keeps the questions asked of Claude about the lists, under
// triage/<host> in the state directory: a line as each is sent and a line
// with the first line of its answer, joined by ID when they are read. See
// ADR-096.
//
// Every write holds a lock on a file that is never replaced, and opens the
// history only once it holds it: the history itself is replaced when it is
// read, so a lock on it, or a file left open, would miss a write of another
// gh-desk.
type History struct {
	dir string
}

// HistoryEntry is a question and the first line of its answer.
type HistoryEntry struct {
	ID       string
	At       time.Time
	Tab      string
	Question string
	Reply    string
}

type historyLine struct {
	V     int       `json:"v"`
	ID    string    `json:"id"`
	At    time.Time `json:"at,omitzero"`
	Tab   string    `json:"tab,omitempty"`
	Q     string    `json:"q,omitempty"`
	Reply string    `json:"reply,omitempty"`
}

// OpenHistory returns the history kept in dir for host.
func OpenHistory(dir, host string) *History {
	if host == "" {
		host = "github.com"
	}
	return &History{dir: filepath.Join(dir, "triage", host)}
}

func (h *History) path() string { return filepath.Join(h.dir, "history.jsonl") }

// locked runs f holding the lock of the history.
func (h *History) locked(f func() error) error {
	if err := os.MkdirAll(h.dir, 0o700); err != nil {
		return err
	}
	l, err := os.OpenFile(filepath.Join(h.dir, "history.lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return err
	}
	defer l.Close()
	if err := waitLock(l); err != nil {
		return err
	}
	return f()
}

func (h *History) append(l historyLine) error {
	l.V = historyVersion
	b, err := json.Marshal(l)
	if err != nil {
		return err
	}
	return h.locked(func() error {
		f, err := os.OpenFile(h.path(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
		if err != nil {
			return err
		}
		_, werr := f.Write(append(b, '\n'))
		return errors.Join(werr, f.Close())
	})
}

// Add keeps a question as it is sent, so it stays when gh-desk quits before
// the answer.
func (h *History) Add(e HistoryEntry) error {
	return h.append(historyLine{ID: e.ID, At: e.At, Tab: e.Tab, Q: e.Question})
}

// Reply keeps the first line of the answer to the question id.
func (h *History) Reply(id, text string) error {
	return h.append(historyLine{ID: id, Reply: text})
}

// Load reads the questions, the newest first, the newest of each question
// and at most historyLimit, and writes the history back as that. A line it
// cannot read is left out; when one is of another format, the file is left
// as it is, so an older gh-desk does not lose what a newer one wrote.
func (h *History) Load() ([]HistoryEntry, error) {
	var out []HistoryEntry
	err := h.locked(func() error {
		b, err := os.ReadFile(h.path())
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		var other bool
		var lines int
		out, lines, other = parseHistory(b)
		if other || lines == writtenLines(out) {
			return nil
		}
		return h.rewrite(out)
	})
	return out, err
}

// parseHistory reads the lines of b, and tells how many there were and
// whether one was of another format.
func parseHistory(b []byte) (out []HistoryEntry, lines int, other bool) {
	var asked []historyLine
	replies := map[string]string{}
	for line := range bytes.Lines(b) {
		lines++
		var l historyLine
		if json.Unmarshal(line, &l) != nil {
			continue
		}
		switch {
		case l.V != historyVersion:
			other = true
		case l.Q != "":
			asked = append(asked, l)
		case l.Reply != "":
			replies[l.ID] = l.Reply
		}
	}
	slices.SortStableFunc(asked, func(a, b historyLine) int { return b.At.Compare(a.At) })
	seen := map[string]bool{}
	for _, l := range asked {
		if seen[l.Q] || len(out) == historyLimit {
			continue
		}
		seen[l.Q] = true
		out = append(out, HistoryEntry{ID: l.ID, At: l.At, Tab: l.Tab, Question: l.Q, Reply: replies[l.ID]})
	}
	return out, lines, other
}

// writtenLines is how many lines the entries take.
func writtenLines(es []HistoryEntry) int {
	n := len(es)
	for _, e := range es {
		if e.Reply != "" {
			n++
		}
	}
	return n
}

// rewrite replaces the history with es, through a temporary file of its own,
// so a crash leaves the old one or the new one. The lock is held.
func (h *History) rewrite(es []HistoryEntry) error {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	for _, e := range slices.Backward(es) {
		if err := enc.Encode(historyLine{V: historyVersion, ID: e.ID, At: e.At, Tab: e.Tab, Q: e.Question}); err != nil {
			return err
		}
		if e.Reply != "" {
			if err := enc.Encode(historyLine{V: historyVersion, ID: e.ID, Reply: e.Reply}); err != nil {
				return err
			}
		}
	}
	tmp, err := os.CreateTemp(h.dir, "history-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	_, werr := tmp.Write(b.Bytes())
	if err := errors.Join(werr, tmp.Close()); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), h.path())
}
