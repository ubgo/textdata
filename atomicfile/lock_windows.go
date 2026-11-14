//go:build windows

package atomicfile

import (
	"errors"
	"os"
	"syscall"
	"unsafe"
)

// LockFileEx and UnlockFileEx come from kernel32 through the standard
// library's lazy DLL loader, so the base module stays stdlib only (no
// golang.org/x/sys). kernel32.dll is a KnownDLL: it always loads from the
// system directory, never from the working directory.
var (
	kernel32         = syscall.NewLazyDLL("kernel32.dll")
	procLockFileEx   = kernel32.NewProc("LockFileEx")
	procUnlockFileEx = kernel32.NewProc("UnlockFileEx")
)

// LockFileEx flags and the error it returns for a range locked elsewhere.
const (
	lockfileFailImmediately = 0x00000001
	lockfileExclusiveLock   = 0x00000002
	errorLockViolation      = syscall.Errno(33) // ERROR_LOCK_VIOLATION
)

// lockRangeLow and lockRangeHigh lock the whole file range (all bits set), the
// convention for a whole-file lock; only the range matters, not the content.
const (
	lockRangeLow  = ^uint32(0)
	lockRangeHigh = ^uint32(0)
)

// tryLock takes an exclusive LockFileEx without blocking. held=false with a
// nil error means another handle holds it. Windows releases the lock when the
// handle closes or the process ends.
func tryLock(f *os.File) (held bool, err error) {
	var ol syscall.Overlapped
	r, _, e := procLockFileEx.Call(f.Fd(), lockfileExclusiveLock|lockfileFailImmediately, 0,
		uintptr(lockRangeLow), uintptr(lockRangeHigh), uintptr(unsafe.Pointer(&ol)))
	if r != 0 {
		return true, nil
	}
	if errors.Is(e, errorLockViolation) {
		return false, nil
	}
	return false, e
}

// unlock releases the range tryLock locked.
func unlock(f *os.File) error {
	var ol syscall.Overlapped
	r, _, e := procUnlockFileEx.Call(f.Fd(), 0, uintptr(lockRangeLow), uintptr(lockRangeHigh), uintptr(unsafe.Pointer(&ol)))
	if r == 0 {
		return e
	}
	return nil
}
