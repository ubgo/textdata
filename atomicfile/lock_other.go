//go:build !(darwin || dragonfly || freebsd || linux || netbsd || openbsd || windows)

package atomicfile

import "os"

// tryLock reports ErrLockUnsupported: this platform has no OS lock this
// package uses. A lock-file emulation (exclusive create plus stale detection)
// was rejected on purpose: it cannot tell a slow holder from a dead one, so it
// either leaves stale locks or breaks live ones.
func tryLock(*os.File) (bool, error) { return false, ErrLockUnsupported }

// unlock has nothing to release.
func unlock(*os.File) error { return nil }
