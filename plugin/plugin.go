// Package plugin defines the plugin interfaces every textdata format accepts.
//
// A plugin has a Name and implements only the abilities it needs; the format
// detects them once, at install, by type assertion. One plugin may hold several
// abilities: a secret store naturally wants Lookuper for undefined references
// and Resolver for its own scheme, sharing one client and one cache.
//
// Plugins written against this package work in every format: the format hands
// them strings (names, paths, values) and a small read-only Document, never its
// own types. A format may add abilities of its own (dotenv's EntryObserver); it
// reports them through Registry.Info with extra checks.
//
// A plugin that is down is an error, never a silent "missing": a secret store
// that is unreachable must not look like a variable that is unset, or a deploy
// quietly proceeds with an empty password.
package plugin

import "errors"

// Plugin is anything a format can install.
//
// Name is required rather than derived so a failure can say which plugin
// caused it: `dotenv: plugin "vault": lookup DB_PASSWORD: connection refused`.
type Plugin interface {
	Name() string
}

// Lookuper supplies a value for a reference the file itself does not define.
//
// It runs AFTER the file's own values, so the file always wins: adding an
// environment fallback cannot silently override a value somebody wrote down.
// ok=false means "I do not have this" and the next Lookuper is tried; the first
// to answer wins. An error aborts expansion.
type Lookuper interface {
	Lookup(name string) (value string, ok bool, err error)
}

// Resolver handles references with a scheme, ${scheme:ref}, for example
// ${gopass:personal/db} or ${file:./key.pem}.
//
// Schemes lists the schemes it owns (lower case, no colon). A reference whose
// scheme no installed Resolver owns is read exactly as before resolvers
// existed, so installing one never changes how other values read. The resolved
// value is used in memory only; a format always writes the reference back.
type Resolver interface {
	Schemes() []string
	Resolve(scheme, ref string) (string, error)
}

// CommandRunner executes a $(command) substitution.
//
// A replacement, not a chain: the last installed wins. Installing one turns
// command substitution on, which is otherwise off, because with it on merely
// reading a file runs commands.
type CommandRunner interface {
	RunCommand(cmd string) (string, error)
}

// ValueTransformer rewrites a value after it has been read and expanded.
//
// path is where the value lives: the key in a flat format, the canonical
// dotted path (see package path) in a tree format. Transformers are chained,
// each seeing the previous one's output, and their output is not expanded
// again. An error aborts the read rather than yielding the untransformed value:
// a caller must never silently receive ciphertext where it expected a secret.
type ValueTransformer interface {
	TransformValue(path, value string) (string, error)
}

// Document is the read-only view of a parsed file a SaveGuard or Validator
// receives. Every format's document type implements it.
type Document interface {
	// FormatName is the format, e.g. "dotenv" or "jsonc".
	FormatName() string
	// Location is the file path, or "" for a document not tied to a file.
	Location() string
	// ValuePaths lists the address of every value, in document order: keys
	// for flat formats, canonical dotted paths for tree formats.
	ValuePaths() []string
	// ValueAt returns the raw (unexpanded) value at path as text: a string's
	// contents, or the literal text of a number, bool, null or container.
	ValueAt(path string) (string, bool)
	// Bytes is the exact content that would be written.
	Bytes() []byte
}

// SaveGuard inspects a document before it is written and may refuse.
//
// It may veto only. Rewriting content on the way to disk would break the
// promise every format rests on: a hook may change what a value reads as, never
// what gets written. Every guard runs before anything is written, and the first
// error aborts the write with the destination untouched.
type SaveGuard interface {
	GuardSave(doc Document) error
}

// ExpandObserver sees every reference resolution: path is the value being
// expanded, name the variable referenced, resolved what it became. Enough to
// answer "which variables does this file actually use". Infallible by design.
type ExpandObserver interface {
	ObserveExpand(path, name, resolved string)
}

// Validator checks a document (schema, required keys, allowed values). Formats
// run validators only when asked, never as a side effect of reading.
type Validator interface {
	Validate(doc Document) error
}

// Includer loads the target of an include reference ("$extends") from
// somewhere other than the local disk. from is the location of the including
// document; it returns the bytes and the location they came from, which
// becomes the base for the included file's own relative references.
type Includer interface {
	Include(from, ref string) (data []byte, location string, err error)
}

// Named errors a plugin returns (wrapped) so callers can branch without
// matching text.
var (
	// ErrUnavailable: the backing service is down or unreachable.
	ErrUnavailable = errors.New("plugin unavailable")
	// ErrUnsupportedScheme: a Resolver was asked for a scheme it does not own.
	ErrUnsupportedScheme = errors.New("unsupported scheme")
	// ErrDenied: the backing service refused (permissions, policy).
	ErrDenied = errors.New("access denied")
	// ErrNotFound: the referenced value does not exist in the backing store.
	// A Resolver returns it; a Lookuper returns ok=false instead.
	ErrNotFound = errors.New("not found")
)
