package dotenv

import (
	"context"
	"fmt"

	"github.com/ubgo/textdata/atomicfile"
)

// Update is the safe read-modify-write of a .env file shared by several
// processes: it takes the cross-process lock for path (atomicfile.Lock),
// opens path, calls fn with the file, saves it if fn changed it, and releases
// the lock. Two processes setting one key each both keep their edit; with
// Open, Set and Save alone, the later Save would write back a copy read
// before the earlier one landed.
//
// ctx bounds only the wait for the lock: when its deadline passes the error
// wraps atomicfile.ErrLockTimeout. A missing file opens empty, as with Open,
// and is created at 0600 only if fn adds something. When fn returns an error
// nothing is written and that error is returned as is. When fn leaves the
// rendered bytes unchanged nothing is written, so a no-op Update does not
// touch the file's mtime. Save runs every SaveGuard as usual.
//
// fn must not keep f or change f.Path: the lock covers path only, and is gone
// once Update returns. The lock is advisory, so every writer of the file has
// to go through Update (or atomicfile.Lock) for it to hold.
// ds:def id=update-yzkcq24k owner=@maintainers stability=stable
func Update(ctx context.Context, path string, fn func(f *File) error, opts ...Option) error {
	lk, err := atomicfile.Lock(ctx, path)
	if err != nil {
		return fmt.Errorf("dotenv: update %s: %w", path, err)
	}
	// The unlock error is dropped deliberately: by then the save has either
	// succeeded (and is durable) or its own error is being returned, and the
	// OS releases the lock with the descriptor regardless.
	defer func() { _ = lk.Close() }()

	f, err := Open(path, opts...)
	if err != nil {
		return err
	}
	before := f.Render()
	if err := fn(f); err != nil {
		return err
	}
	if after := f.Render(); after == before && (f.existed || after == "") {
		return nil
	}
	return f.Save()
}
