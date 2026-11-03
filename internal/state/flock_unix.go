//go:build darwin || dragonfly || freebsd || linux || netbsd || openbsd

package state

import (
	"errors"
	"os"
	"syscall"
)

// locks is whether a lock held here is seen by another process.
const locks = true

func tryLock(f *os.File) (bool, error) {
	err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if errors.Is(err, syscall.EWOULDBLOCK) {
		return false, nil
	}
	return err == nil, err
}
