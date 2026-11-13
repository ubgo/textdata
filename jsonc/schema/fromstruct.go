package schema

import (
	"encoding/json"
	"fmt"
	"path"
	"reflect"
	"regexp"
	"strconv"

	"github.com/ubgo/textdata/jsonc/internal/structtag"
)

// Annotations FromStruct writes.
const (
	formatDateTime  = "date-time"
	encodingBase64  = "base64"
	durationNote    = "Duration in nanoseconds."
	defsRefPrefix   = "#/" + kwDefs + "/"
	rootRef         = "#"
	unsafeDefsChars = `[^A-Za-z0-9_.-]+`
)

var unsafeDefsName = regexp.MustCompile(unsafeDefsChars)

// StructOption configures FromStruct.
type StructOption func(*structOptions)

type structOptions struct {
	id              string
	title           string
	allowAdditional bool
}

// WithID sets "$id".
func WithID(id string) StructOption { return func(o *structOptions) { o.id = id } }

// WithTitle sets the root "title".
func WithTitle(title string) StructOption { return func(o *structOptions) { o.title = title } }

// AllowAdditional leaves additionalProperties out, so unknown keys are not
// even warned about. By default every struct object has
// additionalProperties: false, so a typo in a key is reported (as a warning).
func AllowAdditional() StructOption { return func(o *structOptions) { o.allowAdditional = true } }

// FromStruct builds a draft 2020-12 schema from a config struct (or a pointer
// to one), with the tags jsonc/generate reads:
//
//   - properties and their order follow the `json` tags with encoding/json's
//     rules (embedded structs flattened, json:"-" and unexported skipped);
//   - `doc` is the description, `default` the default (parsed for the field's
//     type, then written as JSON), `enum` the allowed values;
//   - env ",required" / ",notempty" or required:"true" add the key to
//     "required"; notempty adds minLength 1 to a string;
//   - Go types map to JSON types: ints are "integer" (uints with minimum 0),
//     time.Duration is an integer of nanoseconds, time.Time a date-time
//     string, a TextMarshaler a string, []byte a base64 string, a map an
//     object of its element schema, a pointer to a scalar also allows null;
//   - a struct type used more than once, or recursively, goes into "$defs"
//     and is referenced with "$ref";
//   - the root allows a "$schema" string key, so a file can point editors at
//     its schema.
//
// A default or enum value that does not parse for its field's type is an
// error naming the field. Only v's type is used, never its values.
func FromStruct(v any, opts ...StructOption) (*Schema, error) {
	o := structOptions{}
	for _, fn := range opts {
		fn(&o)
	}
	t := reflect.TypeOf(v)
	if t == nil {
		return nil, fmt.Errorf("schema: FromStruct needs a struct, got nil")
	}
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if shape, _, _ := structtag.ShapeOf(t); shape != structtag.ShapeStruct {
		return nil, fmt.Errorf("schema: FromStruct needs a struct, got %s", t)
	}
	g := &gen{opts: o, root: t, uses: map[reflect.Type]int{}, defNames: map[reflect.Type]string{}, taken: map[string]bool{}, defs: map[string]*Schema{}}
	g.count(t, map[reflect.Type]bool{})
	s, err := g.structSchema(t, "")
	if err != nil {
		return nil, err
	}
	s.SchemaURI = Draft
	s.ID = o.id
	s.Title = o.title
	if s.Properties == nil {
		s.Properties = map[string]*Schema{}
	}
	if _, clash := s.Properties[kwSchema]; !clash {
		s.Properties[kwSchema] = &Schema{Types: []string{TypeString}, Description: "The JSON Schema this file follows."}
		s.PropertyOrder = append([]string{kwSchema}, s.PropertyOrder...)
	}
	if len(g.defs) > 0 {
		s.Defs = g.defs
	}
	if _, err := compile(s); err != nil {
		return nil, err
	}
	return s, nil
}

// gen holds the state of one FromStruct call.
type gen struct {
	opts structOptions
	root reflect.Type
	// uses counts how often each walked struct type is reached; a count over
	// one (or a recursive use) earns it a $defs entry.
	uses     map[reflect.Type]int
	defNames map[reflect.Type]string
	taken    map[string]bool
	defs     map[string]*Schema
}

// count walks the type graph once, counting struct uses.
func (g *gen) count(t reflect.Type, onStack map[reflect.Type]bool) {
	shape, elem, _ := structtag.ShapeOf(t)
	switch shape {
	case structtag.ShapeStruct:
		g.uses[elem]++
		if onStack[elem] {
			g.uses[elem]++ // recursive: always a $def
			return
		}
		if g.uses[elem] > 1 {
			return
		}
		onStack[elem] = true
		for _, f := range structtag.Fields(elem) {
			g.count(f.Type, onStack)
		}
		delete(onStack, elem)
	case structtag.ShapeMap, structtag.ShapeSlice:
		g.count(elem.Elem(), onStack)
	}
}

// typeSchema is the schema of a Go type (without field annotations).
func (g *gen) typeSchema(t reflect.Type, goPath string) (*Schema, error) {
	shape, elem, nullable := structtag.ShapeOf(t)
	var s *Schema
	switch shape {
	case structtag.ShapeUnsupported:
		return nil, fmt.Errorf("schema: field %s: type %s cannot be written as JSON", goPath, t)
	case structtag.ShapeStruct:
		r, err := g.structRef(elem, goPath)
		if err != nil || !nullable {
			return r, err
		}
		// A nil pointer is written as null; encoding/json reads null back.
		return &Schema{AnyOf: []*Schema{r, {Types: []string{TypeNull}}}}, nil
	case structtag.ShapeMap:
		vs, err := g.typeSchema(elem.Elem(), goPath+"[]")
		if err != nil {
			return nil, err
		}
		s = &Schema{Types: []string{TypeObject}, AdditionalProperties: vs}
	case structtag.ShapeSlice:
		is, err := g.typeSchema(elem.Elem(), goPath+"[]")
		if err != nil {
			return nil, err
		}
		s = &Schema{Types: []string{TypeArray}, Items: is}
		if elem.Kind() == reflect.Array {
			n := elem.Len()
			s.MinItems, s.MaxItems = &n, &n
		} else {
			nullable = true // a nil slice is written as null
		}
	case structtag.ShapeBytes:
		s = &Schema{Types: []string{TypeString}, ContentEncoding: encodingBase64}
		nullable = true
	case structtag.ShapeString, structtag.ShapeText:
		s = &Schema{Types: []string{TypeString}}
	case structtag.ShapeTime:
		s = &Schema{Types: []string{TypeString}, Format: formatDateTime}
	case structtag.ShapeBool:
		s = &Schema{Types: []string{TypeBoolean}}
	case structtag.ShapeInt:
		s = &Schema{Types: []string{TypeInteger}}
		if bits := elem.Bits(); bits < 64 {
			// int8/16/32: encoding/json rejects a value out of range.
			s.Minimum = json.Number(strconv.FormatInt(-1<<(bits-1), 10))
			s.Maximum = json.Number(strconv.FormatInt(1<<(bits-1)-1, 10))
		}
	case structtag.ShapeDuration:
		s = &Schema{Types: []string{TypeInteger}}
	case structtag.ShapeUint:
		s = &Schema{Types: []string{TypeInteger}, Minimum: "0"}
		if bits := elem.Bits(); bits < 64 {
			s.Maximum = json.Number(strconv.FormatUint(1<<bits-1, 10))
		}
	case structtag.ShapeFloat:
		s = &Schema{Types: []string{TypeNumber}}
	default: // ShapeAny, ShapeOpaque: any JSON value
		return &Schema{}, nil
	}
	if nullable {
		s.Types = append(s.Types, TypeNull)
	}
	return s, nil
}

// structRef is the schema for a struct type: inline, or a $ref when the type
// is used more than once.
func (g *gen) structRef(t reflect.Type, goPath string) (*Schema, error) {
	if t == g.root {
		return &Schema{Ref: rootRef}, nil
	}
	if g.uses[t] <= 1 {
		return g.structSchema(t, goPath)
	}
	name, done := g.defNames[t]
	if !done {
		name = g.defName(t)
		g.defNames[t] = name
		g.defs[name] = &Schema{} // placeholder: a recursive use refers to it
		s, err := g.structSchema(t, goPath)
		if err != nil {
			return nil, err
		}
		g.defs[name] = s
	}
	return &Schema{Ref: defsRefPrefix + escape(name)}, nil
}

// defName picks a unique $defs name: the type name, else package.Type, else
// a numbered one.
func (g *gen) defName(t reflect.Type) string {
	base := unsafeDefsName.ReplaceAllString(t.Name(), "_")
	if base == "" {
		base = "Object"
	}
	candidates := []string{base}
	if pkg := path.Base(t.PkgPath()); pkg != "." && pkg != "/" {
		candidates = append(candidates, unsafeDefsName.ReplaceAllString(pkg, "_")+"."+base)
	}
	for _, c := range candidates {
		if !g.taken[c] {
			g.taken[c] = true
			return c
		}
	}
	for i := 2; ; i++ {
		c := base + strconv.Itoa(i)
		if !g.taken[c] {
			g.taken[c] = true
			return c
		}
	}
}

func (g *gen) structSchema(t reflect.Type, goPath string) (*Schema, error) {
	s := &Schema{Types: []string{TypeObject}, Properties: map[string]*Schema{}}
	if !g.opts.allowAdditional {
		s.AdditionalProperties = False()
	}
	for _, fd := range structtag.Fields(t) {
		fieldPath := fd.GoName
		if goPath != "" {
			fieldPath = goPath + "." + fd.GoName
		}
		ps, err := g.fieldSchema(fd, fieldPath)
		if err != nil {
			return nil, err
		}
		s.Properties[fd.Name] = ps
		s.PropertyOrder = append(s.PropertyOrder, fd.Name)
		if fd.Required {
			s.Required = append(s.Required, fd.Name)
		}
	}
	return s, nil
}

func (g *gen) fieldSchema(fd structtag.Field, fieldPath string) (*Schema, error) {
	s, err := g.typeSchema(fd.Type, fieldPath)
	if err != nil {
		return nil, err
	}
	shape, _, nullable := structtag.ShapeOf(fd.Type)
	if fd.Quoted && isQuotable(shape) {
		// json:",string" writes the scalar inside a JSON string.
		s = &Schema{Types: []string{TypeString}}
		if nullable {
			s.Types = append(s.Types, TypeNull)
		}
	}
	s.Description = fd.Doc
	if shape == structtag.ShapeDuration {
		if s.Description != "" {
			s.Description += " "
		}
		s.Description += durationNote
	}
	if fd.NotEmpty && shape == structtag.ShapeString && !fd.Quoted {
		one := 1
		s.MinLength = &one
	}
	if def, ok, err := fd.DefaultValue(); err != nil {
		return nil, fmt.Errorf("schema: field %s: %w", fieldPath, err)
	} else if ok && !fd.Secret {
		raw, err := fieldJSON(def, fd.Quoted && isQuotable(shape))
		if err != nil {
			return nil, fmt.Errorf("schema: field %s: default: %w", fieldPath, err)
		}
		s.Default = raw
	}
	if len(fd.Enum) > 0 {
		vals, err := fd.EnumValues()
		if err != nil {
			return nil, fmt.Errorf("schema: field %s: %w", fieldPath, err)
		}
		for _, ev := range vals {
			raw, err := fieldJSON(ev, fd.Quoted && isQuotable(shape))
			if err != nil {
				return nil, fmt.Errorf("schema: field %s: enum: %w", fieldPath, err)
			}
			s.Enum = append(s.Enum, raw)
		}
		if nullable {
			s.Enum = append(s.Enum, json.RawMessage("null"))
		}
	}
	return s, nil
}

func isQuotable(shape structtag.Shape) bool {
	switch shape {
	case structtag.ShapeString, structtag.ShapeBool, structtag.ShapeInt, structtag.ShapeUint, structtag.ShapeFloat, structtag.ShapeDuration:
		return true
	}
	return false
}

// fieldJSON writes a value as encoding/json writes the field.
func fieldJSON(v reflect.Value, quoted bool) (json.RawMessage, error) {
	b, err := json.Marshal(v.Interface())
	if err != nil {
		return nil, err
	}
	if quoted {
		if b, err = json.Marshal(string(b)); err != nil {
			return nil, err
		}
	}
	return b, nil
}
