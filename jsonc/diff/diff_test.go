package diff

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/ubgo/textdata/jsonc"
	"github.com/ubgo/textdata/path"
	"github.com/ubgo/textdata/pos"
	"github.com/ubgo/textdata/texttest"
)

func mustParse(t testing.TB, src string) *jsonc.Doc {
	t.Helper()
	d, err := jsonc.Parse([]byte(src))
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	return d
}

// brief renders a change as "kind path old -> new" with compact JSON values.
func brief(c Change) string {
	enc := func(v any) string {
		b, err := json.Marshal(v)
		if err != nil {
			return "!" + err.Error()
		}
		return string(b)
	}
	switch c.Kind {
	case Added:
		return fmt.Sprintf("+ %s %s", c.Path, enc(c.New))
	case Removed:
		return fmt.Sprintf("- %s %s", c.Path, enc(c.Old))
	default:
		return fmt.Sprintf("~ %s %s -> %s", c.Path, enc(c.Old), enc(c.New))
	}
}

func TestDiff(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		a, b string
		want []string
	}{
		{"identical", `{"a":1}`, `{"a":1}`, nil},
		{"comments and layout ignored", "{\n  // port\n  \"a\": 1, /* x */\n  \"b\": [1,2,],\n}", `{"b":[1,2],"a":1}`, nil},
		{"number spelling ignored", `{"a":1,"b":1.5,"c":-0}`, `{"a":1.0,"b":15e-1,"c":0}`, nil},
		{"big integers exact", `{"id":9007199254740993}`, `{"id":9007199254740992}`, []string{"~ id 9007199254740993 -> 9007199254740992"}},
		{"added", `{"a":1}`, `{"a":1,"b":{"c":true}}`, []string{`+ b {"c":true}`}},
		{"removed", `{"a":1,"b":null}`, `{"a":1}`, []string{"- b null"}},
		{"changed scalar", `{"a":"x"}`, `{"a":"y"}`, []string{`~ a "x" -> "y"`}},
		{"null vs missing", `{"a":null}`, `{}`, []string{"- a null"}},
		{"kind change is one change", `{"a":{"b":1}}`, `{"a":[1]}`, []string{`~ a {"b":1} -> [1]`}},
		{"string vs number", `{"a":"1"}`, `{"a":1}`, []string{`~ a "1" -> 1`}},
		{"nested", `{"s":{"p":1,"q":2}}`, `{"s":{"p":1,"q":3,"r":4}}`, []string{"~ s.q 2 -> 3", "+ s.r 4"}},
		{"arrays by index", `{"l":[1,2,3]}`, `{"l":[1,5]}`, []string{"~ l[1] 2 -> 5", "- l[2] 3"}},
		{"array grows", `[1]`, `[1,{"x":1}]`, []string{`+ [1] {"x":1}`}},
		{"keys sorted", `{"b":1,"a":1}`, `{"b":2,"a":2,"c":0}`, []string{"~ a 1 -> 2", "~ b 1 -> 2", "+ c 0"}},
		{"quoted path", `{"a.b":{"":1}}`, `{"a.b":{"":2}}`, []string{`~ "a.b"."" 1 -> 2`}},
		{"root scalar", `1`, `2`, []string{"~  1 -> 2"}},
		{"empty doc is null", ``, `{}`, []string{"~  null -> {}"}},
		{"duplicate keys last wins", `{"a":1,"a":2}`, `{"a":2}`, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := Diff(mustParse(t, tt.a), mustParse(t, tt.b))
			if err != nil {
				t.Fatal(err)
			}
			var lines []string
			for _, c := range got {
				lines = append(lines, brief(c))
			}
			if !reflect.DeepEqual(lines, tt.want) {
				t.Errorf("Diff =\n%q\nwant\n%q", lines, tt.want)
			}
			if got == nil {
				t.Error("Diff returned a nil slice; want empty")
			}
		})
	}
}

func TestPositionsAndPointer(t *testing.T) {
	t.Parallel()
	a := mustParse(t, "{\n  \"keep\": 1,\n  \"gone\": 2,\n  \"mod\": [true]\n}")
	b := mustParse(t, "{\"keep\":1,\"mod\":[false],\n\"/new\":3}")
	got, err := Diff(a, b)
	if err != nil {
		t.Fatal(err)
	}
	want := []Change{
		{Kind: Added, Path: `"/new"`, Pointer: "/~1new", New: json.Number("3"), NewPos: pos.Pos{Offset: 32, Line: 2, Col: 8}},
		{Kind: Removed, Path: "gone", Pointer: "/gone", Old: json.Number("2"), OldPos: pos.Pos{Offset: 25, Line: 3, Col: 11}},
		{Kind: Changed, Path: "mod[0]", Pointer: "/mod/0", Old: true, New: false, OldPos: pos.Pos{Offset: 38, Line: 4, Col: 11}, NewPos: pos.Pos{Offset: 17, Line: 1, Col: 18}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Diff =\n%+v\nwant\n%+v", got, want)
	}
	// The pointer always resolves on the side the value is on.
	for _, c := range got {
		if c.Kind != Added {
			if _, err := a.Get(c.Pointer); err != nil {
				t.Errorf("a.Get(%q): %v", c.Pointer, err)
			}
		}
	}
}

func TestErrors(t *testing.T) {
	t.Parallel()
	ok := mustParse(t, `{}`)
	if _, err := Diff(nil, ok); !errors.Is(err, ErrNilDoc) {
		t.Errorf("nil a: %v", err)
	}
	if _, err := Diff(ok, nil); !errors.Is(err, ErrNilDoc) {
		t.Errorf("nil b: %v", err)
	}
	bad, _ := jsonc.Parse([]byte(`{"a":`))
	if _, err := Diff(ok, bad); !errors.Is(err, jsonc.ErrInvalid) {
		t.Errorf("invalid b: %v", err)
	}
	if _, err := Diff(bad, ok); !errors.Is(err, jsonc.ErrInvalid) {
		t.Errorf("invalid a: %v", err)
	}
}

func TestEqual(t *testing.T) {
	t.Parallel()
	tests := []struct {
		x, y any
		want bool
	}{
		{json.Number("1"), json.Number("1.0"), true},
		{json.Number("1e400"), json.Number("1e400"), true},
		{json.Number("1e400"), json.Number("2e400"), false},
		{json.Number("12345678901234567890"), json.Number("12345678901234567891"), false},
		{json.Number("1"), "1", false},
		{nil, nil, true},
		{nil, false, false},
		{map[string]any{"a": nil}, map[string]any{"b": nil}, false},
		{[]any{1}, []any{json.Number("1")}, false}, // an int is not a decoded value
	}
	for _, tt := range tests {
		if got := Equal(tt.x, tt.y); got != tt.want {
			t.Errorf("Equal(%#v, %#v) = %v, want %v", tt.x, tt.y, got, tt.want)
		}
	}
}

// apply replays changes on a decoded value. Array removals are trailing
// (Diff compares by index), so they are applied from the highest index down.
func apply(t testing.TB, root any, changes []Change) any {
	t.Helper()
	var removals []Change
	for _, c := range changes {
		if c.Kind == Removed {
			if _, last, ok := mustPath(t, c.Path).Parent(); ok && last.Kind == path.KindIndex {
				removals = append([]Change{c}, removals...)
				continue
			}
		}
		root = applyOne(t, root, c)
	}
	for _, c := range removals {
		root = applyOne(t, root, c)
	}
	return root
}

func mustPath(t testing.TB, s string) path.Path {
	t.Helper()
	p, err := path.ParsePointer(s)
	if err == nil && s != "" {
		return p
	}
	p, err = path.Parse(s)
	if err != nil {
		t.Fatalf("path %q: %v", s, err)
	}
	return p
}

func applyOne(t testing.TB, root any, c Change) any {
	t.Helper()
	p, err := path.ParsePointer(c.Pointer)
	if err != nil {
		t.Fatalf("pointer %q: %v", c.Pointer, err)
	}
	if len(p) == 0 {
		return c.New
	}
	parentPath, last, _ := p.Parent()
	parent := root
	for _, seg := range parentPath {
		switch v := parent.(type) {
		case map[string]any:
			parent = v[seg.Key]
		case []any:
			parent = v[seg.Index]
		default:
			t.Fatalf("%s: walk through %T", c.Pointer, parent)
		}
	}
	set := func(v any) {
		// Replace the container in its own parent (slices may reallocate).
		root = replaceAt(t, root, parentPath, v)
	}
	switch v := parent.(type) {
	case map[string]any:
		if c.Kind == Removed {
			delete(v, last.Key)
		} else {
			v[last.Key] = c.New
		}
	case []any:
		switch c.Kind {
		case Removed:
			if last.Index != len(v)-1 {
				t.Fatalf("%s: removal not at the end (len %d)", c.Pointer, len(v))
			}
			set(v[:last.Index])
		case Added:
			if last.Index != len(v) {
				t.Fatalf("%s: addition not at the end (len %d)", c.Pointer, len(v))
			}
			set(append(v, c.New))
		default:
			v[last.Index] = c.New
		}
	default:
		t.Fatalf("%s: parent is %T", c.Pointer, parent)
	}
	return root
}

func replaceAt(t testing.TB, root any, p path.Path, v any) any {
	if len(p) == 0 {
		return v
	}
	switch c := root.(type) {
	case map[string]any:
		c[p[0].Key] = replaceAt(t, c[p[0].Key], p[1:], v)
	case []any:
		c[p[0].Index] = replaceAt(t, c[p[0].Index], p[1:], v)
	}
	return root
}

func decodeT(t testing.TB, d *jsonc.Doc) any {
	t.Helper()
	v, err := decode(d)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

// checkApplies is the property: replaying Diff(a, b) on a's decoded value
// yields b's, and Diff(b, b) is empty.
func checkApplies(t testing.TB, a, b *jsonc.Doc) {
	t.Helper()
	changes, err := Diff(a, b)
	if err != nil {
		t.Fatal(err)
	}
	got := apply(t, decodeT(t, a), changes)
	if want := decodeT(t, b); !Equal(got, want) {
		t.Fatalf("apply(a, Diff(a,b)) = %#v, want %#v (changes %+v)", got, want, changes)
	}
	again, err := Diff(b, b)
	if err != nil || len(again) != 0 {
		t.Fatalf("Diff(b, b) = %+v, %v", again, err)
	}
}

var propertyDocs = []string{
	``, `null`, `1`, `"s"`, `[]`, `{}`, `[1,2,3]`, `[3,2]`, `[[1],[2,[3]]]`, `[[1],[2]]`,
	`{"a":1}`, `{"a":{"b":[1,{"c":2}]}}`, `{"a":{"b":[1,{"c":3},4]}}`, `{"a":[],"x":null}`,
	"{\n  // c\n  \"a\": {\"b\": [1, {\"c\": 2,},],},\n}", `{"/k":{"~":1},"a.b":[true]}`, `{"/k":{"~":2}}`,
	`{"n":9007199254740993}`, `{"n":9007199254740992.0}`,
}

func TestApplyProperty(t *testing.T) {
	t.Parallel()
	for _, x := range propertyDocs {
		for _, y := range propertyDocs {
			checkApplies(t, mustParse(t, x), mustParse(t, y))
		}
	}
}

func FuzzDiff(f *testing.F) {
	for _, s := range texttest.Corpus {
		f.Add([]byte(s), []byte(`{}`))
	}
	for i, x := range propertyDocs {
		f.Add([]byte(x), []byte(propertyDocs[(i+3)%len(propertyDocs)]))
	}
	f.Fuzz(func(t *testing.T, x, y []byte) {
		a, errA := jsonc.Parse(x)
		b, errB := jsonc.Parse(y)
		changes, err := Diff(a, b)
		if errA != nil || errB != nil {
			if err == nil {
				t.Fatalf("Diff accepted an invalid document")
			}
			return
		}
		if err != nil {
			t.Fatalf("Diff: %v", err)
		}
		if self, err := Diff(a, a); err != nil || len(self) != 0 {
			t.Fatalf("Diff(x, x) = %+v, %v", self, err)
		}
		_ = changes
		checkApplies(t, a, b)
		// Diff never mutates its inputs.
		if !bytes.Equal(a.Render(), x) || !bytes.Equal(b.Render(), y) {
			t.Fatal("Diff changed a document")
		}
	})
}
