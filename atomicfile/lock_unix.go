//go:build darwin || dragonfly || freebsd || linux || netbsd || openbsd

package atomicfile

import (
	"errors"
	"os"
	"syscall"
)

// tryLock takes flock(LOCK_EX) without blocking. held=false with a nil error
// means another descriptor holds it. EINTR is retried by the caller's loop.
//
// flock, not fcntl: an fcntl lock belongs to the process and is dropped when
// ANY descriptor of that file is closed, so an unrelated Open+Close of the
// lock file elsewhere in the program would silently release it.
func tryLock(f *os.File) (held bool, err error) {
	err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, syscall.EWOULDBLOCK), errors.Is(err, syscall.EINTR):
		return false, nil
	default:
		return false, err
	}
}

// unlock releases the flock.
func unlock(f *os.File) error {
	return syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
}
