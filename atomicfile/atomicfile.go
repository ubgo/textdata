// Package atomicfile replaces a file's contents so that a reader never sees a
// half-written file and a crash leaves the old contents intact.
//
// The steps: resolve symlinks (so the real file is replaced and the link
// survives), write a sibling temp file at 0600 (never a wider window, even
// briefly, because config files often hold secrets), fsync it, chmod it to the
// target mode, rename it over the target (atomic within a directory), then
// fsync the directory best-effort so the rename itself is durable.
//
// The explicit chmod is required because the mode argument of os.WriteFile and
// os.OpenFile is filtered by the process umask, so it cannot reproduce a
// file's original permissions on its own.
package atomicfile

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// DefaultMode is the mode of a new file when the caller gives none: owner
// read/write only, because the files this package writes are config files that
// often carry secrets.
const DefaultMode fs.FileMode = 0o600

// DefaultTempSuffix is appended to the target's name to form the temp file's
// name, so a leftover from a crash sits next to its file and is recognisable.
const DefaultTempSuffix = ".textdata-tmp"

// maxSymlinkHops bounds how many links Resolve follows before calling it a
// loop, matching the order of magnitude kernels use (Linux: 40).
const maxSymlinkHops = 40

// Options controls one Write. The zero value writes a new file at DefaultMode
// and an existing one at DefaultMode too; set KeepMode to keep an existing
// file's permissions.
type Options struct {
	// Mode is the permission of the written file. 0 means DefaultMode.
	Mode fs.FileMode
	// KeepMode keeps an existing target's permission bits instead of Mode.
	// Overwriting must never widen a file a user deliberately locked down.
	KeepMode bool
	// TempSuffix names the temp file (target + suffix). Empty means
	// DefaultTempSuffix.
	TempSuffix string
	// NoFollow replaces a symlink itself instead of the file it points to.
	NoFollow bool

	// Chmod and Rename replace os.Chmod and os.Rename when set: injection
	// points for tests (their failures cannot be triggered portably) and for
	// sandboxes that route file operations.
	Chmod  func(name string, mode fs.FileMode) error
	Rename func(oldpath, newpath string) error
}

// Write replaces the contents of path with data atomically.
func Write(path string, data []byte, opts Options) error {
	target := path
	if !opts.NoFollow {
		resolved, err := Resolve(path)
		if err != nil {
			return err
		}
		target = resolved
	}

	mode := opts.Mode
	if mode == 0 {
		mode = DefaultMode
	}
	if opts.KeepMode {
		if info, err := os.Stat(target); err == nil {
			mode = info.Mode().Perm()
		}
	}
	suffix := opts.TempSuffix
	if suffix == "" {
		suffix = DefaultTempSuffix
	}
	chmod, rename := os.Chmod, os.Rename
	if opts.Chmod != nil {
		chmod = opts.Chmod
	}
	if opts.Rename != nil {
		rename = opts.Rename
	}

	tmp := target + suffix
	if err := writeSynced(tmp, data); err != nil {
		return err
	}
	// Best-effort cleanup on every failure path. The error is dropped on
	// purpose: after a successful rename the temp name no longer exists and
	// Remove always fails.
	defer func() { _ = os.Remove(tmp) }()

	if err := chmod(tmp, mode); err != nil {
		return fmt.Errorf("atomicfile: set mode on temp file: %w", err)
	}
	if err := rename(tmp, target); err != nil {
		return fmt.Errorf("atomicfile: rename temp file into place: %w", err)
	}
	syncDir(filepath.Dir(target))
	return nil
}

// Resolve follows symlinks from path to the file they name. Unlike
// filepath.EvalSymlinks it accepts a dangling link (the target will be
// created) and a path that does not exist yet (returned as is).
func Resolve(path string) (string, error) {
	cur := path
	for hop := 0; hop < maxSymlinkHops; hop++ {
		info, err := os.Lstat(cur)
		if errors.Is(err, fs.ErrNotExist) {
			return cur, nil
		}
		if err != nil {
			return "", fmt.Errorf("atomicfile: %w", err)
		}
		if info.Mode()&fs.ModeSymlink == 0 {
			return cur, nil
		}
		link, err := os.Readlink(cur)
		if err != nil {
			return "", fmt.Errorf("atomicfile: %w", err)
		}
		if !filepath.IsAbs(link) {
			link = filepath.Join(filepath.Dir(cur), link)
		}
		cur = link
	}
	return "", fmt.Errorf("atomicfile: %s: too many levels of symbolic links", path)
}

// writeSynced creates (or truncates a stale) tmp at 0600, writes data and
// fsyncs it, so the rename that follows publishes durable bytes.
func writeSynced(tmp string, data []byte) error {
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, DefaultMode)
	if err != nil {
		return fmt.Errorf("atomicfile: write temp file: %w", err)
	}
	_, werr := f.Write(data)
	serr := f.Sync()
	cerr := f.Close()
	if err := errors.Join(werr, serr, cerr); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("atomicfile: write temp file: %w", err)
	}
	return nil
}

// syncDir fsyncs a directory so a rename inside it survives a power loss. Some
// platforms cannot open or sync a directory; that only loses durability, never
// correctness, so failures are ignored.
func syncDir(dir string) {
	d, err := os.Open(dir)
	if err != nil {
		return
	}
	_ = d.Sync()
	_ = d.Close()
}
