// Package jsonc reads, edits and writes JSON with comments (JSONC) without
// disturbing a byte it was not asked to change.
//
// Guarantees, pinned by fuzz and property tests:
//
//   - Parse(x).Render() == x, byte for byte, for every input, valid or not.
//   - An edit changes only the bytes of what it touches: comments, blank
//     lines, key order, indentation and line endings elsewhere survive.
//   - Reading never expands anything or touches the process environment;
//     ${VAR} expansion happens only through Expanded / GetExpanded, and a
//     command runs only when the caller turned command substitution on.
//   - Errors carry file:line:col.
//
// The format is JSON (RFC 8259) plus // and /* */ comments, trailing commas
// and an optional UTF-8 BOM. Unquoted keys, single quotes, hex numbers and
// NaN/Infinity are JSON5 and are reported as errors. Strict() turns comments
// and trailing commas into errors too: plain JSON, accepting exactly what
// encoding/json accepts.
//
// Design: a Doc owns its source bytes and a tree of spans over them. Every edit
// is a splice of the source followed by a re-parse, so "only the touched bytes
// change" holds by construction and Render is the source itself.
//
// Paths address values with the dotted form `a."b.c"[2]` or a JSON Pointer
// `/a/b.c/2` (package github.com/ubgo/textdata/path). On duplicate keys the
// last one wins, for reading and for editing, like encoding/json.
package jsonc

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"sync/atomic"

	"github.com/ubgo/textdata/plugin"
	"github.com/ubgo/textdata/pos"
	"github.com/ubgo/textdata/shell"
)

// FormatName is what Doc.FormatName reports to plugins.
const FormatName = "jsonc"

// defaultFileMode is the mode of a file that did not exist before Save:
// owner-only, because config files often hold secrets.
const defaultFileMode fs.FileMode = 0o600

// Errors returned (wrapped) by the edit and read API.
var (
	// ErrInvalid: the document has syntax errors, so it cannot be edited or
	// decoded. Render still returns it byte for byte.
	ErrInvalid = errors.New("jsonc: document has syntax errors")
	// ErrNotFound: nothing exists at the path.
	ErrNotFound = errors.New("jsonc: path not found")
	// ErrType: the value at the path has a different kind than the call needs.
	ErrType = errors.New("jsonc: wrong value type")
	// ErrExists: Restore found the key already live.
	ErrExists = errors.New("jsonc: key already exists")
	// ErrStrict: the operation needs comments, which Strict() forbids.
	ErrStrict = errors.New("jsonc: not allowed in strict mode")
	// ErrLayout: the edit needs the value on lines of its own (Disable).
	ErrLayout = errors.New("jsonc: value does not occupy its own lines")
)

// Doc is a parsed JSONC document. The zero Doc is not usable; use Parse,
// Open or ParseReader.
type Doc struct {
	src      []byte
	root     *node
	comments []comment
	errs     pos.ErrorList
	opts     options
	// idx maps offsets to line:col; built lazily on the first position
	// lookup and dropped on every reparse, so a lookup costs O(log n) after
	// the first instead of a full rescan. Atomic because position lookups
	// are reads: several goroutines may build it at once (each builds the
	// same index) without racing.
	idx atomic.Pointer[pos.Index]

	// path is the file the document was read from and Save writes to.
	path    string
	existed bool
	mode    fs.FileMode
}

// options is the resolved configuration of a Doc.
type options struct {
	strict bool
	file   string // name used in error messages, when not from Open
	expand bool   // Read expands before decoding
	shell  bool   // $(command) substitution allowed
	// strictExpand makes unresolved references errors (WithStrictExpand).
	strictExpand bool
	// run is the installed CommandRunner, or nil for the default shell
	// runner; read it through command, never directly.
	run func(string) (string, error)
	// commandCtx bounds the default shell runner (WithCommandContext); nil
	// is context.Background(), today's unbounded behaviour.
	commandCtx context.Context
	reg        plugin.Registry

	lookupers    []plugin.Lookuper
	resolvers    []plugin.Resolver
	transformers []plugin.ValueTransformer
	guards       []plugin.SaveGuard
	observers    []plugin.ExpandObserver
	validators   []plugin.Validator
}

// Option configures parsing, expansion and saving.
type Option func(*options)

// Strict parses plain JSON: comments and trailing commas (and a BOM) are
// errors. Accepts exactly what encoding/json.Valid accepts.
func Strict() Option { return func(o *options) { o.strict = true } }

// WithFileName sets the name errors report for a document parsed from bytes.
func WithFileName(name string) Option { return func(o *options) { o.file = name } }

// WithExpand makes Read expand ${...} references before decoding. Doc
// methods are unaffected: Get and JSON always return raw values, Expanded and
// GetExpanded always expand.
func WithExpand() Option { return func(o *options) { o.expand = true } }

// WithStrictExpand makes expansion (Expanded, GetExpanded, and Read with
// WithExpand) refuse every reference that would otherwise read as "" or stay
// as written: an unset name (*expand.UnsetError; ${VAR:-d} and ${VAR-d} still
// apply their default and ${VAR:?msg} keeps its *expand.RequiredError), a
// ${scheme:ref} no installed Resolver claims (*expand.UnknownSchemeError), and
// a $(command) while command substitution is off (*expand.CommandOffError).
// The error is a *pos.Error at the string's file:line:col wrapping the typed
// error, which names the variable and its byte offset inside the decoded
// string, never a value; every one matches expand.ErrStrict. Off by default.
//
// Named WithStrictExpand, not Strict, because Strict already means "parse as
// plain JSON".
// ds:def id=withstrictexpand-tnccxzjq owner=@maintainers stability=stable
func WithStrictExpand() Option { return func(o *options) { o.strictExpand = true } }

// WithLookup supplies values for ${NAME} references, e.g. os.LookupEnv. The
// package never reads the environment on its own.
func WithLookup(fn func(name string) (string, bool)) Option {
	return func(o *options) {
		if fn != nil {
			o.add(funcLookuper{fn: fn})
		}
	}
}

// WithCommandSubstitution enables $(command) in expanded values, run through
// shell.Run (package github.com/ubgo/textdata/shell): `sh -c` on Unix-likes;
// on Windows `pwsh` when on PATH, else `powershell.exe`. Off by default: with
// it on, expanding a file runs commands, so turn it on only for files you
// trust. Stdin is the null device, so a prompting command fails instead of
// hanging. A failing command is an error that unwraps to
// *expand.CommandError. WithCommandContext bounds how long a command may run.
func WithCommandSubstitution(enabled bool) Option {
	return func(o *options) { o.shell = enabled }
}

// ds:def id=withcommandcontext-3z9bvvrh owner=@maintainers stability=stable

// WithCommandContext bounds $(command) substitution with ctx: when ctx is
// cancelled or its deadline passes, the running command is killed and
// expansion fails with an error unwrapping to *expand.CommandError and to
// shell.ErrTimeout (a deadline) or context.Canceled. Use context.WithTimeout
// for a time limit.
//
// It applies to the default shell runner only; a CommandRunner plugin is
// called as is. It does not turn substitution on. Without it, commands are
// unbounded, as before. The context is kept for the life of the Doc (and its
// clones), so pass one that outlives the reads that should honour it.
func WithCommandContext(ctx context.Context) Option {
	return func(o *options) { o.commandCtx = ctx }
}

// command returns the $(command) runner: the installed CommandRunner, or the
// default shell runner bounded by WithCommandContext.
func (o *options) command() func(string) (string, error) {
	if o.run != nil {
		return o.run
	}
	ctx := o.commandCtx
	if ctx == nil {
		return shellRun
	}
	return func(cmd string) (string, error) { return shell.Run(ctx, cmd) }
}

// WithPlugin installs plugins; their capabilities (package
// github.com/ubgo/textdata/plugin) are detected once, here. A CommandRunner
// turns command substitution on and replaces any earlier runner.
func WithPlugin(ps ...plugin.Plugin) Option {
	return func(o *options) {
		for _, p := range ps {
			if p != nil {
				o.add(p)
			}
		}
	}
}

func (o *options) add(p plugin.Plugin) {
	o.reg.Add(p)
	if v, ok := p.(plugin.Lookuper); ok {
		o.lookupers = append(o.lookupers, v)
	}
	if v, ok := p.(plugin.Resolver); ok {
		o.resolvers = append(o.resolvers, v)
	}
	if v, ok := p.(plugin.ValueTransformer); ok {
		o.transformers = append(o.transformers, v)
	}
	if v, ok := p.(plugin.SaveGuard); ok {
		o.guards = append(o.guards, v)
	}
	if v, ok := p.(plugin.ExpandObserver); ok {
		o.observers = append(o.observers, v)
	}
	if v, ok := p.(plugin.Validator); ok {
		o.validators = append(o.validators, v)
	}
	if v, ok := p.(plugin.CommandRunner); ok {
		o.shell = true
		o.run = v.RunCommand
	}
}

func newOptions(opts []Option) options {
	var o options
	for _, opt := range opts {
		if opt != nil {
			opt(&o)
		}
	}
	return o
}

// Parse parses src. It always returns a Doc: on syntax errors the Doc holds
// the tree built up to the first error and renders src unchanged, and the
// error is a pos.ErrorList whose entries carry file:line:col. src is copied.
func Parse(src []byte, opts ...Option) (*Doc, error) {
	o := newOptions(opts)
	d := &Doc{src: append([]byte(nil), src...), opts: o, mode: defaultFileMode}
	d.reparse()
	return d, d.errs.Err()
}

// ParseReader reads r to the end and parses it.
func ParseReader(r io.Reader, opts ...Option) (*Doc, error) {
	b, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("jsonc: read: %w", err)
	}
	return Parse(b, opts...)
}

// Open reads and parses the file at path. A missing file is deliberately not
// an error (as with dotenv.Open): it yields an empty document that Save
// creates, so `Open → Set → Save` works on a first run. Callers that need the
// file to be there check Existed, or use Read. Errors name the file.
func Open(path string, opts ...Option) (*Doc, error) {
	o := newOptions(opts)
	if o.file == "" {
		o.file = path
	}
	d := &Doc{opts: o, path: path, mode: defaultFileMode}
	b, err := os.ReadFile(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
	case err != nil:
		return nil, fmt.Errorf("jsonc: %w", err)
	default:
		d.existed = true
		d.src = b
		if info, err := os.Stat(path); err == nil {
			d.mode = info.Mode().Perm()
		}
	}
	d.reparse()
	return d, d.errs.Err()
}

// Read opens path and decodes it into a map. With an Includer plugin
// installed, $extends bases are merged in first; with WithExpand, references
// are expanded next.
//
// A missing file is an error here, unlike Open: Read is for consuming a file
// that should exist. The error names the path and wraps fs.ErrNotExist.
func Read(path string, opts ...Option) (map[string]any, error) {
	d, err := Open(path, opts...)
	if err != nil {
		return nil, err
	}
	if !d.Existed() {
		return nil, fmt.Errorf("jsonc: read %s: %w", path, fs.ErrNotExist)
	}
	if d, err = d.extendIfInstalled(); err != nil {
		return nil, err
	}
	if d.opts.expand {
		if d, err = d.Expanded(); err != nil {
			return nil, err
		}
	}
	var m map[string]any
	if err := d.Decode(&m); err != nil {
		return nil, err
	}
	return m, nil
}

// reparse rebuilds the tree from d.src.
func (d *Doc) reparse() {
	d.root, d.comments, d.errs = parse(d.src, d.opts.strict, d.opts.file)
	d.idx.Store(nil)
}

// FileName is the name the document reports in its errors: the one given
// with WithFileName, else the path it was opened from, else "".
func (d *Doc) FileName() string { return d.opts.file }

// IsStrict reports whether the document was parsed with Strict(), so tools
// that rewrite it (formatters, generators) never add comments or trailing
// commas plain JSON forbids.
func (d *Doc) IsStrict() bool { return d.opts.strict }

// ExpandRequested reports whether the document was opened WithExpand: a
// caller that decodes on its own (into a struct, a config source) uses it to
// expand exactly when the one-call Read would, then calls Expanded and
// Decode.
func (d *Doc) ExpandRequested() bool { return d.opts.expand }

// Err returns the document's syntax errors (a pos.ErrorList), or nil.
func (d *Doc) Err() error { return d.errs.Err() }

// Path is the file the document was opened from and Save writes to.
func (d *Doc) Path() string { return d.path }

// SetPath changes where Save writes.
func (d *Doc) SetPath(path string) { d.path = path }

// Existed reports whether Open found the file.
func (d *Doc) Existed() bool { return d.existed }

// Mode is the permission Save writes with: the file's own when it existed,
// 0600 otherwise.
func (d *Doc) Mode() fs.FileMode { return d.mode }

// Render returns the document's bytes: exactly the input for an unedited
// document. The slice is a copy.
func (d *Doc) Render() []byte { return append([]byte(nil), d.src...) }

// Clone returns an independent copy: its own source bytes, tree, errors,
// path, mode and options, so editing, re-pathing or saving the clone never
// changes the original. Installed plugins are shared (they are the same
// values the caller passed in), but the clone's plugin lists are its own.
func (d *Doc) Clone() *Doc {
	c := &Doc{
		src:     append([]byte(nil), d.src...),
		opts:    d.opts.clone(),
		path:    d.path,
		existed: d.existed,
		mode:    d.mode,
	}
	c.reparse()
	return c
}

// clone copies the option slices with their capacity clipped, so an append
// on one copy can never write into the other's backing array.
func (o options) clone() options {
	o.lookupers = clip(o.lookupers)
	o.resolvers = clip(o.resolvers)
	o.transformers = clip(o.transformers)
	o.guards = clip(o.guards)
	o.observers = clip(o.observers)
	o.validators = clip(o.validators)
	return o
}

func clip[S ~[]E, E any](s S) S {
	if s == nil {
		return nil
	}
	return append(S(nil), s...)
}

// Plugins reports every installed plugin and the capabilities detected.
func (d *Doc) Plugins() []plugin.Info { return d.opts.reg.Info() }

// valid returns ErrInvalid (with the first syntax error) when the document
// cannot be edited or decoded.
func (d *Doc) valid() error {
	if len(d.errs) > 0 {
		return fmt.Errorf("%w: %v", ErrInvalid, d.errs[0])
	}
	return nil
}

// at returns the position of an offset, for error messages.
func (d *Doc) at(off int) pos.Pos {
	ix := d.idx.Load()
	if ix == nil {
		ix = pos.NewIndex(d.src)
		d.idx.Store(ix)
	}
	return ix.Pos(off)
}

// funcLookuper adapts a function to plugin.Lookuper.
type funcLookuper struct {
	fn func(string) (string, bool)
}

func (funcLookuper) Name() string { return "lookup" }

func (f funcLookuper) Lookup(name string) (string, bool, error) {
	v, ok := f.fn(name)
	return v, ok, nil
}
