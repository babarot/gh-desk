//go:build !(darwin || dragonfly || freebsd || linux || netbsd || openbsd)

package state

import "os"

const locks = false

// waitLock always succeeds where flock(2) is not available.
func waitLock(*os.File) error { return nil }

// tryLock always succeeds where flock(2) is not available, so every
// process acts as the holder.
func tryLock(*os.File) (bool, error) { return true, nil }
