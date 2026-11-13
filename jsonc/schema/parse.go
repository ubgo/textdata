package schema

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"

	"github.com/ubgo/textdata/jsonc"
	"github.com/ubgo/textdata/path"
	"github.com/ubgo/textdata/pos"
)

// ErrSchema is wrapped by every error about the schema itself (as opposed to
// the document being validated).
var ErrSchema = errors.New("schema: invalid schema")

// Parse reads a schema from JSON (comments and trailing commas allowed, as
// in any JSONC file) and checks it: keyword types, every $ref resolves, every
// pattern compiles. Errors carry file:line:col (with jsonc.WithFileName in
// opts) and wrap ErrSchema.
func Parse(src []byte, opts ...jsonc.Option) (*Schema, error) {
	doc, err := jsonc.Parse(src, opts...)
	if err != nil {
		return nil, fmt.Errorf("schema: %w", err)
	}
	plain, err := doc.JSON()
	if err != nil {
		return nil, fmt.Errorf("schema: %w", err)
	}
	v, err := decodeOrdered(plain)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrSchema, err)
	}
	b := &builder{doc: doc, file: fileOf(doc, opts)}
	s, err := b.build(v, nil)
	if err != nil {
		return nil, err
	}
	if _, err := compile(s); err != nil {
		return nil, err
	}
	return s, nil
}

// Load reads and parses the schema file at name.
func Load(name string) (*Schema, error) {
	src, err := os.ReadFile(name)
	if err != nil {
		return nil, fmt.Errorf("schema: %w", err)
	}
	return Parse(src, jsonc.WithFileName(name))
}

// UnmarshalJSON reads a schema like Parse, without the $ref and pattern
// checks (Validate reports those).
func (s *Schema) UnmarshalJSON(src []byte) error {
	v, err := decodeOrdered(src)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrSchema, err)
	}
	built, err := (&builder{}).build(v, nil)
	if err != nil {
		return err
	}
	*s = *built
	return nil
}

// fileOf recovers the file name a WithFileName option set, by applying the
// options to a probe document whose error reports it.
func fileOf(doc *jsonc.Doc, opts []jsonc.Option) string {
	if doc.Location() != "" {
		return doc.Location()
	}
	probe, err := jsonc.Parse([]byte("?"), opts...)
	_ = probe // only the error carries the name
	if pe := pos.First(err); pe != nil {
		return pe.File
	}
	return ""
}

// builder turns decoded JSON into a Schema, reporting errors at the
// keyword's position.
type builder struct {
	doc  *jsonc.Doc
	file string
}

func (b *builder) errorf(at path.Path, format string, args ...any) error {
	msg := fmt.Sprintf(format, args...)
	where := "#" + at.Pointer()
	if b.doc != nil {
		if p, err := b.doc.Pos(at.String()); err == nil {
			return &pos.Error{File: b.file, Pos: p, Msg: where + ": " + msg, Err: ErrSchema}
		}
	}
	return fmt.Errorf("%w: %s: %s", ErrSchema, where, msg)
}

func (b *builder) build(v any, at path.Path) (*Schema, error) {
	switch x := v.(type) {
	case bool:
		return &Schema{Bool: &x}, nil
	case *orderedObject:
		return b.object(x, at)
	}
	return nil, b.errorf(at, "a schema must be an object or a boolean")
}

func (b *builder) object(o *orderedObject, at path.Path) (*Schema, error) {
	s := &Schema{}
	for _, k := range o.keys {
		v := o.vals[k]
		kat := at.Append(path.Key(k))
		var err error
		switch k {
		case kwSchema:
			s.SchemaURI, err = b.str(v, kat)
		case kwID:
			s.ID, err = b.str(v, kat)
		case kwRef:
			s.Ref, err = b.str(v, kat)
		case kwComment:
			s.Comment, err = b.str(v, kat)
		case kwTitle:
			s.Title, err = b.str(v, kat)
		case kwDescription:
			s.Description, err = b.str(v, kat)
		case kwFormat:
			s.Format, err = b.str(v, kat)
		case kwContentEncoding:
			s.ContentEncoding, err = b.str(v, kat)
		case kwPattern:
			s.Pattern, err = b.str(v, kat)
		case kwType:
			s.Types, err = b.types(v, kat)
		case kwEnum:
			arr, ok := v.([]any)
			if !ok {
				return nil, b.errorf(kat, "enum must be an array")
			}
			s.Enum = make([]json.RawMessage, 0, len(arr))
			for _, e := range arr {
				s.Enum = append(s.Enum, rawOf(e))
			}
		case kwConst:
			s.Const = rawOf(v)
		case kwDefault:
			s.Default = rawOf(v)
		case kwMinimum:
			s.Minimum, err = b.number(v, kat)
		case kwMaximum:
			s.Maximum, err = b.number(v, kat)
		case kwExclusiveMinimum:
			s.ExclusiveMinimum, err = b.number(v, kat)
		case kwExclusiveMaximum:
			s.ExclusiveMaximum, err = b.number(v, kat)
		case kwMinLength:
			s.MinLength, err = b.count(v, kat)
		case kwMaxLength:
			s.MaxLength, err = b.count(v, kat)
		case kwMinItems:
			s.MinItems, err = b.count(v, kat)
		case kwMaxItems:
			s.MaxItems, err = b.count(v, kat)
		case kwItems:
			s.Items, err = b.build(v, kat)
		case kwAdditional:
			s.AdditionalProperties, err = b.build(v, kat)
		case kwRequired:
			s.Required, err = b.strings(v, kat)
		case kwNot:
			s.Not, err = b.build(v, kat)
		case kwAllOf:
			s.AllOf, err = b.list(v, kat)
		case kwAnyOf:
			s.AnyOf, err = b.list(v, kat)
		case kwOneOf:
			s.OneOf, err = b.list(v, kat)
		case kwProperties, kwDefs:
			m, ok := v.(*orderedObject)
			if !ok {
				return nil, b.errorf(kat, "%s must be an object", k)
			}
			out := make(map[string]*Schema, len(m.keys))
			for _, name := range m.keys {
				if out[name], err = b.build(m.vals[name], kat.Append(path.Key(name))); err != nil {
					return nil, err
				}
			}
			if k == kwProperties {
				s.Properties, s.PropertyOrder = out, append([]string(nil), m.keys...)
			} else {
				s.Defs = out
			}
		default:
			if s.Extra == nil {
				s.Extra = map[string]json.RawMessage{}
			}
			s.Extra[k] = rawOf(v)
		}
		if err != nil {
			return nil, err
		}
	}
	return s, nil
}

func (b *builder) list(v any, at path.Path) ([]*Schema, error) {
	arr, ok := v.([]any)
	if !ok || len(arr) == 0 {
		return nil, b.errorf(at, "must be a non-empty array of schemas")
	}
	out := make([]*Schema, 0, len(arr))
	for i, e := range arr {
		s, err := b.build(e, at.Append(path.Index(i)))
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, nil
}

func (b *builder) str(v any, at path.Path) (string, error) {
	s, ok := v.(string)
	if !ok {
		return "", b.errorf(at, "must be a string")
	}
	return s, nil
}

func (b *builder) strings(v any, at path.Path) ([]string, error) {
	arr, ok := v.([]any)
	if !ok {
		return nil, b.errorf(at, "must be an array of strings")
	}
	out := make([]string, 0, len(arr))
	for i, e := range arr {
		s, ok := e.(string)
		if !ok {
			return nil, b.errorf(at.Append(path.Index(i)), "must be a string")
		}
		out = append(out, s)
	}
	return out, nil
}

func (b *builder) types(v any, at path.Path) ([]string, error) {
	var names []string
	switch x := v.(type) {
	case string:
		names = []string{x}
	case []any:
		var err error
		if names, err = b.strings(x, at); err != nil {
			return nil, err
		}
	default:
		// "type" takes one name or a list of names; say both.
		return nil, b.errorf(at, "must be a string or an array of strings")
	}
	for _, n := range names {
		if !typeNames[n] {
			return nil, b.errorf(at, "unknown type %q", n)
		}
	}
	return names, nil
}

func (b *builder) number(v any, at path.Path) (json.Number, error) {
	n, ok := v.(json.Number)
	if !ok {
		return "", b.errorf(at, "must be a number")
	}
	return n, nil
}

func (b *builder) count(v any, at path.Path) (*int, error) {
	n, ok := v.(json.Number)
	if ok {
		if i, err := strconv.Atoi(n.String()); err == nil && i >= 0 {
			return &i, nil
		}
		if r, ok := parseNum(n.String()); ok && r.isInt() && r.sign() >= 0 {
			// 2.0 or 1e2: an integer written as a number; clamp huge counts.
			i := r.clampInt()
			return &i, nil
		}
	}
	return nil, b.errorf(at, "must be a non-negative integer")
}

// rawOf re-encodes a decoded value (key order kept).
func rawOf(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		return json.RawMessage("null")
	}
	return b
}

// orderedObject is a decoded JSON object that remembers key order. A
// duplicate key keeps its first position and its last value.
type orderedObject struct {
	keys []string
	vals map[string]any
}

// MarshalJSON writes the members in their original order.
func (o *orderedObject) MarshalJSON() ([]byte, error) {
	w := &objWriter{}
	for _, k := range o.keys {
		w.val(k, o.vals[k])
	}
	if w.firstErr != nil {
		return nil, w.firstErr
	}
	return w.bytes(), nil
}

// decodeOrdered decodes one JSON value: *orderedObject, []any, string,
// json.Number, bool or nil.
func decodeOrdered(src []byte) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(src))
	dec.UseNumber()
	v, err := decodeValue(dec)
	if err != nil {
		return nil, err
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, fmt.Errorf("unexpected data after the schema")
	}
	return v, nil
}

func decodeValue(dec *json.Decoder) (any, error) {
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	switch t := tok.(type) {
	case json.Delim:
		switch t {
		case '{':
			o := &orderedObject{vals: map[string]any{}}
			for dec.More() {
				kt, err := dec.Token()
				if err != nil {
					return nil, err
				}
				k, _ := kt.(string) // object keys are always strings
				v, err := decodeValue(dec)
				if err != nil {
					return nil, err
				}
				if _, dup := o.vals[k]; !dup {
					o.keys = append(o.keys, k)
				}
				o.vals[k] = v
			}
			_, err := dec.Token()
			return o, err
		case '[':
			arr := []any{}
			for dec.More() {
				v, err := decodeValue(dec)
				if err != nil {
					return nil, err
				}
				arr = append(arr, v)
			}
			_, err := dec.Token()
			return arr, err
		}
		return nil, fmt.Errorf("unexpected %v", t)
	}
	return tok, nil
}
