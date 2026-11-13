package jsonc

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/ubgo/textdata/path"
	"github.com/ubgo/textdata/pos"
)

// located is the result of resolving a path.
type located struct {
	n      *node // the value
	parent *node // its container; nil for the root
	it     *item // the item holding it; nil for the root
	index  int   // its index in parent.items
}

// parsePath parses the dotted form or, with a leading '/', a JSON Pointer.
func parsePath(p string) (path.Path, error) {
	pp, err := path.ParseAny(p)
	if err != nil {
		return nil, fmt.Errorf("jsonc: %w", err)
	}
	return pp, nil
}

// find resolves p. A missing step returns ErrNotFound naming the first
// missing part.
func (d *Doc) find(p path.Path) (located, error) {
	if d.root == nil {
		return located{}, fmt.Errorf("%w: %q (the document is empty)", ErrNotFound, p.String())
	}
	cur := located{n: d.root, index: -1}
	for i, seg := range p {
		next, ok := child(cur.n, seg)
		if !ok {
			return cur, fmt.Errorf("%w: %q", ErrNotFound, p[:i+1].String())
		}
		cur = located{n: cur.n.items[next].value, parent: cur.n, it: cur.n.items[next], index: next}
	}
	return cur, nil
}

// child finds seg in n: the LAST member with that key (duplicates: last
// wins), or the element at that index.
func child(n *node, seg path.Segment) (int, bool) {
	switch n.kind {
	case KindObject:
		if !seg.MatchesKey() {
			return 0, false
		}
		for i := len(n.items) - 1; i >= 0; i-- {
			if n.items[i].key == seg.Key {
				return i, true
			}
		}
	case KindArray:
		if seg.MatchesIndex() && seg.Index >= 0 && seg.Index < len(n.items) {
			return seg.Index, true
		}
	}
	return 0, false
}

// lookup parses and resolves a path string on a valid document.
func (d *Doc) lookup(p string) (located, path.Path, error) {
	if err := d.valid(); err != nil {
		return located{}, nil, err
	}
	pp, err := parsePath(p)
	if err != nil {
		return located{}, nil, err
	}
	loc, err := d.find(pp)
	return loc, pp, err
}

// Has reports whether a value exists at path.
func (d *Doc) Has(p string) bool {
	_, _, err := d.lookup(p)
	return err == nil
}

// Kind returns the kind of the value at path.
func (d *Doc) Kind(p string) (Kind, error) {
	loc, _, err := d.lookup(p)
	if err != nil {
		return KindInvalid, err
	}
	return loc.n.kind, nil
}

// Get decodes the value at path the way encoding/json decodes into an
// interface: map[string]any, []any, string, float64, bool or nil.
func (d *Doc) Get(p string) (any, error) {
	loc, _, err := d.lookup(p)
	if err != nil {
		return nil, err
	}
	var v any
	if err := json.Unmarshal(d.plain(loc.n.start, loc.n.end), &v); err != nil {
		return nil, fmt.Errorf("jsonc: decode %q: %w", p, err)
	}
	return v, nil
}

// GetString returns the string at path.
func (d *Doc) GetString(p string) (string, error) {
	loc, _, err := d.typed(p, KindString)
	if err != nil {
		return "", err
	}
	return loc.n.str, nil
}

// GetBool returns the bool at path.
func (d *Doc) GetBool(p string) (bool, error) {
	loc, _, err := d.typed(p, KindBool)
	if err != nil {
		return false, err
	}
	return d.src[loc.n.start] == 't', nil
}

// GetInt returns the number at path as an int64, exactly: a number with a
// fraction or outside int64 is an ErrType error, never a rounded value.
func (d *Doc) GetInt(p string) (int64, error) {
	loc, _, err := d.typed(p, KindNumber)
	if err != nil {
		return 0, err
	}
	lit := string(d.src[loc.n.start:loc.n.end])
	if n, err := strconv.ParseInt(lit, 10, 64); err == nil {
		return n, nil
	}
	f, err := strconv.ParseFloat(lit, 64)
	if err == nil && f == math.Trunc(f) && f >= math.MinInt64 && f < math.MaxInt64 {
		return int64(f), nil
	}
	return 0, d.typeErr(loc, p, "an integer")
}

// GetFloat returns the number at path as a float64.
func (d *Doc) GetFloat(p string) (float64, error) {
	loc, _, err := d.typed(p, KindNumber)
	if err != nil {
		return 0, err
	}
	f, err := strconv.ParseFloat(string(d.src[loc.n.start:loc.n.end]), 64)
	if err != nil {
		return 0, d.typeErr(loc, p, "a float64")
	}
	return f, nil
}

// Keys returns the member names of the object at path, in document order,
// each once (a duplicate key is listed where it first appears).
func (d *Doc) Keys(p string) ([]string, error) {
	loc, _, err := d.typed(p, KindObject)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var out []string
	for _, it := range loc.n.items {
		if !seen[it.key] {
			seen[it.key] = true
			out = append(out, it.key)
		}
	}
	return out, nil
}

// Len returns the number of elements of the array, or of distinct keys of
// the object, at path.
func (d *Doc) Len(p string) (int, error) {
	loc, _, err := d.lookup(p)
	if err != nil {
		return 0, err
	}
	switch loc.n.kind {
	case KindArray:
		return len(loc.n.items), nil
	case KindObject:
		keys, _ := d.Keys(p)
		return len(keys), nil
	}
	return 0, d.typeErr(loc, p, "an array or object")
}

// Comment returns the comment attached to the value at path: its leading
// comment lines joined by "\n", or else its trailing comment; "" when none.
func (d *Doc) Comment(p string) (string, error) {
	loc, _, err := d.lookup(p)
	if err != nil {
		return "", err
	}
	if loc.it == nil {
		return "", nil
	}
	if len(loc.it.leading) > 0 {
		lines := make([]string, len(loc.it.leading))
		for i, c := range loc.it.leading {
			lines[i] = c.text(d.src)
		}
		return strings.Join(lines, "\n"), nil
	}
	if loc.it.trailing != nil {
		return loc.it.trailing.text(d.src), nil
	}
	return "", nil
}

// JSON returns plain JSON: the document without comments, trailing commas
// and BOM, every other byte (whitespace included) as written. A document
// with syntax errors yields ErrInvalid; an empty document yields "null".
func (d *Doc) JSON() ([]byte, error) {
	if err := d.valid(); err != nil {
		return nil, err
	}
	if d.root == nil {
		return []byte("null"), nil
	}
	return d.plain(0, len(d.src)), nil
}

// Decode decodes the document into v with encoding/json.
func (d *Doc) Decode(v any) error {
	b, err := d.JSON()
	if err != nil {
		return err
	}
	if err := json.Unmarshal(b, v); err != nil {
		return fmt.Errorf("jsonc: decode: %w", err)
	}
	return nil
}

// plain returns src[from:to] without comments, trailing commas and BOM.
func (d *Doc) plain(from, to int) []byte {
	var cut [][2]int
	for _, c := range d.comments {
		if c.start < from || c.end > to {
			continue
		}
		start, end := c.start, c.end
		switch {
		case ownLineStart(d.src, start) && ownLineEnd(d.src, end):
			// A comment alone on its lines goes with its lines.
			start, end = lineStart(d.src, start), lineEnd(d.src, end)
			if end < len(d.src) {
				end++
			}
			start, end = max(start, from), min(end, to)
		case ownLineEnd(d.src, end):
			// A comment ending a line takes the blanks before it.
			for start > from && (d.src[start-1] == ' ' || d.src[start-1] == '\t') {
				start--
			}
		}
		cut = append(cut, [2]int{start, end})
	}
	if from == 0 && bytes.HasPrefix(d.src, []byte(bom)) {
		cut = append(cut, [2]int{0, len(bom)})
	}
	walk(d.root, func(n *node) {
		if last := lastItem(n); last != nil && last.comma >= from && last.comma < to {
			cut = append(cut, [2]int{last.comma, last.comma + 1})
		}
	})
	sort.Slice(cut, func(i, j int) bool { return cut[i][0] < cut[j][0] })
	var b bytes.Buffer
	at := from
	for _, c := range cut {
		if c[0] < at { // overlapping cuts (adjacent comment lines)
			c[0] = at
		}
		if c[1] <= at {
			continue
		}
		b.Write(d.src[at:c[0]])
		at = c[1]
	}
	b.Write(d.src[at:to])
	return b.Bytes()
}

// walk visits n and every container below it.
func walk(n *node, fn func(*node)) {
	if n == nil || (n.kind != KindObject && n.kind != KindArray) {
		return
	}
	fn(n)
	for _, it := range n.items {
		walk(it.value, fn)
	}
}

// typed resolves p and checks the kind.
func (d *Doc) typed(p string, want Kind) (located, path.Path, error) {
	loc, pp, err := d.lookup(p)
	if err != nil {
		return loc, pp, err
	}
	if loc.n.kind != want {
		return loc, pp, d.typeErr(loc, p, article(want))
	}
	return loc, pp, nil
}

func (d *Doc) typeErr(loc located, p, want string) error {
	return &typeError{pe: &pos.Error{
		File: d.opts.file,
		Pos:  d.at(loc.n.start),
		Msg:  fmt.Sprintf("%q is %s, not %s", p, article(loc.n.kind), want),
	}}
}

// typeError is a positioned error that also matches ErrType, so callers can
// use errors.Is(err, ErrType) and errors.As(err, **pos.Error) alike.
type typeError struct{ pe *pos.Error }

func (e *typeError) Error() string { return e.pe.Error() }

// Unwrap exposes the position and the ErrType sentinel.
func (e *typeError) Unwrap() []error { return []error{e.pe, ErrType} }

func article(k Kind) string {
	switch k {
	case KindObject, KindArray:
		return "an " + k.String()
	default:
		return "a " + k.String()
	}
}

// ValuePaths implements plugin.Document: the canonical path of every scalar
// and every empty container, in document order.
func (d *Doc) ValuePaths() []string {
	var out []string
	var visit func(n *node, p path.Path)
	visit = func(n *node, p path.Path) {
		if (n.kind != KindObject && n.kind != KindArray) || len(n.items) == 0 {
			out = append(out, p.String())
			return
		}
		for i, it := range n.items {
			var seg path.Segment
			if n.kind == KindArray {
				seg = path.Index(i)
			} else {
				seg = path.Key(it.key)
			}
			visit(it.value, p.Append(seg))
		}
	}
	if d.root != nil && len(d.errs) == 0 {
		visit(d.root, nil)
	}
	return out
}

// ValueAt implements plugin.Document: a string's contents, or the literal
// source text of any other value (comments included for containers).
func (d *Doc) ValueAt(p string) (string, bool) {
	loc, _, err := d.lookup(p)
	if err != nil {
		return "", false
	}
	if loc.n.kind == KindString {
		return loc.n.str, true
	}
	return string(d.src[loc.n.start:loc.n.end]), true
}

// FormatName implements plugin.Document.
func (d *Doc) FormatName() string { return FormatName }

// Location implements plugin.Document: the file path, or "".
func (d *Doc) Location() string { return d.path }

// Bytes implements plugin.Document: exactly what Save would write.
func (d *Doc) Bytes() []byte { return d.Render() }
