package vault

import (
	"fmt"
	"strconv"
	"strings"
)

// Ref is a parsed vault reference: the text after "vault:" in
// ${vault:secret/data/app#password}.
//
// Grammar:
//
//	ref     = path [ "?version=" digits ] [ "#" field ]
//	path    = segment *( "/" segment )   ; a leading "/" is ignored
//	segment = 1*( any byte except "/", "?", "#" )
//
// The path is the Vault HTTP API path below /v1/, written exactly as the API
// wants it: a KV v2 secret includes its "data/" segment
// (secret/data/app), a KV v1 secret does not (kv/app). Which engine answered
// is detected from the response shape, so both work without configuration.
// version (KV v2 only) selects an older version. field names one key of the
// secret; it may be left out when the secret holds exactly one key. A field
// may contain any byte, including "#" and "?".
//
// "." and ".." segments and empty segments are refused, so a reference can
// never reach a path other than the one it spells.
type Ref struct {
	// Path is the API path without the leading "/v1/", e.g. "secret/data/app".
	Path string
	// Version is the KV v2 version to read; 0 means the latest.
	Version int
	// Field is the key inside the secret; "" means "the only key".
	Field string
	// HasField records that a "#" was written, so "secret/app#" (an empty
	// field name) round-trips and is refused as a field rather than ignored.
	HasField bool
}

// versionParam is the only query parameter a reference may carry.
const versionParam = "?version="

// ParseRef parses a reference. The error wraps ErrInvalidRef and never
// contains anything but the reference itself, which is not a secret.
func ParseRef(s string) (Ref, error) {
	var r Ref
	rest := s
	if i := strings.IndexByte(rest, '#'); i >= 0 {
		r.Field, r.HasField = rest[i+1:], true
		rest = rest[:i]
		if r.Field == "" {
			return Ref{}, fmt.Errorf("%w %q: empty field after '#'", ErrInvalidRef, s)
		}
	}
	if i := strings.IndexByte(rest, '?'); i >= 0 {
		q := rest[i:]
		rest = rest[:i]
		digits, ok := strings.CutPrefix(q, versionParam)
		if !ok {
			return Ref{}, fmt.Errorf("%w %q: only %q is allowed after the path", ErrInvalidRef, s, versionParam+"N")
		}
		v, err := strconv.Atoi(digits)
		if err != nil || v <= 0 || strconv.Itoa(v) != digits {
			return Ref{}, fmt.Errorf("%w %q: version must be a positive integer", ErrInvalidRef, s)
		}
		r.Version = v
	}
	rest = strings.TrimPrefix(rest, "/")
	if rest == "" {
		return Ref{}, fmt.Errorf("%w %q: empty path", ErrInvalidRef, s)
	}
	for _, seg := range strings.Split(rest, "/") {
		switch seg {
		case "":
			return Ref{}, fmt.Errorf("%w %q: empty path segment", ErrInvalidRef, s)
		case ".", "..":
			return Ref{}, fmt.Errorf("%w %q: %q segments are not allowed", ErrInvalidRef, s, seg)
		}
	}
	r.Path = rest
	return r, nil
}

// String renders the reference in canonical form; ParseRef(r.String())
// returns r for every r ParseRef produced.
func (r Ref) String() string {
	var b strings.Builder
	b.WriteString(r.Path)
	if r.Version > 0 {
		b.WriteString(versionParam)
		b.WriteString(strconv.Itoa(r.Version))
	}
	if r.HasField {
		b.WriteByte('#')
		b.WriteString(r.Field)
	}
	return b.String()
}

// cacheKey identifies one secret read: the field is not part of it, so two
// fields of one secret cost one request.
func (r Ref) cacheKey() string {
	if r.Version > 0 {
		return r.Path + versionParam + strconv.Itoa(r.Version)
	}
	return r.Path
}
