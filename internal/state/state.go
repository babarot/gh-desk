// Package state keeps the merges gh-desk is waiting for, so they go on
// at the next launch: pending.json under the state directory, next to a
// lock only one gh-desk holds at a time. The operating system releases
// the lock when its holder exits, however it exits, so a crash never
// leaves it behind.
package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sync"

	"github.com/babarot/gh-desk/internal/model"
)

// DefaultDir is $XDG_STATE_HOME/gh-desk, or ~/.local/state/gh-desk.
func DefaultDir() string {
	dir := os.Getenv("XDG_STATE_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			home = "."
		}
		dir = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(dir, "gh-desk")
}

// Store is the record of pending merges, held under the lock.
type Store struct {
	path string
	lock *os.File
	mu   sync.Mutex
	// items are by pull request, in the order they were first put.
	items []model.Pending
}

// Open takes the lock in dir and reads the record. It returns a nil Store
// and no error when another gh-desk holds the lock.
func Open(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filepath.Join(dir, "pending.lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	ok, err := tryLock(f)
	if err != nil || !ok {
		f.Close()
		return nil, err
	}
	writeHolder(f)
	s := &Store{path: filepath.Join(dir, "pending.json"), lock: f}
	b, err := os.ReadFile(s.path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
	case err != nil:
		f.Close()
		return nil, err
	default:
		if err := json.Unmarshal(b, &s.items); err != nil {
			f.Close()
			return nil, fmt.Errorf("%s: %w", s.path, err)
		}
	}
	return s, nil
}

// holder is who holds the lock, written in the lock file so another
// gh-desk can say which one keeps the merges that wait.
type holder struct {
	PID int    `json:"pid"`
	Dir string `json:"dir"`
}

// writeHolder writes this process into the lock file it holds. A failure
// leaves the others saying "another gh-desk" without naming it.
func writeHolder(f *os.File) {
	dir, _ := os.Getwd()
	b, err := json.Marshal(holder{PID: os.Getpid(), Dir: dir})
	if err != nil {
		return
	}
	if f.Truncate(0) == nil {
		_, _ = f.WriteAt(b, 0)
	}
}

// Holder reads who holds the lock in dir: its process ID and the directory
// it was started in. It reports false when the lock file names no one, as
// one written by an older gh-desk does.
func Holder(dir string) (pid int, wd string, ok bool) {
	b, err := os.ReadFile(filepath.Join(dir, "pending.lock"))
	if err != nil {
		return 0, "", false
	}
	var h holder
	if json.Unmarshal(b, &h) != nil || h.PID <= 0 {
		return 0, "", false
	}
	return h.PID, h.Dir, true
}

// Pending is the record as it was read or last written.
func (s *Store) Pending() []model.Pending {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.items)
}

// Put records p, replacing what was recorded for its pull request.
func (s *Store) Put(p model.Pending) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	i := slices.IndexFunc(s.items, func(x model.Pending) bool { return x.Key == p.Key })
	if i < 0 {
		s.items = append(s.items, p)
	} else {
		s.items[i] = p
	}
	return s.write()
}

// Remove forgets a pull request.
func (s *Store) Remove(k model.PRKey) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := len(s.items)
	s.items = slices.DeleteFunc(s.items, func(x model.Pending) bool { return x.Key == k })
	if len(s.items) == n {
		return nil
	}
	return s.write()
}

// write replaces the file whole, through a temporary file, so a crash
// leaves the old record or the new one, never half of one.
func (s *Store) write() error {
	b, err := json.MarshalIndent(s.items, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

// Close releases the lock.
func (s *Store) Close() error { return s.lock.Close() }
