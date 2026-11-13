package structtag

import (
	"encoding"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"
)

// Decode parses raw into dst (settable) with cfgkit's rules for a default
// tag, so a default means the same thing to cfgkit, to the generated file and
// to the schema:
//
//   - time.Duration: time.ParseDuration ("30s"), never a bare number;
//   - time.Time: RFC 3339;
//   - a TextUnmarshaler (or BinaryUnmarshaler, for url.URL): its own method;
//   - string, bool (strconv.ParseBool), ints, uints, floats: strconv;
//   - slices: split on delim, items trimmed, "" is an empty slice;
//   - maps: entries split on delim, key from value on the first kvdelim;
//   - pointers: allocated, then the element decoded.
//
// The error names the text and the type, never the field: the caller owns the
// field path.
func Decode(dst reflect.Value, raw, delim, kvdelim string) error {
	switch dst.Type() {
	case durationType:
		d, err := time.ParseDuration(raw)
		if err != nil {
			return fmt.Errorf("%q is not a valid duration", raw)
		}
		dst.SetInt(int64(d))
		return nil
	case timeType:
		t, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			return fmt.Errorf("%q is not a valid RFC3339 time", raw)
		}
		dst.Set(reflect.ValueOf(t))
		return nil
	}
	if u, ok := unmarshalerFor(dst); ok {
		if err := u(raw); err != nil {
			return fmt.Errorf("%q is not a valid %s: %w", raw, dst.Type(), err)
		}
		return nil
	}
	switch dst.Kind() {
	case reflect.String:
		dst.SetString(raw)
		return nil
	case reflect.Bool:
		b, err := strconv.ParseBool(raw)
		if err != nil {
			return fmt.Errorf("%q is not a valid bool", raw)
		}
		dst.SetBool(b)
		return nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		n, err := strconv.ParseInt(raw, 10, dst.Type().Bits())
		if err != nil {
			return fmt.Errorf("%q is not a valid %s", raw, dst.Kind())
		}
		dst.SetInt(n)
		return nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		n, err := strconv.ParseUint(raw, 10, dst.Type().Bits())
		if err != nil {
			return fmt.Errorf("%q is not a valid %s", raw, dst.Kind())
		}
		dst.SetUint(n)
		return nil
	case reflect.Float32, reflect.Float64:
		f, err := strconv.ParseFloat(raw, dst.Type().Bits())
		if err != nil {
			return fmt.Errorf("%q is not a valid %s", raw, dst.Kind())
		}
		dst.SetFloat(f)
		return nil
	case reflect.Slice:
		return decodeSlice(dst, raw, delim, kvdelim)
	case reflect.Map:
		return decodeMap(dst, raw, delim, kvdelim)
	case reflect.Pointer:
		if dst.IsNil() {
			dst.Set(reflect.New(dst.Type().Elem()))
		}
		return Decode(dst.Elem(), raw, delim, kvdelim)
	}
	return fmt.Errorf("a default is not supported for type %s (implement encoding.TextUnmarshaler for it)", dst.Type())
}

func decodeSlice(dst reflect.Value, raw, delim, kvdelim string) error {
	if raw == "" {
		dst.Set(reflect.MakeSlice(dst.Type(), 0, 0))
		return nil
	}
	parts := strings.Split(raw, delim)
	out := reflect.MakeSlice(dst.Type(), len(parts), len(parts))
	for i, p := range parts {
		if err := Decode(out.Index(i), strings.TrimSpace(p), delim, kvdelim); err != nil {
			return fmt.Errorf("element %d: %w", i, err)
		}
	}
	dst.Set(out)
	return nil
}

func decodeMap(dst reflect.Value, raw, delim, kvdelim string) error {
	t := dst.Type()
	out := reflect.MakeMap(t)
	if raw == "" {
		dst.Set(out)
		return nil
	}
	for i, entry := range strings.Split(raw, delim) {
		entry = strings.TrimSpace(entry)
		rawKey, rawVal, ok := strings.Cut(entry, kvdelim)
		if !ok {
			return fmt.Errorf("entry %d (%q) has no %q between key and value", i, entry, kvdelim)
		}
		key := reflect.New(t.Key()).Elem()
		if err := Decode(key, strings.TrimSpace(rawKey), delim, kvdelim); err != nil {
			return fmt.Errorf("entry %d: key: %w", i, err)
		}
		val := reflect.New(t.Elem()).Elem()
		if err := Decode(val, strings.TrimSpace(rawVal), delim, kvdelim); err != nil {
			return fmt.Errorf("entry %d (key %q): %w", i, rawKey, err)
		}
		out.SetMapIndex(key, val)
	}
	dst.Set(out)
	return nil
}

// unmarshalerFor returns the text (preferred) or binary unmarshal method of v
// or its address, allocating a nil pointer first.
func unmarshalerFor(v reflect.Value) (func(string) error, bool) {
	if v.Kind() == reflect.Pointer && isUnmarshaler(v.Type()) {
		if v.IsNil() {
			v.Set(reflect.New(v.Type().Elem()))
		}
		return unmarshalFn(v), true
	}
	if v.CanAddr() {
		if p := v.Addr(); isUnmarshaler(p.Type()) {
			return unmarshalFn(p), true
		}
	}
	return nil, false
}

func isUnmarshaler(t reflect.Type) bool {
	return t.Implements(textUnmarshalerType) || t.Implements(binaryUnmarshalerTyp)
}

func unmarshalFn(v reflect.Value) func(string) error {
	if u, ok := v.Interface().(encoding.TextUnmarshaler); ok {
		return func(raw string) error { return u.UnmarshalText([]byte(raw)) }
	}
	u := v.Interface().(encoding.BinaryUnmarshaler)
	return func(raw string) error { return u.UnmarshalBinary([]byte(raw)) }
}

// Value parses raw into a new value of type t (see Decode).
func Value(t reflect.Type, raw, delim, kvdelim string) (reflect.Value, error) {
	v := reflect.New(t).Elem()
	if err := Decode(v, raw, delim, kvdelim); err != nil {
		return reflect.Value{}, err
	}
	return v, nil
}

// DefaultValue parses the field's default tag into a value of its type. ok is
// false when the field has no default.
func (f Field) DefaultValue() (v reflect.Value, ok bool, err error) {
	if !f.HasDefault {
		return reflect.Value{}, false, nil
	}
	v, err = Value(f.Type, f.Default, f.Delim, f.KVDelim)
	if err != nil {
		return reflect.Value{}, true, fmt.Errorf("default %q: %w", f.Default, err)
	}
	return v, true, nil
}

// EnumValues parses every enum tag entry into a value of the field's type
// (pointers removed).
func (f Field) EnumValues() ([]reflect.Value, error) {
	t := f.Type
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	out := make([]reflect.Value, 0, len(f.Enum))
	for _, e := range f.Enum {
		v, err := Value(t, e, f.Delim, f.KVDelim)
		if err != nil {
			return nil, fmt.Errorf("enum %q: %w", e, err)
		}
		out = append(out, v)
	}
	return out, nil
}
