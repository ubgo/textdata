// Package extends loads the targets of include references ("$extends") for
// every textdata format.
//
// It is a plugin.Includer: install it on a document (for example
// jsonc.WithPlugin(extends.New())) and the format resolves its include
// references through it. Without an Includer installed, a format treats
// "$extends" as an ordinary key, so reading a file never opens another file
// unless the caller asked.
//
// Two includers are provided:
//
//   - New reads from the local disk. A reference is resolved relative to the
//     folder of the including file. Absolute references are allowed unless
//     AllowAbsolute(false); WithRoot confines every target to one folder,
//     checked on the real path and opened through os.Root, so a symlink
//     cannot lead outside it.
//   - NewFS reads from an fs.FS (embed.FS, testing/fstest.MapFS, os.DirFS).
//     The FS is its own root: absolute references and references climbing
//     above it are refused.
//
// Both refuse targets over a size limit (WithMaxSize, 10 MiB by default), so a
// reference to a device or a huge file cannot exhaust memory.
//
// The package imports only the textdata base; it knows nothing about any
// format, so one includer serves dotenv, jsonc and any later format.
package extends

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	pathpkg "path"
	"path/filepath"
	"strings"

	"github.com/ubgo/textdata/plugin"
)

// Name is the plugin name the disk includer reports; errors raised through
// it read `jsonc: plugin "extends": include ../base.jsonc: ...`.
const Name = "extends"

// FSName is the plugin name the fs.FS includer reports.
const FSName = "extends-fs"

// DefaultMaxSize is the largest target read when WithMaxSize is not given.
const DefaultMaxSize int64 = 10 << 20

// ErrTooLarge is returned (wrapped) when a target exceeds the size limit.
var ErrTooLarge = errors.New("extends: target exceeds the size limit")

// ErrNotRegular is returned (wrapped) when a target is a directory, device or
// other non-regular file.
var ErrNotRegular = errors.New("extends: target is not a regular file")

// config is the resolved option set shared by both includers.
type config struct {
	root     string // "" = no confinement (disk only)
	allowAbs bool
	maxSize  int64
}

// Option configures an includer.
type Option func(*config)

// WithRoot confines every target to dir and its subfolders. The check runs on
// the target's real path (symlinks resolved), so a link pointing outside dir
// is refused with plugin.ErrDenied, as is a "../" reference that climbs out.
// A relative dir is taken relative to the working directory at Include time.
// Ignored by NewFS, whose FS is already the root.
func WithRoot(dir string) Option { return func(c *config) { c.root = dir } }

// AllowAbsolute allows (true, the default for New) or refuses (false) an
// absolute reference such as "/etc/app/base.jsonc". A refused reference fails
// with plugin.ErrDenied. NewFS always refuses absolute references.
func AllowAbsolute(allow bool) Option { return func(c *config) { c.allowAbs = allow } }

// WithMaxSize sets the largest target, in bytes, that is read; a bigger one
// fails with ErrTooLarge. n <= 0 restores DefaultMaxSize.
func WithMaxSize(n int64) Option {
	return func(c *config) {
		if n <= 0 {
			n = DefaultMaxSize
		}
		c.maxSize = n
	}
}

func newConfig(opts []Option) config {
	c := config{allowAbs: true, maxSize: DefaultMaxSize}
	for _, o := range opts {
		if o != nil {
			o(&c)
		}
	}
	return c
}

// Disk is the local-disk includer. The zero value is not usable; use New.
type Disk struct {
	cfg config
}

// New returns a disk includer.
func New(opts ...Option) *Disk { return &Disk{cfg: newConfig(opts)} }

// Name implements plugin.Plugin.
func (*Disk) Name() string { return Name }

// Include implements plugin.Includer. ref is resolved against the folder of
// from (the working directory when from is ""); the returned location is the
// cleaned joined path, which is the base for the target's own references and
// what a format compares to detect cycles.
//
// A missing target wraps both fs.ErrNotExist and plugin.ErrNotFound; a
// refused one wraps plugin.ErrDenied.
func (x *Disk) Include(from, ref string) ([]byte, string, error) {
	if ref == "" {
		return nil, "", errors.New("extends: empty reference")
	}
	if strings.IndexByte(ref, 0) >= 0 {
		return nil, "", errors.New("extends: reference contains a NUL byte")
	}
	var loc string
	if filepath.IsAbs(ref) {
		if !x.cfg.allowAbs {
			return nil, "", fmt.Errorf("extends: absolute reference %q: %w", ref, plugin.ErrDenied)
		}
		loc = filepath.Clean(ref)
	} else {
		dir := "."
		if from != "" {
			dir = filepath.Dir(from)
		}
		loc = filepath.Join(dir, ref)
	}
	var f *os.File
	var err error
	if x.cfg.root != "" {
		f, err = x.confine(loc)
	} else {
		f, err = openRegular(loc)
	}
	if err != nil {
		return nil, "", err
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil {
		return nil, "", fmt.Errorf("extends: %s: %w", loc, err)
	}
	if !info.Mode().IsRegular() {
		return nil, "", fmt.Errorf("extends: %s: %w", loc, ErrNotRegular)
	}
	data, err := readLimited(f, x.cfg.maxSize)
	if err != nil {
		return nil, "", fmt.Errorf("extends: %s: %w", loc, err)
	}
	return data, loc, nil
}

// confine refuses loc unless its real path (symlinks resolved) lies inside
// the real root, and opens it through an os.Root at that root: the check and
// the open agree even if a link is swapped in between, because os.Root itself
// refuses to follow a link out of the root. A target that does not exist is
// checked lexically, so a "../" escape reports ErrDenied rather than not
// found.
func (x *Disk) confine(loc string) (*os.File, error) {
	rootAbs, err := filepath.Abs(x.cfg.root)
	if err != nil {
		return nil, fmt.Errorf("extends: root: %w", err)
	}
	rootReal := rootAbs
	if r, err := filepath.EvalSymlinks(rootAbs); err == nil {
		rootReal = r
	}
	target, err := filepath.Abs(loc)
	if err != nil {
		return nil, fmt.Errorf("extends: %s: %w", loc, err)
	}
	denied := fmt.Errorf("extends: %s is outside the root %s: %w", loc, x.cfg.root, plugin.ErrDenied)
	if real, err := filepath.EvalSymlinks(target); err == nil {
		target = real
		if !inside(rootReal, target) {
			return nil, denied
		}
	} else if inside(rootAbs, target) {
		rootReal = rootAbs
	} else if !inside(rootReal, target) {
		return nil, denied
	}
	rel, err := filepath.Rel(rootReal, target)
	if err != nil {
		return nil, denied
	}
	root, err := os.OpenRoot(rootReal)
	if err != nil {
		return nil, fmt.Errorf("extends: root: %w", err)
	}
	defer func() { _ = root.Close() }()
	if info, err := root.Stat(rel); err == nil && !info.Mode().IsRegular() {
		return nil, fmt.Errorf("extends: %s: %w", loc, ErrNotRegular)
	}
	f, err := root.Open(rel)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, notFound(loc, err)
		}
		// The real path was inside a moment ago: a refusal now is a link
		// swapped to point out, or a permission problem; both are denials.
		return nil, fmt.Errorf("extends: %s: %w: %w", loc, plugin.ErrDenied, err)
	}
	return f, nil
}

// inside reports whether target is root or below it. Both are absolute and
// clean.
func inside(root, target string) bool {
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

// FS is the fs.FS includer. The zero value is not usable; use NewFS.
type FS struct {
	fsys fs.FS
	cfg  config
}

// NewFS returns an includer reading from fsys. Locations are slash-separated
// FS paths ("conf/base.jsonc"); a reference is resolved against the folder of
// from, and one that is absolute or climbs above the FS root fails with
// plugin.ErrDenied. WithRoot and AllowAbsolute are ignored.
func NewFS(fsys fs.FS, opts ...Option) *FS {
	return &FS{fsys: fsys, cfg: newConfig(opts)}
}

// Name implements plugin.Plugin.
func (*FS) Name() string { return FSName }

// Include implements plugin.Includer.
func (x *FS) Include(from, ref string) ([]byte, string, error) {
	if ref == "" {
		return nil, "", errors.New("extends: empty reference")
	}
	if strings.HasPrefix(ref, "/") || filepath.IsAbs(ref) {
		return nil, "", fmt.Errorf("extends: absolute reference %q: %w", ref, plugin.ErrDenied)
	}
	dir := "."
	if from != "" {
		dir = pathpkg.Dir(from)
	}
	loc := pathpkg.Join(dir, ref)
	if !fs.ValidPath(loc) {
		return nil, "", fmt.Errorf("extends: %q leaves the file system root: %w", ref, plugin.ErrDenied)
	}
	if info, err := fs.Stat(x.fsys, loc); err == nil && !info.Mode().IsRegular() {
		return nil, "", fmt.Errorf("extends: %s: %w", loc, ErrNotRegular)
	}
	f, err := x.fsys.Open(loc)
	if err != nil {
		return nil, "", notFound(loc, err)
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil {
		return nil, "", fmt.Errorf("extends: %s: %w", loc, err)
	}
	if !info.Mode().IsRegular() {
		return nil, "", fmt.Errorf("extends: %s: %w", loc, ErrNotRegular)
	}
	data, err := readLimited(f, x.cfg.maxSize)
	if err != nil {
		return nil, "", fmt.Errorf("extends: %s: %w", loc, err)
	}
	return data, loc, nil
}

// openRegular opens loc after checking it is a regular file: opening a named
// pipe would block until a writer appears, so the type is checked first (and
// again on the open handle, by the caller).
func openRegular(loc string) (*os.File, error) {
	if info, err := os.Stat(loc); err == nil && !info.Mode().IsRegular() {
		return nil, fmt.Errorf("extends: %s: %w", loc, ErrNotRegular)
	}
	f, err := os.Open(loc)
	if err != nil {
		return nil, notFound(loc, err)
	}
	return f, nil
}

// notFound wraps an open error; a missing file also wraps plugin.ErrNotFound
// so a format can try the next includer.
func notFound(loc string, err error) error {
	if errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("extends: %s: %w: %w", loc, plugin.ErrNotFound, err)
	}
	return fmt.Errorf("extends: %w", err)
}

// readLimited reads r to the end, failing once more than max bytes arrive.
func readLimited(r io.Reader, max int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, max+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > max {
		return nil, fmt.Errorf("%w (%d bytes)", ErrTooLarge, max)
	}
	return data, nil
}

// Interface checks.
var (
	_ plugin.Includer = (*Disk)(nil)
	_ plugin.Includer = (*FS)(nil)
	_ plugin.Plugin   = (*Disk)(nil)
	_ plugin.Plugin   = (*FS)(nil)
)
