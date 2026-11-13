// Package diff compares two JSONC documents by meaning: which values were
// added, removed or changed, addressed by canonical path. Comments, layout,
// key order, trailing commas and number spelling (1 vs 1.0) are invisible to
// it; byte-level comparison is what diff(1) and textdiff are for. It answers
// "does the configuration differ, and where".
//
// Rules:
//
//   - Objects are compared key by key; a key on one side only is Added or
//     Removed with its whole value. Duplicate keys read last-wins, like
//     jsonc.Doc.Get.
//   - Arrays are compared by index: element i against element i, extra
//     elements are Added or Removed. An insertion near the front therefore
//     shows as a run of changes; that is the price of a diff that is cheap,
//     deterministic and always applies cleanly.
//   - A value whose kind differs (object vs array vs scalar) is one Changed
//     entry at that path, never a recursion.
//   - Numbers are compared exactly: two integer literals by their integer
//     value (no float64 rounding, so 9007199254740993 differs from
//     9007199254740992), any other pair by float64 value.
//
// The order is deterministic: depth first, object keys in byte order, array
// elements by index.
package diff

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"sort"
	"strconv"
	"strings"

	"github.com/ubgo/textdata/jsonc"
	"github.com/ubgo/textdata/path"
	"github.com/ubgo/textdata/pos"
)

// Kind says what happened at a path. Closed set; the string values are wire
// API for the CLI's --json output.
type Kind string

const (
	// Added: the path exists in b only. New holds its value.
	Added Kind = "added"
	// Removed: the path exists in a only. Old holds its value.
	Removed Kind = "removed"
	// Changed: the path exists in both with different values (or kinds).
	Changed Kind = "changed"
)

// Change is one difference between two documents.
type Change struct {
	// Kind is Added, Removed or Changed.
	Kind Kind
	// Path is the canonical dotted path (path.Path.String): "" is the root.
	Path string
	// Pointer is the same path as an RFC 6901 JSON Pointer. It is never
	// ambiguous, even for a first key starting with '/', so it is the form to
	// feed back into jsonc.Doc methods.
	Pointer string
	// Old is a's value, decoded like encoding/json with UseNumber
	// (map[string]any, []any, string, json.Number, bool, nil). Nil for Added.
	Old any
	// New is b's value, decoded the same way. Nil for Removed.
	New any
	// OldPos is where the value starts in a; the zero Pos for Added.
	OldPos pos.Pos
	// NewPos is where the value starts in b; the zero Pos for Removed.
	NewPos pos.Pos
}

// ErrNilDoc is returned when either document is nil.
var ErrNilDoc = errors.New("diff: nil document")

// Diff returns the changes that turn a into b, or an empty slice when they
// mean the same thing. A document with syntax errors is refused with its
// error (jsonc.ErrInvalid). An empty document reads as null.
func Diff(a, b *jsonc.Doc) ([]Change, error) {
	if a == nil || b == nil {
		return nil, ErrNilDoc
	}
	av, err := decode(a)
	if err != nil {
		return nil, err
	}
	bv, err := decode(b)
	if err != nil {
		return nil, err
	}
	w := walker{a: a, b: b, out: []Change{}}
	w.compare(nil, av, bv)
	return w.out, nil
}

// Equal reports whether two decoded values (as Diff decodes them) mean the
// same JSON value. Exposed so callers comparing values outside a document,
// such as a test applying changes, use the very rule Diff uses.
func Equal(x, y any) bool {
	switch xv := x.(type) {
	case map[string]any:
		yv, ok := y.(map[string]any)
		if !ok || len(xv) != len(yv) {
			return false
		}
		for k, v := range xv {
			w, ok := yv[k]
			if !ok || !Equal(v, w) {
				return false
			}
		}
		return true
	case []any:
		yv, ok := y.([]any)
		if !ok || len(xv) != len(yv) {
			return false
		}
		for i := range xv {
			if !Equal(xv[i], yv[i]) {
				return false
			}
		}
		return true
	case json.Number:
		yv, ok := y.(json.Number)
		return ok && numbersEqual(xv, yv)
	case string:
		yv, ok := y.(string)
		return ok && xv == yv
	case bool:
		yv, ok := y.(bool)
		return ok && xv == yv
	case nil:
		return y == nil
	}
	return false
}

// decode reads the whole document with UseNumber, so numbers keep their
// literal and compare exactly.
func decode(d *jsonc.Doc) (any, error) {
	src, err := d.JSON()
	if err != nil {
		return nil, err
	}
	dec := json.NewDecoder(bytes.NewReader(src))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, fmt.Errorf("diff: decode: %w", err)
	}
	return v, nil
}

type walker struct {
	a, b *jsonc.Doc
	out  []Change
}

func (w *walker) compare(p path.Path, x, y any) {
	switch xv := x.(type) {
	case map[string]any:
		if yv, ok := y.(map[string]any); ok {
			w.objects(p, xv, yv)
			return
		}
	case []any:
		if yv, ok := y.([]any); ok {
			w.arrays(p, xv, yv)
			return
		}
	}
	if !Equal(x, y) {
		w.emit(Changed, p, x, y)
	}
}

func (w *walker) objects(p path.Path, x, y map[string]any) {
	keys := make([]string, 0, len(x)+len(y))
	for k := range x {
		keys = append(keys, k)
	}
	for k := range y {
		if _, ok := x[k]; !ok {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	for _, k := range keys {
		xv, inX := x[k]
		yv, inY := y[k]
		cp := p.Append(path.Key(k))
		switch {
		case !inY:
			w.emit(Removed, cp, xv, nil)
		case !inX:
			w.emit(Added, cp, nil, yv)
		default:
			w.compare(cp, xv, yv)
		}
	}
}

func (w *walker) arrays(p path.Path, x, y []any) {
	for i := 0; i < max(len(x), len(y)); i++ {
		cp := p.Append(path.Index(i))
		switch {
		case i >= len(y):
			w.emit(Removed, cp, x[i], nil)
		case i >= len(x):
			w.emit(Added, cp, nil, y[i])
		default:
			w.compare(cp, x[i], y[i])
		}
	}
}

func (w *walker) emit(k Kind, p path.Path, old, new any) {
	c := Change{Kind: k, Path: p.String(), Pointer: p.Pointer(), Old: old, New: new}
	if k != Added {
		c.OldPos = position(w.a, c.Pointer)
	}
	if k != Removed {
		c.NewPos = position(w.b, c.Pointer)
	}
	w.out = append(w.out, c)
}

// position is the value's start, or the zero Pos when it cannot be found (an
// empty document has no root value).
func position(d *jsonc.Doc, pointer string) pos.Pos {
	p, err := d.Pos(pointer)
	if err != nil {
		return pos.Pos{}
	}
	return p
}

// numbersEqual compares two JSON number literals by value. Integer literals
// compare as big integers (exact at any size); anything else as float64,
// which is how encoding/json would read them.
func numbersEqual(x, y json.Number) bool {
	if x == y {
		return true
	}
	if isInt(string(x)) && isInt(string(y)) {
		xi, okX := new(big.Int).SetString(string(x), 10)
		yi, okY := new(big.Int).SetString(string(y), 10)
		return okX && okY && xi.Cmp(yi) == 0
	}
	xf, errX := strconv.ParseFloat(string(x), 64)
	yf, errY := strconv.ParseFloat(string(y), 64)
	if errX != nil || errY != nil {
		// Out of float64 range: only an identical literal is equal, which
		// was checked above.
		return false
	}
	return xf == yf
}

// isInt reports whether a JSON number literal has no fraction or exponent.
func isInt(s string) bool { return !strings.ContainsAny(s, ".eE") }
