// Package generate writes a JSONC file from scratch: nested objects, arrays,
// values, comments and blank lines, in a chosen indent style. It is what an
// `init` command uses to create a commented config file, and FromStruct builds
// one straight from a cfgkit-style config struct.
//
// Guarantees, pinned by tests:
//
//   - The output always parses with jsonc.Parse with zero errors (values go
//     through encoding/json; Raw text is the caller's responsibility).
//   - With Comments(false) and no trailing commas the output is plain JSON,
//     accepted by jsonc.Strict() and encoding/json.
//   - The output is deterministic: the same calls give the same bytes.
//
// A File is not safe for concurrent use while it is being built.
package generate

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ubgo/textdata/jsonc"
)

// Line endings.
const (
	lf   = "\n"
	crlf = "\r\n"
)

// Comment markers and punctuation of the output.
const (
	lineComment   = "// "
	keySeparator  = ": "
	defaultIndent = "  "
	schemaKey     = "$schema"
)

// Option configures a File (layout and FromStruct behaviour).
type Option func(*options)

type options struct {
	indent         string
	eol            string
	comments       bool
	trailingCommas bool
	schemaURL      string
	finalNewline   bool
}

func newOptions(opts []Option) options {
	o := options{indent: defaultIndent, eol: lf, comments: true, finalNewline: true}
	for _, fn := range opts {
		fn(&o)
	}
	return o
}

// Indent sets the indent unit. Only spaces and tabs are accepted; anything
// else makes Bytes fail, since it would not be whitespace in JSON.
func Indent(unit string) Option { return func(o *options) { o.indent = unit } }

// Spaces indents with n spaces (the default is 2).
func Spaces(n int) Option { return Indent(strings.Repeat(" ", max(n, 0))) }

// Tabs indents with one tab per level.
func Tabs() Option { return Indent("\t") }

// CRLF writes "\r\n" line endings instead of "\n".
func CRLF() Option { return func(o *options) { o.eol = crlf } }

// Comments turns comment output on (the default) or off. Off, every comment
// and header is dropped and the output is plain JSON unless TrailingCommas is
// on.
func Comments(on bool) Option { return func(o *options) { o.comments = on } }

// TrailingCommas puts a comma after the last member of every non-empty
// object and array (JSONC only: Strict() rejects it).
func TrailingCommas(on bool) Option { return func(o *options) { o.trailingCommas = on } }

// SchemaURL adds `"$schema": url` as the first key of the root object, so
// editors offer completion and validation from that schema.
func SchemaURL(url string) Option { return func(o *options) { o.schemaURL = url } }

// FinalNewline ends the file with a line ending (the default) or not.
func FinalNewline(on bool) Option { return func(o *options) { o.finalNewline = on } }

// ItemOption decorates one member or element.
type ItemOption func(*entry)

// Doc sets the comment lines written above the item ("\n" separates lines).
func Doc(text string) ItemOption {
	return func(e *entry) {
		if text != "" {
			e.leading = append(e.leading, splitLines(text)...)
		}
	}
}

// Trailing sets the comment written after the item on its last line. Line
// breaks in text become spaces.
func Trailing(text string) ItemOption {
	return func(e *entry) { e.trailing = strings.Join(splitLines(text), " ") }
}

// BlankBefore puts an empty line before the item (and its comment).
func BlankBefore() ItemOption { return func(e *entry) { e.blankBefore = true } }

type entryKind int

const (
	entryValue entryKind = iota
	entryComment
	entryBlank
)

// entry is one line-level element of a container: a member or element, a
// standalone comment, or a blank line.
type entry struct {
	kind        entryKind
	key         string
	value       any // a Go value, jsonc.Raw, *Object or *Array
	leading     []string
	trailing    string
	blankBefore bool
}

// Object is an object being built. Members keep the order they were added.
type Object struct{ entries []*entry }

// Array is an array being built.
type Array struct{ entries []*entry }

// rootObject names the File's embedded root so Object's methods (Set,
// Object, Array, Comment, Blank) are promoted without a field called Object
// shadowing the Object method.
type rootObject = Object

// File is a JSONC file being built: a header comment and a root object whose
// methods (Set, Object, Array, Comment, Blank) it offers directly.
type File struct {
	*rootObject
	header []string
	opts   options
}

// Root returns the root object.
func (f *File) Root() *Object { return f.rootObject }

// New starts a file whose root is an empty object. With SchemaURL, the
// "$schema" key is already its first member.
func New(opts ...Option) *File {
	f := &File{rootObject: &Object{}, opts: newOptions(opts)}
	if f.opts.schemaURL != "" {
		f.Set(schemaKey, f.opts.schemaURL)
	}
	return f
}

// Header adds comment lines above the root object's opening brace.
func (f *File) Header(text string) *File {
	f.header = append(f.header, splitLines(text)...)
	return f
}

// Set adds the member key with a value. value is any Go value encoding/json
// can write (maps and slices are laid out over several lines) or jsonc.Raw,
// written verbatim. Setting a key already added replaces that member in place.
func (o *Object) Set(key string, value any, opts ...ItemOption) *Object {
	o.put(&entry{kind: entryValue, key: key, value: value}, opts)
	return o
}

// Object adds (or replaces) the member key holding a new object and returns
// that object.
func (o *Object) Object(key string, opts ...ItemOption) *Object {
	child := &Object{}
	o.put(&entry{kind: entryValue, key: key, value: child}, opts)
	return child
}

// Array adds (or replaces) the member key holding a new array and returns it.
func (o *Object) Array(key string, opts ...ItemOption) *Array {
	child := &Array{}
	o.put(&entry{kind: entryValue, key: key, value: child}, opts)
	return child
}

// Comment adds a standalone comment line (or lines) at this point.
func (o *Object) Comment(text string) *Object {
	o.entries = append(o.entries, &entry{kind: entryComment, leading: splitLines(text)})
	return o
}

// Blank adds an empty line at this point.
func (o *Object) Blank() *Object {
	o.entries = append(o.entries, &entry{kind: entryBlank})
	return o
}

func (o *Object) put(e *entry, opts []ItemOption) {
	for _, fn := range opts {
		fn(e)
	}
	for i, old := range o.entries {
		if old.kind == entryValue && old.key == e.key {
			o.entries[i] = e
			return
		}
	}
	o.entries = append(o.entries, e)
}

// Add appends an element (see Object.Set for what value may be).
func (a *Array) Add(value any, opts ...ItemOption) *Array {
	a.add(&entry{kind: entryValue, value: value}, opts)
	return a
}

// Object appends a new object element and returns it.
func (a *Array) Object(opts ...ItemOption) *Object {
	child := &Object{}
	a.add(&entry{kind: entryValue, value: child}, opts)
	return child
}

// Array appends a new array element and returns it.
func (a *Array) Array(opts ...ItemOption) *Array {
	child := &Array{}
	a.add(&entry{kind: entryValue, value: child}, opts)
	return child
}

// Comment adds a standalone comment line (or lines) at this point.
func (a *Array) Comment(text string) *Array {
	a.entries = append(a.entries, &entry{kind: entryComment, leading: splitLines(text)})
	return a
}

// Blank adds an empty line at this point.
func (a *Array) Blank() *Array {
	a.entries = append(a.entries, &entry{kind: entryBlank})
	return a
}

func (a *Array) add(e *entry, opts []ItemOption) {
	for _, fn := range opts {
		fn(e)
	}
	a.entries = append(a.entries, e)
}

// Bytes renders the file.
func (f *File) Bytes() ([]byte, error) {
	if strings.Trim(f.opts.indent, " \t") != "" {
		return nil, fmt.Errorf("generate: indent %q is not spaces or tabs", f.opts.indent)
	}
	w := &writer{opts: f.opts}
	if f.opts.comments {
		for _, h := range f.header {
			w.comment("", h)
		}
	}
	if err := w.container(f.rootObject.entries, true, ""); err != nil {
		return nil, err
	}
	if f.opts.finalNewline {
		w.buf.WriteString(f.opts.eol)
	}
	return w.buf.Bytes(), nil
}

// Doc renders the file and parses it, for a caller that goes on editing it.
// opts are passed to jsonc.Parse (e.g. jsonc.WithFileName).
func (f *File) Doc(opts ...jsonc.Option) (*jsonc.Doc, error) {
	b, err := f.Bytes()
	if err != nil {
		return nil, err
	}
	return jsonc.Parse(b, opts...)
}

// writer renders entries into buf.
type writer struct {
	buf  bytes.Buffer
	opts options
}

func (w *writer) comment(indent, text string) {
	w.buf.WriteString(indent)
	if text == "" {
		w.buf.WriteString(strings.TrimRight(lineComment, " "))
	} else {
		w.buf.WriteString(lineComment)
		w.buf.WriteString(text)
	}
	w.buf.WriteString(w.opts.eol)
}

// container writes an object (keyed) or array at the current position; indent
// is the indentation of the line holding its opening bracket.
func (w *writer) container(entries []*entry, keyed bool, indent string) error {
	open, closing := "[", "]"
	if keyed {
		open, closing = "{", "}"
	}
	visible := entries
	if !w.opts.comments {
		visible = nil
		for _, e := range entries {
			if e.kind != entryComment {
				visible = append(visible, e)
			}
		}
	}
	if len(visible) == 0 {
		w.buf.WriteString(open + closing)
		return nil
	}
	last := -1
	for i, e := range visible {
		if e.kind == entryValue {
			last = i
		}
	}
	inner := indent + w.opts.indent
	w.buf.WriteString(open)
	w.buf.WriteString(w.opts.eol)
	for i, e := range visible {
		switch e.kind {
		case entryBlank:
			w.buf.WriteString(w.opts.eol)
			continue
		case entryComment:
			for _, l := range e.leading {
				w.comment(inner, l)
			}
			continue
		}
		if e.blankBefore {
			w.buf.WriteString(w.opts.eol)
		}
		if w.opts.comments {
			for _, l := range e.leading {
				w.comment(inner, l)
			}
		}
		w.buf.WriteString(inner)
		if keyed {
			k, err := marshal(e.key, "", "")
			if err != nil {
				return err
			}
			w.buf.WriteString(k)
			w.buf.WriteString(keySeparator)
		}
		if err := w.value(e, inner); err != nil {
			return err
		}
		if i != last || w.opts.trailingCommas {
			w.buf.WriteByte(',')
		}
		if w.opts.comments && e.trailing != "" {
			w.buf.WriteByte(' ')
			w.buf.WriteString(lineComment)
			w.buf.WriteString(e.trailing)
		}
		w.buf.WriteString(w.opts.eol)
	}
	w.buf.WriteString(indent)
	w.buf.WriteString(closing)
	return nil
}

func (w *writer) value(e *entry, indent string) error {
	switch v := e.value.(type) {
	case *Object:
		return w.container(v.entries, true, indent)
	case *Array:
		return w.container(v.entries, false, indent)
	case jsonc.Raw:
		w.buf.WriteString(string(v))
		return nil
	}
	s, err := marshal(e.value, indent, w.opts.indent)
	if err != nil {
		if e.key != "" {
			return fmt.Errorf("generate: key %q: %w", e.key, err)
		}
		return fmt.Errorf("generate: %w", err)
	}
	if w.opts.eol != lf {
		s = strings.ReplaceAll(s, lf, w.opts.eol)
	}
	w.buf.WriteString(s)
	return nil
}

// marshal writes v as JSON without HTML escaping, laid out with prefix and
// indent when non-empty; the result has no trailing newline.
func marshal(v any, prefix, indent string) (string, error) {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if indent != "" {
		enc.SetIndent(prefix, indent)
	}
	if err := enc.Encode(v); err != nil {
		return "", err
	}
	return strings.TrimSuffix(b.String(), lf), nil
}

// splitLines splits text into lines, accepting "\n" and "\r\n" and dropping
// stray "\r" so a comment never smuggles a line break into the file.
func splitLines(text string) []string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "")
	return strings.Split(text, "\n")
}
