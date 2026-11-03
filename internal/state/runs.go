package state

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Runs keeps a directory for each running gh-triage, for files that live
// only as long as it does (what is given to Claude Code). Each holds a lock
// while its gh-triage runs, so one whose lock is free was left by a
// gh-triage that did not exit cleanly.
type Runs struct {
	Dir string
}

const (
	runPrefix = "run-"
	runLock   = ".lock"
	// youngRun is how new a directory Sweep leaves alone, as its gh-triage
	// may not have taken its lock yet.
	youngRun = time.Minute
)

// New makes a directory for this run and takes its lock. release removes
// it; the lock goes with the process however it exits, and the directory
// is then left for Sweep.
func (r Runs) New() (dir string, release func(), err error) {
	if err := os.MkdirAll(r.Dir, 0o700); err != nil {
		return "", nil, err
	}
	dir, err = os.MkdirTemp(r.Dir, runPrefix)
	if err != nil {
		return "", nil, err
	}
	f, err := os.OpenFile(filepath.Join(dir, runLock), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		os.RemoveAll(dir)
		return "", nil, err
	}
	if ok, err := tryLock(f); err != nil || !ok {
		f.Close()
		os.RemoveAll(dir)
		return "", nil, errors.Join(errors.New("could not lock the directory of this run"), err)
	}
	writeHolder(f)
	return dir, func() {
		os.RemoveAll(dir)
		f.Close()
	}, nil
}

// Sweep removes the directories of runs no longer alive, and returns how
// many it removed. Where a lock is not seen by another process, it cannot
// tell, and removes none.
func (r Runs) Sweep() (int, error) {
	if !locks {
		return 0, nil
	}
	es, err := os.ReadDir(r.Dir)
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	removed := 0
	for _, e := range es {
		if !e.IsDir() || !strings.HasPrefix(e.Name(), runPrefix) {
			continue
		}
		dir := filepath.Join(r.Dir, e.Name())
		// One just made may not hold its lock yet.
		if fi, err := e.Info(); err != nil || time.Since(fi.ModTime()) < youngRun {
			continue
		}
		f, err := os.OpenFile(filepath.Join(dir, runLock), os.O_RDWR, 0)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return removed, err
		}
		ok, err := tryLock(f)
		if err != nil || !ok {
			f.Close()
			continue
		}
		err = os.RemoveAll(dir)
		f.Close()
		if err != nil {
			return removed, err
		}
		removed++
	}
	return removed, nil
}
