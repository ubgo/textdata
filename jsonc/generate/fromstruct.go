package generate

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/ubgo/textdata/jsonc"
	"github.com/ubgo/textdata/jsonc/internal/structtag"
)

// Notes FromStruct appends to a field's doc comment.
const (
	noteRequired = "Required."
	noteNotEmpty = "Required, must not be empty."
	noteSecret   = "Secret: do not commit a real value."
	noteOneOf    = "One of: "
	noteDuration = "Duration in nanoseconds."
)

// FromStruct builds a commented file from a config struct (or a pointer to
// one), using the tags cfgkit reads (see the package README for the list):
//
//   - keys come from `json` tags with encoding/json's rules: embedded structs
//     are flattened, unexported fields and json:"-" are skipped;
//   - `doc` becomes the comment above the key, followed by notes for
//     required, notempty, secret and enum fields;
//   - each value is v's own value, or, when that is the zero value, the
//     field's `default` tag parsed with cfgkit's rules for its type. A default
//     that does not parse is an error naming the field, even when v sets it;
//   - nested structs (and nil pointers to structs) become nested objects;
//     nil maps and slices become {} and [];
//   - a secret field always gets its zero value, never v's or the default;
//   - time.Duration is written as encoding/json writes it, an integer of
//     nanoseconds, with the readable form ("30s") as a trailing comment.
//
// Decoding the result with encoding/json (doc.Decode) yields v with the
// defaults filled in. A struct implementing cfgkit's Defaulter is not called:
// call Defaults on v before passing it.
func FromStruct(v any, opts ...Option) (*File, error) {
	rv := reflect.ValueOf(v)
	if !rv.IsValid() {
		return nil, fmt.Errorf("generate: FromStruct needs a struct, got nil")
	}
	for rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			rv = reflect.New(rv.Type().Elem())
		}
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Struct {
		return nil, fmt.Errorf("generate: FromStruct needs a struct, got %s", rv.Type())
	}
	f := New(opts...)
	g := &structGen{onStack: map[reflect.Type]bool{}}
	if err := g.fill(f.Root(), rv, ""); err != nil {
		return nil, err
	}
	return f, nil
}

// structGen walks a struct value into an Object.
type structGen struct {
	// onStack holds the struct types being walked, so a nil pointer back to
	// one of them (a recursive type) is written as null instead of forever.
	onStack map[reflect.Type]bool
}

func (g *structGen) fill(o *Object, sv reflect.Value, goPath string) error {
	t := sv.Type()
	g.onStack[t] = true
	defer delete(g.onStack, t)
	for _, fd := range structtag.Fields(t) {
		fieldPath := fd.GoName
		if goPath != "" {
			fieldPath = goPath + "." + fd.GoName
		}
		fv, ok := structtag.ByIndex(sv, fd.Index)
		if !ok {
			continue
		}
		shape, elem, _ := structtag.ShapeOf(fd.Type)
		if shape == structtag.ShapeUnsupported {
			return fmt.Errorf("generate: field %s: type %s cannot be written as JSON", fieldPath, fd.Type)
		}
		if _, err := fd.EnumValues(); err != nil {
			return fmt.Errorf("generate: field %s: %w", fieldPath, err)
		}
		def, hasDef, err := fd.DefaultValue()
		if err != nil {
			return fmt.Errorf("generate: field %s: %w", fieldPath, err)
		}
		text := comment(fd)
		itemOpts := []ItemOption{Doc(text)}
		// A blank line sets off every documented member and every section,
		// except at the top of its object.
		if len(o.entries) > 0 && (text != "" || (shape == structtag.ShapeStruct && !fd.Quoted && !fd.Secret)) {
			itemOpts = append(itemOpts, BlankBefore())
		}

		if shape == structtag.ShapeStruct && !fd.Quoted && !fd.Secret {
			inner := fv
			for inner.Kind() == reflect.Pointer {
				if inner.IsNil() {
					inner = reflect.Value{}
					break
				}
				inner = inner.Elem()
			}
			if !inner.IsValid() {
				if g.onStack[elem] {
					o.Set(fd.Name, nil, itemOpts...)
					continue
				}
				inner = reflect.New(elem).Elem()
			}
			if err := g.fill(o.Object(fd.Name, itemOpts...), inner, fieldPath); err != nil {
				return err
			}
			continue
		}

		val := fv
		switch {
		case fd.Secret:
			val = reflect.Zero(fd.Type)
		case hasDef && fv.IsZero():
			val = def
		}
		val = emptyContainer(val)
		var x any = val.Interface()
		if fd.Quoted {
			if x, err = quoted(val); err != nil {
				return fmt.Errorf("generate: field %s: %w", fieldPath, err)
			}
		}
		if shape == structtag.ShapeDuration {
			if d, ok := duration(val); ok {
				itemOpts = append(itemOpts, Trailing(d.String()))
			}
		}
		o.Set(fd.Name, x, itemOpts...)
	}
	return nil
}

// comment is a field's doc text plus one line of notes.
func comment(fd structtag.Field) string {
	var notes []string
	switch {
	case fd.NotEmpty:
		notes = append(notes, noteNotEmpty)
	case fd.Required:
		notes = append(notes, noteRequired)
	}
	if len(fd.Enum) > 0 {
		notes = append(notes, noteOneOf+strings.Join(fd.Enum, ", ")+".")
	}
	if shape, _, _ := structtag.ShapeOf(fd.Type); shape == structtag.ShapeDuration {
		notes = append(notes, noteDuration)
	}
	if fd.Secret {
		notes = append(notes, noteSecret)
	}
	lines := []string{}
	if fd.Doc != "" {
		lines = append(lines, fd.Doc)
	}
	if len(notes) > 0 {
		lines = append(lines, strings.Join(notes, " "))
	}
	return strings.Join(lines, "\n")
}

// emptyContainer turns a nil map or slice (behind any pointers) into an
// empty one, so a template shows {} or [] rather than null.
func emptyContainer(v reflect.Value) reflect.Value {
	switch v.Kind() {
	case reflect.Map:
		if v.IsNil() {
			return reflect.MakeMap(v.Type())
		}
	case reflect.Slice:
		if v.IsNil() {
			return reflect.MakeSlice(v.Type(), 0, 0)
		}
	}
	return v
}

// quoted writes a scalar the way encoding/json writes a field tagged
// ",string": its JSON text inside a JSON string.
func quoted(v reflect.Value) (any, error) {
	for v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return nil, nil
		}
		v = v.Elem()
	}
	switch v.Kind() {
	case reflect.String, reflect.Bool,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr,
		reflect.Float32, reflect.Float64:
	default:
		return v.Interface(), nil
	}
	inner, err := json.Marshal(v.Interface())
	if err != nil {
		return nil, err
	}
	outer, err := marshal(string(inner), "", "")
	if err != nil {
		return nil, err
	}
	return jsonc.Raw(outer), nil
}

func duration(v reflect.Value) (time.Duration, bool) {
	for v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return 0, false
		}
		v = v.Elem()
	}
	return time.Duration(v.Int()), true
}
