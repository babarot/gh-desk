package app

import (
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/babarot/gh-desk/internal/model"
	"github.com/babarot/gh-desk/internal/runner"
	"github.com/babarot/gh-desk/internal/state"
)

// pendingLock is the lock on the record of pending merges for a gh-desk
// started while another held it. Once that one quits, Take takes the lock
// and hands the record to the runner, so merges may wait from then on.
type pendingLock struct {
	dir string
	rn  *runner.Runner
	// mu guards store and closed: Take runs off the TUI's loop.
	mu     sync.Mutex
	store  *state.Store
	closed bool
}

func (l *pendingLock) Take() ([]model.Pending, bool, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	switch {
	case l.closed:
		return nil, false, nil
	case l.store != nil:
		return nil, true, nil
	}
	s, err := state.Open(l.dir)
	if err != nil || s == nil {
		return nil, false, err
	}
	l.store = s
	l.rn.Adopt(s)
	return s.Pending(), true, nil
}

func (l *pendingLock) Holder() (int, string, bool) {
	pid, dir, ok := state.Holder(l.dir)
	return pid, homeShort(dir), ok
}

// close releases the lock if it was taken, and stops taking it.
func (l *pendingLock) close() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.closed = true
	if l.store != nil {
		l.store.Close()
	}
}

// homeShort writes a directory under the home directory as ~/….
func homeShort(dir string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return dir
	}
	if dir == home {
		return "~"
	}
	if rest, ok := strings.CutPrefix(dir, home+string(filepath.Separator)); ok {
		return "~" + string(filepath.Separator) + rest
	}
	return dir
}
