package structtag

import (
	"encoding"
	"encoding/json"
	"reflect"
	"time"
)

// Shape is how a Go type appears in JSON, as encoding/json writes it.
type Shape int

// The shapes.
const (
	// ShapeUnsupported: encoding/json cannot write the type (chan, func,
	// complex, a map with a non-string-like key).
	ShapeUnsupported Shape = iota
	// ShapeStruct: a struct walked field by field (an object).
	ShapeStruct
	// ShapeMap: a map (an object with free keys).
	ShapeMap
	// ShapeSlice: a slice or array (an array).
	ShapeSlice
	// ShapeBytes: []byte (a base64 string).
	ShapeBytes
	ShapeString
	ShapeBool
	ShapeInt
	ShapeUint
	ShapeFloat
	// ShapeDuration: time.Duration, an integer count of nanoseconds in JSON.
	ShapeDuration
	// ShapeTime: time.Time, an RFC 3339 string.
	ShapeTime
	// ShapeText: a type with MarshalText, written as a string.
	ShapeText
	// ShapeOpaque: a type that writes its own JSON (MarshalJSON) or owns its
	// text form only on input; any JSON value may be valid.
	ShapeOpaque
	// ShapeAny: an interface; any JSON value.
	ShapeAny
)

var (
	durationType         = reflect.TypeOf(time.Duration(0))
	timeType             = reflect.TypeOf(time.Time{})
	jsonMarshalerType    = reflect.TypeOf((*json.Marshaler)(nil)).Elem()
	jsonUnmarshalerType  = reflect.TypeOf((*json.Unmarshaler)(nil)).Elem()
	textMarshalerType    = reflect.TypeOf((*encoding.TextMarshaler)(nil)).Elem()
	textUnmarshalerType  = reflect.TypeOf((*encoding.TextUnmarshaler)(nil)).Elem()
	binaryUnmarshalerTyp = reflect.TypeOf((*encoding.BinaryUnmarshaler)(nil)).Elem()
)

// ShapeOf returns t's shape, t with every pointer removed, and whether t was
// a pointer (JSON null is then a valid value).
func ShapeOf(t reflect.Type) (shape Shape, elem reflect.Type, nullable bool) {
	for t.Kind() == reflect.Pointer {
		t, nullable = t.Elem(), true
	}
	return shapeOf(t), t, nullable
}

func implements(t, iface reflect.Type) bool {
	return t.Implements(iface) || reflect.PointerTo(t).Implements(iface)
}

func shapeOf(t reflect.Type) Shape {
	switch t {
	case durationType:
		return ShapeDuration
	case timeType:
		return ShapeTime
	}
	if implements(t, jsonMarshalerType) {
		return ShapeOpaque
	}
	if implements(t, textMarshalerType) {
		return ShapeText
	}
	switch t.Kind() {
	case reflect.Struct:
		// A struct that decodes itself is a leaf: walking into it would
		// describe fields its own decoder does not read.
		if implements(t, jsonUnmarshalerType) || implements(t, textUnmarshalerType) || implements(t, binaryUnmarshalerTyp) {
			return ShapeOpaque
		}
		return ShapeStruct
	case reflect.Map:
		switch t.Key().Kind() {
		case reflect.String, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
			reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
			return ShapeMap
		}
		if implements(t.Key(), textMarshalerType) {
			return ShapeMap
		}
		return ShapeUnsupported
	case reflect.Slice:
		if t.Elem().Kind() == reflect.Uint8 && !implements(t.Elem(), jsonMarshalerType) && !implements(t.Elem(), textMarshalerType) {
			return ShapeBytes
		}
		return ShapeSlice
	case reflect.Array:
		return ShapeSlice
	case reflect.String:
		return ShapeString
	case reflect.Bool:
		return ShapeBool
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return ShapeInt
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return ShapeUint
	case reflect.Float32, reflect.Float64:
		return ShapeFloat
	case reflect.Interface:
		return ShapeAny
	}
	return ShapeUnsupported
}
