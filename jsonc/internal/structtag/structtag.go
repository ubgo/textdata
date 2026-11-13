// Package structtag reads the struct tags a config struct carries, so the
// generator (jsonc/generate) and the schema builder (jsonc/schema) see the same
// fields, names, defaults and rules.
//
// The tags are the ones github.com/ubgo/cfgkit reads, so one struct drives the
// environment, the JSONC file, its comments and its schema:
//
//	json:"name,omitempty,string"  the key, exactly as encoding/json reads it
//	doc:"text"                    the field's description
//	default:"v"                   a default, parsed like cfgkit parses it
//	env:"KEY,required,notempty"   cfgkit's flat key; only the options matter here
//	required:"true"               required without an env tag
//	enum:"a,b,c"                  the allowed values (split on delim, default ",")
//	secret:"true"                 never render a value for it
//	delim:";" kvdelim:"="         separators for slice and map defaults
//
// Key names follow encoding/json exactly (embedded structs flattened, the
// dominant-field rule on name clashes, "-" skipped), because the file a
// generator writes is read back with encoding/json; any other rule would make
// a generated key that decoding ignores.
package structtag

import (
	"reflect"
	"strings"
)

// Tag names.
const (
	TagJSON     = "json"
	TagEnv      = "env"
	TagDefault  = "default"
	TagDoc      = "doc"
	TagEnum     = "enum"
	TagSecret   = "secret"
	TagRequired = "required"
	TagDelim    = "delim"
	TagKVDelim  = "kvdelim"
)

// Tag options and values.
const (
	optRequired  = "required"
	optNotEmpty  = "notempty"
	optOmitEmpty = "omitempty"
	optString    = "string"
	skipName     = "-"
	tagTrue      = "true"
)

// Default separators, the same as cfgkit's.
const (
	DefaultDelim   = ","
	DefaultKVDelim = ":"
)

// Field is one JSON-visible field of a struct.
type Field struct {
	// Name is the JSON key.
	Name string
	// GoName is the Go field name, for error messages.
	GoName string
	// Index is the reflect index path from the struct (more than one step for
	// a field promoted from an embedded struct).
	Index []int
	// Type is the field's declared type.
	Type reflect.Type

	Doc        string
	Default    string
	HasDefault bool
	// Enum is the raw allowed values, split on Delim.
	Enum []string

	// Required: env ",required" or ",notempty", or required:"true".
	Required bool
	// NotEmpty: env ",notempty"; implies Required.
	NotEmpty bool
	Secret   bool
	// OmitEmpty and Quoted are the json tag's ",omitempty" and ",string".
	OmitEmpty bool
	Quoted    bool

	Delim   string
	KVDelim string

	depth  int
	tagged bool
}

// Fields returns the JSON-visible fields of struct type t in encoding/json's
// order. t may be a pointer to a struct; any other type has no fields.
func Fields(t reflect.Type) []Field {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return nil
	}
	var all []Field
	collect(t, nil, 0, map[reflect.Type]bool{}, &all)
	return dominant(all)
}

// collect walks t depth first in field order, which is encoding/json's index
// order, flattening untagged embedded structs.
func collect(t reflect.Type, index []int, depth int, onStack map[reflect.Type]bool, out *[]Field) {
	if onStack[t] {
		return
	}
	onStack[t] = true
	defer delete(onStack, t)
	for i := range t.NumField() {
		sf := t.Field(i)
		ft := sf.Type
		if ft.Kind() == reflect.Pointer {
			ft = ft.Elem()
		}
		if sf.Anonymous {
			if !sf.IsExported() && ft.Kind() != reflect.Struct {
				continue
			}
		} else if !sf.IsExported() {
			continue
		}
		jsonTag := sf.Tag.Get(TagJSON)
		if jsonTag == skipName {
			continue
		}
		name, opts := split(jsonTag)
		idx := append(append([]int(nil), index...), i)
		if name == "" && sf.Anonymous && ft.Kind() == reflect.Struct {
			collect(ft, idx, depth+1, onStack, out)
			continue
		}
		f := newField(sf, name, opts)
		f.Index = idx
		f.depth = depth
		*out = append(*out, f)
	}
}

// dominant applies encoding/json's rule for clashing names: the shallowest
// field wins; at equal depth a single tagged field wins; otherwise the name is
// dropped altogether.
func dominant(all []Field) []Field {
	byName := map[string][]int{}
	for i, f := range all {
		byName[f.Name] = append(byName[f.Name], i)
	}
	keep := make([]bool, len(all))
	for _, idxs := range byName {
		minDepth := all[idxs[0]].depth
		for _, i := range idxs {
			minDepth = min(minDepth, all[i].depth)
		}
		var top, tagged []int
		for _, i := range idxs {
			if all[i].depth == minDepth {
				top = append(top, i)
				if all[i].tagged {
					tagged = append(tagged, i)
				}
			}
		}
		switch {
		case len(top) == 1:
			keep[top[0]] = true
		case len(tagged) == 1:
			keep[tagged[0]] = true
		}
	}
	var out []Field
	for i, f := range all {
		if keep[i] {
			out = append(out, f)
		}
	}
	return out
}

func newField(sf reflect.StructField, name string, jsonOpts []string) Field {
	f := Field{
		Name:      name,
		GoName:    sf.Name,
		Type:      sf.Type,
		Doc:       sf.Tag.Get(TagDoc),
		Secret:    sf.Tag.Get(TagSecret) == tagTrue,
		OmitEmpty: has(jsonOpts, optOmitEmpty),
		Quoted:    has(jsonOpts, optString),
		Delim:     sf.Tag.Get(TagDelim),
		KVDelim:   sf.Tag.Get(TagKVDelim),
		tagged:    name != "",
	}
	if f.Name == "" {
		f.Name = sf.Name
	}
	if f.Delim == "" {
		f.Delim = DefaultDelim
	}
	if f.KVDelim == "" {
		f.KVDelim = DefaultKVDelim
	}
	if def, ok := sf.Tag.Lookup(TagDefault); ok {
		f.Default, f.HasDefault = def, true
	}
	if env, ok := sf.Tag.Lookup(TagEnv); ok {
		_, envOpts := split(env)
		f.NotEmpty = has(envOpts, optNotEmpty)
		f.Required = f.NotEmpty || has(envOpts, optRequired)
	}
	if sf.Tag.Get(TagRequired) == tagTrue {
		f.Required = true
	}
	if enum, ok := sf.Tag.Lookup(TagEnum); ok && enum != "" {
		for _, e := range strings.Split(enum, f.Delim) {
			f.Enum = append(f.Enum, strings.TrimSpace(e))
		}
	}
	return f
}

// split separates a tag's name from its comma-separated options.
func split(tag string) (string, []string) {
	parts := strings.Split(tag, ",")
	return parts[0], parts[1:]
}

func has(opts []string, name string) bool {
	for _, o := range opts {
		if strings.TrimSpace(o) == name {
			return true
		}
	}
	return false
}

// ByIndex returns the field at index inside struct value v. A nil embedded
// pointer on the way is read as a zero struct (encoding/json would allocate
// it on decode), so the result is always usable for reading. ok is false when
// the value cannot be read (it sits behind an unexported field).
func ByIndex(v reflect.Value, index []int) (reflect.Value, bool) {
	for i, x := range index {
		if i > 0 {
			for v.Kind() == reflect.Pointer {
				if v.IsNil() {
					v = reflect.New(v.Type().Elem()).Elem()
					continue
				}
				v = v.Elem()
			}
		}
		v = v.Field(x)
	}
	return v, v.CanInterface()
}
