package atomicfile

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"
)

// DefaultLockSuffix is appended to the (symlink-resolved) target's name to
// form the lock file's name: config.jsonc is guarded by config.jsonc.lock.
const DefaultLockSuffix = ".lock"

// Poll bounds for Lock. The OS calls used never block (so a context can stop
// the wait), and Lock retries with a doubling sleep between these bounds:
// quick for a lock that is about to be released, cheap for one held long.
const (
	lockPollMin = time.Millisecond
	lockPollMax = 50 * time.Millisecond
)

// ErrLockTimeout reports that Lock gave up because its context's deadline
// passed while another holder kept the lock. The error also wraps
// context.DeadlineExceeded.
// ds:def id=errlocktimeout-vstm9t8y owner=@maintainers stability=stable
var ErrLockTimeout = errors.New("atomicfile: timed out waiting for lock")

// ErrLockUnsupported reports a platform with no OS file lock this package
// uses (anything but the BSDs, Darwin, Linux and Windows, for example
// Solaris, AIX, Plan 9, js and wasip1).
var ErrLockUnsupported = errors.New("atomicfile: file locking is not supported on this platform")

// FileLock is an exclusive, advisory, cross-process lock on one file, held
// from Lock until Close. Advisory: it excludes only other Lock callers (every
// textdata Update helper, and any program that takes the same lock), never a
// plain open or write.
type FileLock struct {
	f    *os.File
	path string
	once sync.Once
	err  error
}

// Lock takes an exclusive lock guarding path, waiting until it is free or
// ctx is done, and returns it held. Use it around a read-modify-write so two
// processes cannot lose each other's edits; Write alone only makes each
// write atomic, it does not stop process B from overwriting what process A
// wrote between B's read and B's write:
//
//	lk, err := atomicfile.Lock(ctx, path)
//	if err != nil { return err }
//	defer lk.Close()
//	// read path, change it, atomicfile.Write(path, ...)
//
// What is locked: a sidecar file, path's symlink-resolved name plus
// DefaultLockSuffix, created at 0600 if missing. Not path itself, because
// Write replaces path by rename, and a lock held on the old file would not
// exclude a process that opens the new one. The sidecar is never removed:
// deleting it while another process waits on it would let a third process
// lock a fresh file, and two holders would both think they own the lock. The
// directory must exist.
//
// The lock is the OS's own (flock on Unix-likes, LockFileEx on Windows), so
// it is released when Close is called, when the file is closed, or when the
// process dies, even by SIGKILL: no stale lock is ever left behind. It is
// held per Lock call, not per process, so two goroutines of one process
// exclude each other too. It does not nest: a second Lock of the same path
// from the holder waits for the first one's Close.
//
// Waiting: without a deadline on ctx, Lock waits as long as the lock is held.
// When ctx's deadline passes the error wraps ErrLockTimeout and
// context.DeadlineExceeded; when ctx is cancelled it wraps context.Canceled.
// Both name the lock file.
// ds:def id=lock-hhsrdwus owner=@maintainers stability=stable
func Lock(ctx context.Context, path string) (*FileLock, error) {
	target, err := Resolve(path)
	if err != nil {
		return nil, err
	}
	lockPath := target + DefaultLockSuffix
	f, err := os.OpenFile(lockPath, os.O_RDWR|os.O_CREATE, DefaultMode)
	if err != nil {
		return nil, fmt.Errorf("atomicfile: open lock file: %w", err)
	}

	wait := lockPollMin
	for {
		held, err := tryLock(f)
		if err != nil {
			_ = f.Close() // the lock error is the one worth reporting
			return nil, fmt.Errorf("atomicfile: lock %s: %w", lockPath, err)
		}
		if held {
			return &FileLock{f: f, path: lockPath}, nil
		}
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			_ = f.Close() // nothing was locked; the context error is the result
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return nil, fmt.Errorf("atomicfile: lock %s: %w (%w)", lockPath, ErrLockTimeout, ctx.Err())
			}
			return nil, fmt.Errorf("atomicfile: lock %s: %w", lockPath, ctx.Err())
		case <-timer.C:
		}
		wait = min(wait*2, lockPollMax)
	}
}

// Path returns the lock file's path (the resolved target plus
// DefaultLockSuffix).
func (l *FileLock) Path() string { return l.path }

// Close releases the lock. It is safe to call more than once and on a nil
// FileLock; only the first call does anything, and later calls return its
// result.
func (l *FileLock) Close() error {
	if l == nil {
		return nil
	}
	l.once.Do(func() {
		// Closing the descriptor releases the OS lock on every platform; an
		// explicit unlock first makes the release independent of any other
		// descriptor this process might hold on the same file.
		uerr := unlock(l.f)
		cerr := l.f.Close()
		if uerr != nil {
			l.err = fmt.Errorf("atomicfile: unlock %s: %w", l.path, uerr)
		} else if cerr != nil {
			l.err = fmt.Errorf("atomicfile: close lock %s: %w", l.path, cerr)
		}
	})
	return l.err
}
