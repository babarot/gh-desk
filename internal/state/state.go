// Package state keeps the merges gh-triage is waiting for, so they go on
// at the next launch: pending.json under the state directory, next to a
// lock only one gh-triage holds at a time. The operating system releases
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

	"github.com/babarot/gh-triage/internal/model"
)

// DefaultDir is $XDG_STATE_HOME/gh-triage, or ~/.local/state/gh-triage.
func DefaultDir() string {
	dir := os.Getenv("XDG_STATE_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			home = "."
		}
		dir = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(dir, "gh-triage")
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
// and no error when another gh-triage holds the lock.
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
