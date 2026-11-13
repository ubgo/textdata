// Package path parses the paths tree formats use to address a value:
// the dotted form a."b.c"[2] and RFC 6901 JSON Pointer /a/b.c/2.
//
// Flat formats such as dotenv address values by key and do not use it.
//
// Dotted grammar:
//
//	path    = [ segment *( "." segment | index ) ]
//	segment = name | quoted | index
//	name    = 1*( any byte except '.', '[', '"' )
//	quoted  = '"' *( char | '\"' | '\\' ) '"'
//	index   = "[" 1*digit "]"
//
// The empty string is the root. A leading index addresses the root array
// ("[0].name").
package path

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// Kind says what a Segment addresses.
type Kind int

const (
	// KindKey addresses an object member by name.
	KindKey Kind = iota
	// KindIndex addresses an array item by position.
	KindIndex
	// KindEither comes from a JSON Pointer token made only of digits, which
	// RFC 6901 resolves by the container: an index in an array, a member name
	// in an object.
	KindEither
)

// Segment is one step of a path.
type Segment struct {
	Kind Kind
	// Key is the member name (KindKey, KindEither).
	Key string
	// Index is the array position (KindIndex, KindEither).
	Index int
}

// Key returns a member-name segment.
func Key(name string) Segment { return Segment{Kind: KindKey, Key: name} }

// Index returns an array-position segment.
func Index(i int) Segment { return Segment{Kind: KindIndex, Index: i} }

// MatchesKey reports whether s can address the member name.
func (s Segment) MatchesKey() bool { return s.Kind != KindIndex }

// MatchesIndex reports whether s can address an array item.
func (s Segment) MatchesIndex() bool { return s.Kind != KindKey }

// Path is a sequence of segments from the root. The nil Path is the root.
type Path []Segment

// ErrSyntax is wrapped by every parse error.
var ErrSyntax = errors.New("path: invalid syntax")

// Parse parses the dotted form.
func Parse(s string) (Path, error) {
	var p Path
	i := 0
	expectSep := false // after a segment, only '.', '[' or end may follow
	for i < len(s) {
		c := s[i]
		switch {
		case c == '[':
			end := strings.IndexByte(s[i:], ']')
			if end < 0 {
				return nil, syntaxErr(s, i, "unclosed '['")
			}
			n, err := parseIndex(s[i+1 : i+end])
			if err != nil {
				return nil, syntaxErr(s, i, err.Error())
			}
			p = append(p, Index(n))
			i += end + 1
			expectSep = true
		case expectSep && c == '.':
			i++
			if i == len(s) {
				return nil, syntaxErr(s, i, "path ends with '.'")
			}
			if s[i] == '.' || s[i] == '[' {
				return nil, syntaxErr(s, i, "empty segment")
			}
			expectSep = false
		case expectSep:
			return nil, syntaxErr(s, i, "expected '.' or '['")
		case c == '"':
			name, n, err := unquote(s[i:])
			if err != nil {
				return nil, syntaxErr(s, i, err.Error())
			}
			p = append(p, Key(name))
			i += n
			expectSep = true
		case c == '.':
			return nil, syntaxErr(s, i, "empty segment")
		default:
			j := i
			for j < len(s) && s[j] != '.' && s[j] != '[' && s[j] != '"' {
				j++
			}
			if j < len(s) && s[j] == '"' {
				return nil, syntaxErr(s, j, "quote inside a bare name")
			}
			p = append(p, Key(s[i:j]))
			i = j
			expectSep = true
		}
	}
	return p, nil
}

// MustParse is Parse that panics on error, for paths written in source code.
func MustParse(s string) Path {
	p, err := Parse(s)
	if err != nil {
		panic(err)
	}
	return p
}

// ParsePointer parses an RFC 6901 JSON Pointer. "" is the root; every other
// pointer starts with '/'. "~1" decodes to '/', "~0" to '~'. Tokens made only
// of digits (without a leading zero, or "0") become KindEither segments.
func ParsePointer(s string) (Path, error) {
	if s == "" {
		return nil, nil
	}
	if s[0] != '/' {
		return nil, syntaxErr(s, 0, "JSON Pointer must start with '/'")
	}
	var p Path
	for _, tok := range strings.Split(s[1:], "/") {
		key, err := unescapePointer(tok)
		if err != nil {
			return nil, syntaxErr(s, 0, err.Error())
		}
		if n, err := parseIndex(key); err == nil {
			p = append(p, Segment{Kind: KindEither, Key: key, Index: n})
			continue
		}
		p = append(p, Key(key))
	}
	return p, nil
}

// ParseAny parses a JSON Pointer when s starts with '/', and the dotted form
// otherwise. The empty string is the root in both.
func ParseAny(s string) (Path, error) {
	if strings.HasPrefix(s, "/") {
		return ParsePointer(s)
	}
	return Parse(s)
}

// String renders the canonical dotted form. Names that are empty or contain
// '.', '[', ']', '"' or '\' are quoted, and so is a first name starting with
// '/', so ParseAny reads it back as a dotted path rather than a JSON Pointer;
// Parse(p.String()) and ParseAny(p.String()) equal p (a KindEither segment
// renders as its key).
func (p Path) String() string {
	var b strings.Builder
	for i, s := range p {
		if s.Kind == KindIndex {
			b.WriteByte('[')
			b.WriteString(strconv.Itoa(s.Index))
			b.WriteByte(']')
			continue
		}
		if i > 0 {
			b.WriteByte('.')
		}
		if needsQuote(s.Key) || (i == 0 && strings.HasPrefix(s.Key, "/")) {
			b.WriteString(quote(s.Key))
		} else {
			b.WriteString(s.Key)
		}
	}
	return b.String()
}

// Pointer renders the RFC 6901 form.
func (p Path) Pointer() string {
	var b strings.Builder
	for _, s := range p {
		b.WriteByte('/')
		if s.Kind == KindIndex {
			b.WriteString(strconv.Itoa(s.Index))
			continue
		}
		b.WriteString(strings.NewReplacer("~", "~0", "/", "~1").Replace(s.Key))
	}
	return b.String()
}

// Parent returns p without its last segment, and that segment. The root has
// no parent: ok is false.
func (p Path) Parent() (parent Path, last Segment, ok bool) {
	if len(p) == 0 {
		return nil, Segment{}, false
	}
	return p[: len(p)-1 : len(p)-1], p[len(p)-1], true
}

// Append returns a new path with segs added; p is not modified.
func (p Path) Append(segs ...Segment) Path {
	out := make(Path, 0, len(p)+len(segs))
	out = append(out, p...)
	return append(out, segs...)
}

func needsQuote(name string) bool {
	return name == "" || strings.ContainsAny(name, `.[]"\`)
}

func quote(name string) string {
	var b strings.Builder
	b.WriteByte('"')
	for i := 0; i < len(name); i++ {
		if name[i] == '"' || name[i] == '\\' {
			b.WriteByte('\\')
		}
		b.WriteByte(name[i])
	}
	b.WriteByte('"')
	return b.String()
}

// unquote reads a quoted segment at the start of s and returns the name and
// how many bytes it used.
func unquote(s string) (string, int, error) {
	var b strings.Builder
	for i := 1; i < len(s); i++ {
		switch s[i] {
		case '\\':
			if i+1 >= len(s) || (s[i+1] != '"' && s[i+1] != '\\') {
				return "", 0, errors.New(`only \" and \\ may be escaped`)
			}
			b.WriteByte(s[i+1])
			i++
		case '"':
			return b.String(), i + 1, nil
		default:
			b.WriteByte(s[i])
		}
	}
	return "", 0, errors.New("unclosed quote")
}

func parseIndex(s string) (int, error) {
	if s == "" {
		return 0, errors.New("empty index")
	}
	if len(s) > 1 && s[0] == '0' {
		return 0, errors.New("index has a leading zero")
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return 0, fmt.Errorf("index %q is not a number", s)
		}
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("index %q is out of range", s)
	}
	return n, nil
}

func unescapePointer(tok string) (string, error) {
	if !strings.Contains(tok, "~") {
		return tok, nil
	}
	var b strings.Builder
	for i := 0; i < len(tok); i++ {
		if tok[i] != '~' {
			b.WriteByte(tok[i])
			continue
		}
		if i+1 >= len(tok) || (tok[i+1] != '0' && tok[i+1] != '1') {
			return "", errors.New("'~' must be followed by 0 or 1")
		}
		if tok[i+1] == '0' {
			b.WriteByte('~')
		} else {
			b.WriteByte('/')
		}
		i++
	}
	return b.String(), nil
}

func syntaxErr(s string, at int, msg string) error {
	return fmt.Errorf("%w: %q at byte %d: %s", ErrSyntax, s, at, msg)
}
