package jsonc

import (
	"bytes"
	"context"
	"fmt"

	"github.com/ubgo/textdata/atomicfile"
)

// Update is the safe read-modify-write of a JSONC file shared by several
// processes: it takes the cross-process lock for path (atomicfile.Lock),
// opens path, calls fn with the document, saves it if fn changed it, and
// releases the lock. Two processes updating the same file one key each both
// keep their edit; with Open, Set and Save alone, the later Save would write
// back a copy read before the earlier one landed.
//
// ctx bounds only the wait for the lock: when its deadline passes the error
// wraps atomicfile.ErrLockTimeout. A missing file opens empty, as with Open,
// and is created at 0600 only if fn adds something. A file with syntax errors
// is refused before fn runs. When fn returns an error nothing is written and
// that error is returned as is. When fn leaves the bytes unchanged nothing is
// written, so a no-op Update does not touch the file's mtime. Save runs every
// SaveGuard as usual.
//
// fn must not keep d or call SetPath on it: the lock covers path only, and is
// gone once Update returns. The lock is advisory, so every writer of the file
// has to go through Update (or atomicfile.Lock) for it to hold.
// ds:def id=update-v2hk4zns owner=@maintainers stability=stable
func Update(ctx context.Context, path string, fn func(d *Doc) error, opts ...Option) error {
	lk, err := atomicfile.Lock(ctx, path)
	if err != nil {
		return fmt.Errorf("jsonc: update %s: %w", path, err)
	}
	// The unlock error is dropped deliberately: by then the save has either
	// succeeded (and is durable) or its own error is being returned, and the
	// OS releases the lock with the descriptor regardless.
	defer func() { _ = lk.Close() }()

	d, err := Open(path, opts...)
	if err != nil {
		return err
	}
	before := d.Render()
	if err := fn(d); err != nil {
		return err
	}
	if bytes.Equal(before, d.src) && (d.existed || len(d.src) == 0) {
		return nil
	}
	return d.Save()
}
