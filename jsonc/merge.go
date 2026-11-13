package jsonc

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ubgo/textdata/path"
)

// compactJSON is a value already encoded as compact JSON, key order kept.
// render indents it in the document's style (json.Indent keeps the order,
// where re-encoding a decoded map would sort the keys).
type compactJSON []byte

// keyedValue is a member value inserted with its key's exact source text
// (escapes and raw bytes kept), so a merged key decodes exactly as in the
// document it came from. jsonString would turn invalid UTF-8 into U+FFFD.
type keyedValue struct {
	keyText string
	value   any
}

// Merge deep-merges other into d, for a base file plus an override file:
// objects merge recursively by key; any other value in other (scalar, array,
// or an object replacing a non-object) replaces d's whole; a key d lacks is
// added at the end of its object in d's indentation and comma style, as Set
// adds it, carrying other's leading // comment lines along when it lands on
// a line of its own (never under Strict()).
//
// Every byte of d outside the values it replaces or the members it adds is
// kept, comments included. A value equal to d's (as JSON) is left alone, so a
// re-merge changes nothing. Values copied from other are re-indented and lose
// comments inside them; key order is kept.
//
// Duplicate keys follow encoding/json: other's last member with a key is the
// one merged, and it lands on d's last member with that key. An empty other
// is a no-op; an empty d becomes a copy of other's root object.
//
// Merge refuses, leaving d unchanged, when either document has syntax errors
// (ErrInvalid) or a root value is not an object (ErrType).
func (d *Doc) Merge(other *Doc) error {
	if other == nil {
		return nil
	}
	if err := d.valid(); err != nil {
		return fmt.Errorf("jsonc: merge into: %w", err)
	}
	if err := other.valid(); err != nil {
		return fmt.Errorf("jsonc: merge from: %w", err)
	}
	if other.root == nil {
		return nil
	}
	if other.root.kind != KindObject {
		return fmt.Errorf("%w: merge: the merged document's root is %s, not an object", ErrType, article(other.root.kind))
	}
	if d.root == nil {
		return d.createRoot(compactJSON(other.compact(other.root)))
	}
	if d.root.kind != KindObject {
		return fmt.Errorf("%w: merge: the target document's root is %s, not an object", ErrType, article(d.root.kind))
	}
	// Work on a copy so a failure halfway leaves d as it was.
	c := &Doc{src: append([]byte(nil), d.src...), opts: d.opts}
	c.reparse()
	if err := c.mergeObject(nil, other, other.root); err != nil {
		return err
	}
	d.src, d.root, d.comments, d.errs = c.src, c.root, c.comments, c.errs
	d.idx.Store(nil)
	return nil
}

// mergeObject merges the members of on (an object of o) into the object of
// d at p. Each edit re-parses, so nodes of d are looked up by path every time.
func (d *Doc) mergeObject(p path.Path, o *Doc, on *node) error {
	for i, it := range on.items {
		if last, _ := child(on, path.Key(it.key)); last != i {
			continue // an earlier duplicate: the last one wins
		}
		pp := p.Append(path.Key(it.key))
		loc, err := d.find(pp)
		if err == nil {
			if loc.n.kind == KindObject && it.value.kind == KindObject {
				if err := d.mergeObject(pp, o, it.value); err != nil {
					return err
				}
				continue
			}
			v := o.compact(it.value)
			if bytes.Equal(d.compact(loc.n), v) {
				continue
			}
			if err := d.replace(loc, compactJSON(v)); err != nil {
				return fmt.Errorf("jsonc: merge %q: %w", pp.String(), err)
			}
			continue
		}
		ploc, err := d.find(p)
		if err != nil {
			return fmt.Errorf("jsonc: merge %q: %w", pp.String(), err)
		}
		if err := d.insertItem(ploc.n, len(ploc.n.items), it.key, true,
			keyedValue{keyText: string(o.src[it.keyStart:it.keyEnd]), value: compactJSON(o.compact(it.value))}); err != nil {
			return fmt.Errorf("jsonc: merge %q: %w", pp.String(), err)
		}
		if err := d.carryComments(pp, o, it); err != nil {
			return fmt.Errorf("jsonc: merge %q: %w", pp.String(), err)
		}
	}
	return nil
}

// carryComments copies the leading comments of oit (a member of o) above the
// member just added at p, when that is clean: d is not strict, the member
// starts its own line and every comment fits on one line.
func (d *Doc) carryComments(p path.Path, o *Doc, oit *item) error {
	if d.opts.strict || len(oit.leading) == 0 {
		return nil
	}
	loc, err := d.find(p)
	if err != nil || loc.it == nil || !ownLineStart(d.src, loc.it.start()) {
		return nil
	}
	indent := indentOf(d.src, loc.it.start())
	ls := lineStart(d.src, loc.it.start())
	eol := d.eolAt(ls)
	var b strings.Builder
	for _, c := range oit.leading {
		text := string(o.src[c.start:c.end])
		if strings.ContainsAny(text, "\r\n") {
			return nil
		}
		b.WriteString(indent)
		b.WriteString(text)
		b.WriteString(eol)
	}
	return d.apply(splice{ls, ls, b.String()})
}

// compact returns n's value as compact plain JSON (no comments, no trailing
// commas, no whitespace), key order kept.
func (d *Doc) compact(n *node) []byte {
	var b bytes.Buffer
	if err := json.Compact(&b, d.plain(n.start, n.end)); err != nil {
		// Unreachable for a valid document: plain() of a value is JSON.
		return d.plain(n.start, n.end)
	}
	return b.Bytes()
}
