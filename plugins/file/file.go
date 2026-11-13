// Package fileplugin resolves ${file:path} references to the contents of a
// file, in every textdata format: ${file:./key.pem} reads key.pem next to the
// document.
//
// It is read-only and reads only when a format asks it to resolve a reference
// it owns. Relative refs resolve against the base directory given to New
// (normally the document's folder), never against the process's working
// directory at resolve time.
//
// Refs are confined by default: the root is the base directory, and a ref that
// leaves it, by "..", an absolute path, or a symlink pointing outside, is
// refused with plugin.ErrDenied before any byte is read. The read itself goes
// through an os.Root, so a symlink swapped in after the check cannot escape
// either. WithRoot moves the root (wider or narrower); Unconfined turns
// confinement off, for documents you fully trust only.
package fileplugin

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/ubgo/textdata/plugin"
)

// Scheme is the reference scheme this plugin owns: ${file:ref}.
const Scheme = "file"

// pluginName is what Name returns, so a failure names this plugin.
const pluginName = "file"

// DefaultMaxSize is the largest file read when WithMaxSize is not given
// (1 MiB): config values are keys and certificates, not data sets, and a ref
// to a huge or endless file must fail fast instead of exhausting memory.
const DefaultMaxSize int64 = 1 << 20

// ErrTooLarge is wrapped when the file is bigger than the size limit.
var ErrTooLarge = errors.New("file too large")

// ErrNotRegular is wrapped when the ref names a directory, device, FIFO or
// socket: reading those could block forever or return something unbounded.
var ErrNotRegular = errors.New("not a regular file")

// Option configures a Plugin.
type Option func(*Plugin)

// WithRoot replaces the default root (the base directory) with root, made
// absolute against the base directory when relative: WithRoot("..") lets a
// document reach its parent folder. Absolute refs, refs whose ".." climbs out
// of root, and symlinks resolving outside root are refused with
// plugin.ErrDenied. It also undoes an earlier Unconfined; the last of the two
// wins. An empty root keeps the default.
func WithRoot(root string) Option {
	return func(p *Plugin) {
		p.root = root
		p.unconfined = false
	}
}

// Unconfined turns confinement off: any path the process can read is allowed,
// absolute refs and symlinks included. For trusted documents only; a document
// from anyone else could then read ${file:/home/you/.ssh/id_ed25519}. The last
// of Unconfined and WithRoot wins.
func Unconfined() Option {
	return func(p *Plugin) { p.unconfined = true }
}

// WithMaxSize sets the largest file, in bytes, the plugin reads; a bigger file
// is an error wrapping ErrTooLarge, never a truncated value. n <= 0 keeps the
// default DefaultMaxSize.
func WithMaxSize(n int64) Option {
	return func(p *Plugin) {
		if n > 0 {
			p.maxSize = n
		}
	}
}

// WithTrimNewline controls whether one trailing "\n" or "\r\n" is removed from
// the contents (default on): a key file saved by an editor ends in a newline
// the value almost never wants. Only one is removed; turn it off to get the
// exact bytes.
func WithTrimNewline(enabled bool) Option {
	return func(p *Plugin) { p.trim = enabled }
}

// Plugin is the file plugin. Safe for concurrent use; it holds no mutable
// state after New.
type Plugin struct {
	base string // absolute base directory for relative refs
	root string // absolute real confinement root, "" when unconfined
	// unconfined is set by Unconfined: no root at all.
	unconfined bool
	maxSize    int64
	trim       bool
	err        error // a construction problem, reported by every Resolve
}

// New returns a file plugin whose relative refs resolve against baseDir
// ("" means the working directory at construction) and are confined to it
// unless WithRoot or Unconfined says otherwise. baseDir and the root are made
// absolute here, once, so a later os.Chdir changes nothing. A problem with
// them (a root that does not exist) is reported by every Resolve call.
func New(baseDir string, opts ...Option) *Plugin {
	p := &Plugin{maxSize: DefaultMaxSize, trim: true}
	for _, o := range opts {
		o(p)
	}
	if baseDir == "" {
		baseDir = "."
	}
	base, err := filepath.Abs(baseDir)
	if err != nil {
		p.err = fmt.Errorf("file: base directory %q: %w", baseDir, err)
		return p
	}
	p.base = base
	if p.unconfined {
		p.root = ""
	} else {
		root := p.root
		if root == "" {
			root = base
		}
		if !filepath.IsAbs(root) {
			root = filepath.Join(base, root)
		}
		// The real root: confinement compares real paths, so a root reached
		// through a symlink (macOS /tmp) still contains its own files.
		real, err := filepath.EvalSymlinks(filepath.Clean(root))
		if err != nil {
			p.err = fmt.Errorf("file: root %q: %w", root, err)
			return p
		}
		p.root = real
		// Compare like with like: the base as a real path too.
		if rb, err := filepath.EvalSymlinks(base); err == nil {
			p.base = rb
		}
	}
	return p
}

// Name implements plugin.Plugin.
func (p *Plugin) Name() string { return pluginName }

// Schemes implements plugin.Resolver.
func (p *Plugin) Schemes() []string { return []string{Scheme} }

// Resolve implements plugin.Resolver: the contents of the file at ref. A
// missing file wraps plugin.ErrNotFound; a ref escaping the root or a
// permission error wraps plugin.ErrDenied; a file over the size limit wraps
// ErrTooLarge; a non-regular file wraps ErrNotRegular.
func (p *Plugin) Resolve(scheme, ref string) (string, error) {
	if !strings.EqualFold(scheme, Scheme) {
		return "", fmt.Errorf("file: scheme %q: %w", scheme, plugin.ErrUnsupportedScheme)
	}
	return p.Read(ref)
}

// Read returns the contents of the file at ref, applying the same rules as
// Resolve. Exported for callers that want a value without a document.
func (p *Plugin) Read(ref string) (string, error) {
	if p.err != nil {
		return "", p.err
	}
	if ref == "" {
		return "", errors.New("file: empty reference")
	}
	if strings.IndexByte(ref, 0) >= 0 {
		return "", fmt.Errorf("file: reference %q contains a NUL byte", ref)
	}
	var data []byte
	var err error
	if p.root != "" {
		data, err = p.readConfined(ref)
	} else {
		data, err = p.readFree(ref)
	}
	if err != nil {
		return "", err
	}
	s := string(data)
	if p.trim {
		if strings.HasSuffix(s, "\r\n") {
			s = s[:len(s)-2]
		} else {
			s = strings.TrimSuffix(s, "\n")
		}
	}
	return s, nil
}

// readFree reads ref with no confinement.
func (p *Plugin) readFree(ref string) ([]byte, error) {
	full := ref
	if !filepath.IsAbs(full) {
		full = filepath.Join(p.base, ref)
	}
	st, err := os.Stat(full)
	if err != nil {
		return nil, classify(ref, err)
	}
	if !st.Mode().IsRegular() {
		return nil, fmt.Errorf("file: %s: %w", ref, ErrNotRegular)
	}
	f, err := os.Open(full)
	if err != nil {
		return nil, classify(ref, err)
	}
	defer f.Close()
	return p.readAll(ref, f)
}

// readConfined reads ref inside the root: a lexical check, a real-path check
// that catches symlinks pointing out, then the read through os.Root, which
// refuses any escape that appears between the check and the open.
func (p *Plugin) readConfined(ref string) ([]byte, error) {
	if filepath.IsAbs(ref) || filepath.VolumeName(ref) != "" || strings.HasPrefix(ref, `\`) || strings.HasPrefix(ref, "/") {
		return nil, fmt.Errorf("file: %s: absolute reference outside the root: %w", ref, plugin.ErrDenied)
	}
	full := filepath.Join(p.base, ref)
	rel, ok := within(p.root, full)
	if !ok {
		return nil, fmt.Errorf("file: %s: escapes the root: %w", ref, plugin.ErrDenied)
	}
	real, err := filepath.EvalSymlinks(full)
	if err != nil {
		return nil, classify(ref, err)
	}
	if _, ok := within(p.root, real); !ok {
		return nil, fmt.Errorf("file: %s: symlink leads outside the root: %w", ref, plugin.ErrDenied)
	}
	r, err := os.OpenRoot(p.root)
	if err != nil {
		return nil, fmt.Errorf("file: open root: %w", plugin.ErrUnavailable)
	}
	defer r.Close()
	st, err := r.Stat(rel)
	if err != nil {
		return nil, classifyRoot(ref, err)
	}
	if !st.Mode().IsRegular() {
		return nil, fmt.Errorf("file: %s: %w", ref, ErrNotRegular)
	}
	f, err := r.Open(rel)
	if err != nil {
		return nil, classifyRoot(ref, err)
	}
	defer f.Close()
	return p.readAll(ref, f)
}

// readAll reads at most maxSize bytes and fails rather than truncate. The open
// file is checked again, so a file swapped for a FIFO after Stat is refused.
func (p *Plugin) readAll(ref string, f *os.File) ([]byte, error) {
	st, err := f.Stat()
	if err != nil {
		return nil, classify(ref, err)
	}
	if !st.Mode().IsRegular() {
		return nil, fmt.Errorf("file: %s: %w", ref, ErrNotRegular)
	}
	data, err := io.ReadAll(io.LimitReader(f, p.maxSize+1))
	if err != nil {
		return nil, classify(ref, err)
	}
	if int64(len(data)) > p.maxSize {
		return nil, fmt.Errorf("file: %s: more than %d bytes: %w", ref, p.maxSize, ErrTooLarge)
	}
	return data, nil
}

// within reports whether path lies inside root (or is root) and returns its
// path relative to root.
func within(root, path string) (string, bool) {
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", false
	}
	return rel, true
}

// classify maps an OS error to the plugin's named errors. The ref, not the
// absolute path, names the file, so errors read like the document.
func classify(ref string, err error) error {
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return fmt.Errorf("file: %s: %w", ref, plugin.ErrNotFound)
	case errors.Is(err, fs.ErrPermission):
		return fmt.Errorf("file: %s: permission denied: %w", ref, plugin.ErrDenied)
	}
	return fmt.Errorf("file: %s: %v: %w", ref, err, plugin.ErrUnavailable)
}

// classifyRoot is classify for errors from os.Root, where anything other than
// not-found or permission is an escape attempt (os.Root's escape error is not
// exported), so it is refused.
func classifyRoot(ref string, err error) error {
	if errors.Is(err, fs.ErrNotExist) || errors.Is(err, fs.ErrPermission) {
		return classify(ref, err)
	}
	return fmt.Errorf("file: %s: %v: %w", ref, err, plugin.ErrDenied)
}

// Compile-time check: the ability a format detects by type assertion.
var _ plugin.Resolver = (*Plugin)(nil)
