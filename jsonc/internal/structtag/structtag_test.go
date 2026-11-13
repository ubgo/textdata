package structtag

import (
	"encoding/json"
	"net/url"
	"reflect"
	"sort"
	"testing"
	"time"
)

type E1 struct {
	A string
	B string `json:"b"`
}

type E2 struct {
	A string
	B string
	C string
}

type e3 struct {
	D string `json:"d"`
}

type deep struct{ E1 }

type sample struct {
	E1
	*E2
	e3
	deep
	A      string `json:"-"`
	X      int    `json:"x,omitempty,string" doc:"x doc" default:"7" env:"X,notempty" enum:"1;7" delim:";"`
	Y      int    `env:"Y,required"`
	Z      int    `required:"true" secret:"true"`
	hidden int
}

func keys(fs []Field) []string {
	out := []string{}
	for _, f := range fs {
		out = append(out, f.Name)
	}
	return out
}

// TestFieldsMatchEncodingJSON: the visible keys are exactly the ones
// encoding/json writes.
func TestFieldsMatchEncodingJSON(t *testing.T) {
	t.Parallel()
	v := sample{E2: &E2{}, X: 1} // X is omitempty
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	want := []string{}
	for k := range m {
		want = append(want, k)
	}
	got := keys(Fields(reflect.TypeOf(v)))
	sort.Strings(want)
	sorted := append([]string(nil), got...)
	sort.Strings(sorted)
	if !reflect.DeepEqual(sorted, want) {
		t.Fatalf("keys %v, encoding/json writes %v", got, want)
	}
}

func TestFieldTags(t *testing.T) {
	t.Parallel()
	byName := map[string]Field{}
	for _, f := range Fields(reflect.TypeOf(&sample{})) {
		byName[f.Name] = f
	}
	x := byName["x"]
	if x.Doc != "x doc" || !x.HasDefault || x.Default != "7" || !x.NotEmpty || !x.Required || !x.OmitEmpty || !x.Quoted || !reflect.DeepEqual(x.Enum, []string{"1", "7"}) {
		t.Fatalf("x: %+v", x)
	}
	if y := byName["Y"]; !y.Required || y.NotEmpty {
		t.Fatalf("Y: %+v", y)
	}
	if z := byName["Z"]; !z.Required || !z.Secret {
		t.Fatalf("Z: %+v", z)
	}
	if vals, err := x.EnumValues(); err != nil || vals[1].Int() != 7 {
		t.Fatalf("EnumValues: %v %v", vals, err)
	}
	if Fields(reflect.TypeOf(1)) != nil {
		t.Fatal("a non-struct has no fields")
	}
}

type text struct{ v string }

func (t *text) UnmarshalText(b []byte) error { t.v = "T:" + string(b); return nil }

func TestDecode(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		raw  string
		into any
		want any
	}{
		{"30s", time.Duration(0), 30 * time.Second},
		{"2026-01-02T03:04:05Z", time.Time{}, time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)},
		{"x", text{}, text{v: "T:x"}},
		{"s", "", "s"},
		{"true", false, true},
		{"-8", int8(0), int8(-8)},
		{"8", uint(0), uint(8)},
		{"1.5", float32(0), float32(1.5)},
		{"a, b", []string(nil), []string{"a", "b"}},
		{"", []int(nil), []int{}},
		{"a:1, b:2", map[string]int(nil), map[string]int{"a": 1, "b": 2}},
		{"u:http://h:1/x", map[string]string(nil), map[string]string{"u": "http://h:1/x"}},
	} {
		v := reflect.New(reflect.TypeOf(tc.into)).Elem()
		if err := Decode(v, tc.raw, DefaultDelim, DefaultKVDelim); err != nil {
			t.Fatalf("Decode(%q, %T): %v", tc.raw, tc.into, err)
		}
		if !reflect.DeepEqual(v.Interface(), tc.want) {
			t.Fatalf("Decode(%q, %T) = %#v, want %#v", tc.raw, tc.into, v.Interface(), tc.want)
		}
	}
	p := reflect.New(reflect.TypeOf((*int)(nil))).Elem()
	if err := Decode(p, "5", ",", ":"); err != nil || *(p.Interface().(*int)) != 5 {
		t.Fatalf("pointer: %v", err)
	}
	u := reflect.New(reflect.TypeOf(url.URL{})).Elem()
	if err := Decode(u, "https://example.com/a", ",", ":"); err != nil || u.Interface().(url.URL).Host != "example.com" {
		t.Fatalf("binary unmarshaler: %v", err)
	}
	for _, bad := range []struct {
		raw  string
		into any
	}{
		{"soon", time.Duration(0)}, {"2026-01-02", time.Time{}}, {"maybe", false}, {"300", int8(0)},
		{"-1", uint(0)}, {"x", 0.0}, {"1,x", []int{}}, {"novalue", map[string]int{}}, {"a:x", map[string]int{}},
		{"x:1", map[int]int{}}, {"x", make(chan int)},
	} {
		v := reflect.New(reflect.TypeOf(bad.into)).Elem()
		if err := Decode(v, bad.raw, ",", ":"); err == nil {
			t.Errorf("Decode(%q, %T) must fail", bad.raw, bad.into)
		}
	}
}

func TestShapeOf(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		v    any
		want Shape
		null bool
	}{
		{time.Second, ShapeDuration, false},
		{time.Time{}, ShapeTime, false},
		{json.RawMessage(nil), ShapeOpaque, false},
		{url.URL{}, ShapeOpaque, false},
		{text{}, ShapeOpaque, false},
		{E1{}, ShapeStruct, false},
		{&E1{}, ShapeStruct, true},
		{map[string]int{}, ShapeMap, false},
		{map[int]int{}, ShapeMap, false},
		{map[[2]int]int{}, ShapeUnsupported, false},
		{[]byte{}, ShapeBytes, false},
		{[2]byte{}, ShapeSlice, false},
		{[]int{}, ShapeSlice, false},
		{"", ShapeString, false},
		{true, ShapeBool, false},
		{int16(0), ShapeInt, false},
		{uint8(0), ShapeUint, false},
		{0.0, ShapeFloat, false},
		{complex(1, 1), ShapeUnsupported, false},
		{make(chan int), ShapeUnsupported, false},
	} {
		got, _, null := ShapeOf(reflect.TypeOf(tc.v))
		if got != tc.want || null != tc.null {
			t.Errorf("ShapeOf(%T) = %v %v, want %v %v", tc.v, got, null, tc.want, tc.null)
		}
	}
	var a any
	if got, _, _ := ShapeOf(reflect.TypeOf(&a).Elem()); got != ShapeAny {
		t.Errorf("interface: %v", got)
	}
}

func TestByIndexNilEmbedded(t *testing.T) {
	t.Parallel()
	v := reflect.ValueOf(sample{})
	for _, f := range Fields(v.Type()) {
		if f.Name == "C" {
			got, ok := ByIndex(v, f.Index)
			if !ok || got.String() != "" {
				t.Fatalf("a field behind a nil embedded pointer reads as zero: %v %v", got, ok)
			}
			return
		}
	}
	t.Fatal("C not found")
}
