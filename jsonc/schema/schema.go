// Package schema builds JSON Schema (draft 2020-12) from a config struct and
// validates JSONC documents against a schema, reporting file:line:col.
//
// FromStruct reads the same tags as jsonc/generate (cfgkit's tags), so a file
// written by generate.FromStruct validates clean against schema.FromStruct of
// the same struct; adding `"$schema"` to the file lets editors complete keys.
//
// Validate checks the subset of draft 2020-12 a config file needs: type,
// enum, const, minimum, maximum, exclusiveMinimum, exclusiveMaximum,
// minLength, maxLength, pattern, items, minItems, maxItems, properties,
// required, additionalProperties, allOf, anyOf, oneOf, not and local $ref
// into $defs. Annotations
// (title, description, default, format, examples, ...) are kept and ignored.
// Any other keyword (if/then/else, patternProperties, ...) is kept for
// round trips and reported as a warning where it applies, never silently
// treated as satisfied. A key that additionalProperties: false forbids is a
// warning, not an error: an unknown key is usually a typo or a newer option,
// and a config tool should say so without refusing the file.
//
// pattern is a Go regexp (RE2): lookaround and backreferences are not
// supported and make Parse fail. Like ECMA-262 it matches anywhere in the
// string unless anchored.
//
// A Schema is safe for concurrent Validate calls; Validate never modifies it.
package schema

import (
	"bytes"
	"encoding/json"
	"sort"
)

// Draft is the dialect URI FromStruct writes in "$schema".
const Draft = "https://json-schema.org/draft/2020-12/schema"

// Keywords, in the order MarshalJSON writes them.
const (
	kwSchema           = "$schema"
	kwID               = "$id"
	kwRef              = "$ref"
	kwComment          = "$comment"
	kwTitle            = "title"
	kwDescription      = "description"
	kwType             = "type"
	kwFormat           = "format"
	kwContentEncoding  = "contentEncoding"
	kwEnum             = "enum"
	kwConst            = "const"
	kwDefault          = "default"
	kwMinimum          = "minimum"
	kwMaximum          = "maximum"
	kwExclusiveMinimum = "exclusiveMinimum"
	kwExclusiveMaximum = "exclusiveMaximum"
	kwMinLength        = "minLength"
	kwMaxLength        = "maxLength"
	kwPattern          = "pattern"
	kwItems            = "items"
	kwMinItems         = "minItems"
	kwMaxItems         = "maxItems"
	kwProperties       = "properties"
	kwRequired         = "required"
	kwAdditional       = "additionalProperties"
	kwDefs             = "$defs"
	kwAllOf            = "allOf"
	kwAnyOf            = "anyOf"
	kwOneOf            = "oneOf"
	kwNot              = "not"
)

// Type names.
const (
	TypeObject  = "object"
	TypeArray   = "array"
	TypeString  = "string"
	TypeNumber  = "number"
	TypeInteger = "integer"
	TypeBoolean = "boolean"
	TypeNull    = "null"
)

// typeNames is the closed set "type" accepts.
var typeNames = map[string]bool{
	TypeObject: true, TypeArray: true, TypeString: true, TypeNumber: true,
	TypeInteger: true, TypeBoolean: true, TypeNull: true,
}

// annotations are keywords with no assertion: kept in Extra and never warned
// about.
var annotations = map[string]bool{
	"examples": true, "deprecated": true, "readOnly": true, "writeOnly": true,
	"contentMediaType": true, "contentSchema": true, "$anchor": true,
	"$dynamicAnchor": true, "$vocabulary": true, "definitions": true,
	"markdownDescription": true, "deprecationMessage": true,
	"errorMessage": true, "defaultSnippets": true,
}

// Schema is one JSON Schema. The zero Schema accepts everything.
type Schema struct {
	// Bool, when non-nil, makes this a boolean schema: true accepts every
	// value, false none. The other fields are then ignored.
	Bool *bool

	SchemaURI   string // $schema
	ID          string // $id
	Ref         string // $ref: a local reference ("#", "#/$defs/name")
	Comment     string // $comment
	Title       string
	Description string

	// Types is "type": one name or several.
	Types           []string
	Format          string
	ContentEncoding string

	Enum    []json.RawMessage
	Const   json.RawMessage
	Default json.RawMessage

	// Number bounds are kept as written so comparisons are exact.
	Minimum          json.Number
	Maximum          json.Number
	ExclusiveMinimum json.Number
	ExclusiveMaximum json.Number

	MinLength *int
	MaxLength *int
	Pattern   string

	Items    *Schema
	MinItems *int
	MaxItems *int

	Properties map[string]*Schema
	// PropertyOrder is the order properties are written in; Parse and
	// FromStruct fill it. Properties missing from it are written after,
	// sorted.
	PropertyOrder []string
	Required      []string
	// AdditionalProperties: nil allows any other key; a false boolean schema
	// forbids them (a warning); any other schema validates them.
	AdditionalProperties *Schema

	// AllOf, AnyOf and OneOf hold subschemas the value must match all of,
	// at least one of, or exactly one of; Not one it must not match.
	AllOf []*Schema
	AnyOf []*Schema
	OneOf []*Schema
	Not   *Schema

	Defs map[string]*Schema

	// Extra holds every other keyword as written.
	Extra map[string]json.RawMessage
}

// True and False return the boolean schemas.
func True() *Schema  { b := true; return &Schema{Bool: &b} }
func False() *Schema { b := false; return &Schema{Bool: &b} }

// JSON returns the schema as indented JSON (two spaces), keywords in a fixed
// order and properties in PropertyOrder, so output is stable for diffs and
// go:generate.
func (s *Schema) JSON() ([]byte, error) {
	compact, err := s.MarshalJSON()
	if err != nil {
		return nil, err
	}
	var out bytes.Buffer
	if err := json.Indent(&out, compact, "", "  "); err != nil {
		return nil, err
	}
	out.WriteByte('\n')
	return out.Bytes(), nil
}

// MarshalJSON writes the schema with keywords in a fixed order.
func (s *Schema) MarshalJSON() ([]byte, error) {
	if s == nil {
		return []byte("true"), nil
	}
	if s.Bool != nil {
		return json.Marshal(*s.Bool)
	}
	w := &objWriter{}
	w.str(kwSchema, s.SchemaURI)
	w.str(kwID, s.ID)
	w.str(kwRef, s.Ref)
	w.str(kwComment, s.Comment)
	w.str(kwTitle, s.Title)
	w.str(kwDescription, s.Description)
	switch len(s.Types) {
	case 0:
	case 1:
		w.val(kwType, s.Types[0])
	default:
		w.val(kwType, s.Types)
	}
	w.str(kwFormat, s.Format)
	w.str(kwContentEncoding, s.ContentEncoding)
	if s.Enum != nil {
		w.val(kwEnum, s.Enum)
	}
	w.raw(kwConst, s.Const)
	w.raw(kwDefault, s.Default)
	w.num(kwMinimum, s.Minimum)
	w.num(kwMaximum, s.Maximum)
	w.num(kwExclusiveMinimum, s.ExclusiveMinimum)
	w.num(kwExclusiveMaximum, s.ExclusiveMaximum)
	w.intp(kwMinLength, s.MinLength)
	w.intp(kwMaxLength, s.MaxLength)
	w.str(kwPattern, s.Pattern)
	if s.Items != nil {
		w.val(kwItems, s.Items)
	}
	w.intp(kwMinItems, s.MinItems)
	w.intp(kwMaxItems, s.MaxItems)
	if s.Properties != nil {
		w.key(kwProperties)
		pw := &objWriter{}
		for _, name := range ordered(s.Properties, s.PropertyOrder) {
			pw.val(name, s.Properties[name])
		}
		w.err(pw.firstErr)
		w.buf.Write(pw.bytes())
	}
	if s.Required != nil {
		w.val(kwRequired, s.Required)
	}
	if s.AdditionalProperties != nil {
		w.val(kwAdditional, s.AdditionalProperties)
	}
	if s.AllOf != nil {
		w.val(kwAllOf, s.AllOf)
	}
	if s.AnyOf != nil {
		w.val(kwAnyOf, s.AnyOf)
	}
	if s.OneOf != nil {
		w.val(kwOneOf, s.OneOf)
	}
	if s.Not != nil {
		w.val(kwNot, s.Not)
	}
	for _, k := range sortedKeys(s.Extra) {
		w.raw(k, s.Extra[k])
	}
	if s.Defs != nil {
		w.key(kwDefs)
		dw := &objWriter{}
		for _, name := range sortedKeys(s.Defs) {
			dw.val(name, s.Defs[name])
		}
		w.err(dw.firstErr)
		w.buf.Write(dw.bytes())
	}
	if w.firstErr != nil {
		return nil, w.firstErr
	}
	return w.bytes(), nil
}

// ordered lists m's keys in order first (once each, only those present),
// then the rest sorted.
func ordered[V any](m map[string]V, order []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, k := range order {
		if _, ok := m[k]; ok && !seen[k] {
			seen[k] = true
			out = append(out, k)
		}
	}
	for _, k := range sortedKeys(m) {
		if !seen[k] {
			out = append(out, k)
		}
	}
	return out
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// objWriter writes one JSON object member by member.
type objWriter struct {
	buf      bytes.Buffer
	n        int
	firstErr error
}

func (w *objWriter) err(err error) {
	if err != nil && w.firstErr == nil {
		w.firstErr = err
	}
}

func (w *objWriter) key(k string) {
	if w.n == 0 {
		w.buf.WriteByte('{')
	} else {
		w.buf.WriteByte(',')
	}
	w.n++
	kb, _ := json.Marshal(k) // a string always marshals
	w.buf.Write(kb)
	w.buf.WriteByte(':')
}

func (w *objWriter) val(k string, v any) {
	b, err := json.Marshal(v)
	if err != nil {
		w.err(err)
		return
	}
	w.key(k)
	w.buf.Write(b)
}

func (w *objWriter) str(k, v string) {
	if v != "" {
		w.val(k, v)
	}
}

func (w *objWriter) raw(k string, v json.RawMessage) {
	if v != nil {
		w.val(k, v)
	}
}

func (w *objWriter) num(k string, v json.Number) {
	if v != "" {
		w.val(k, v)
	}
}

func (w *objWriter) intp(k string, v *int) {
	if v != nil {
		w.val(k, *v)
	}
}

func (w *objWriter) bytes() []byte {
	if w.n == 0 {
		return []byte("{}")
	}
	w.buf.WriteByte('}')
	return w.buf.Bytes()
}
