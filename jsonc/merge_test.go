package jsonc

import (
	"bytes"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/ubgo/textdata/texttest"
)

// refMerge is the reference deep merge on decoded values: objects merge by
// key, anything else in b replaces a.
func refMerge(a, b any) any {
	am, aok := a.(map[string]any)
	bm, bok := b.(map[string]any)
	if !aok || !bok {
		return b
	}
	out := make(map[string]any, len(am)+len(bm))
	for k, v := range am {
		out[k] = v
	}
	for k, v := range bm {
		if old, ok := out[k]; ok {
			out[k] = refMerge(old, v)
		} else {
			out[k] = v
		}
	}
	return out
}

func decodeDoc(t testing.TB, d *Doc) any {
	t.Helper()
	j, err := d.JSON()
	if err != nil {
		t.Fatalf("JSON: %v", err)
	}
	// UseNumber: a number out of float64 range (1e700) is still valid JSON.
	dec := json.NewDecoder(bytes.NewReader(j))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		t.Fatalf("decode %q: %v", j, err)
	}
	return v
}

// checkMerge merges b into a, then checks the result re-parses, its JSON
// equals the reference merge, and merging b again changes nothing.
func checkMerge(t testing.TB, a, b *Doc) []byte {
	t.Helper()
	var want any
	switch {
	case b.root == nil:
		want = decodeDoc(t, a)
	case a.root == nil:
		want = decodeDoc(t, b)
	default:
		want = refMerge(decodeDoc(t, a), decodeDoc(t, b))
	}
	if err := a.Merge(b); err != nil {
		t.Fatalf("Merge(%q, %q): %v", a.src, b.src, err)
	}
	out := a.Render()
	if _, err := Parse(out, optsOf(a)...); err != nil {
		t.Fatalf("merged output does not re-parse: %v\n%q", err, out)
	}
	if got := decodeDoc(t, a); !reflect.DeepEqual(got, want) {
		t.Fatalf("merged JSON = %#v, want %#v\nout: %q", got, want, out)
	}
	if err := a.Merge(b); err != nil {
		t.Fatalf("second Merge: %v", err)
	}
	if again := a.Render(); string(again) != string(out) {
		t.Fatalf("second Merge changed bytes:\n%q\n%q", out, again)
	}
	return out
}

func optsOf(d *Doc) []Option {
	if d.opts.strict {
		return []Option{Strict()}
	}
	return nil
}

func TestMergeExamples(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, base, over, want string
	}{
		{
			name: "replace scalar keeps comments",
			base: "{\n  // the port\n  \"port\": 8080, // http\n  \"host\": \"a\"\n}\n",
			over: `{"port": 9090}`,
			want: "{\n  // the port\n  \"port\": 9090, // http\n  \"host\": \"a\"\n}\n",
		},
		{
			name: "add key in base style with comment",
			base: "{\n    \"a\": 1,\n}\n",
			over: "{\n  // new one\n  \"b\": {\"x\": [1, 2]}\n}",
			want: "{\n    \"a\": 1,\n    // new one\n    \"b\": {\n        \"x\": [\n            1,\n            2\n        ]\n    },\n}\n",
		},
		{
			name: "nested merge",
			base: "{\n  \"db\": {\n    \"host\": \"h\", // keep\n    \"port\": 1\n  }\n}",
			over: `{"db": {"port": 2, "user": "u"}}`,
			want: "{\n  \"db\": {\n    \"host\": \"h\", // keep\n    \"port\": 2,\n    \"user\": \"u\"\n  }\n}",
		},
		{
			name: "array replaces whole",
			base: `{"list": [1, 2, 3]}`,
			over: `{"list": [4]}`,
			want: `{"list": [4]}`,
		},
		{
			name: "object replaces scalar",
			base: "{\n  \"a\": 1\n}",
			over: `{"a": {"b": "c"}}`,
			want: "{\n  \"a\": {\n    \"b\": \"c\"\n  }\n}",
		},
		{
			name: "scalar replaces object",
			base: "{\n  \"a\": {\n    \"b\": 1\n  }\n}",
			over: `{"a": null}`,
			want: "{\n  \"a\": null\n}",
		},
		{
			name: "key order of other kept",
			base: "{}",
			over: `{"z": 1, "a": {"y": 1, "b": 2}}`,
			want: `{"z": 1, "a": {"y":1,"b":2}}`,
		},
		{
			name: "inline object",
			base: `{"a": 1, "b": 2}`,
			over: `{"c": 3}`,
			want: `{"a": 1, "b": 2, "c": 3}`,
		},
		{
			name: "equal value untouched",
			base: "{\"a\": [1, /* one */ 2]}",
			over: `{"a": [1,2]}`,
			want: "{\"a\": [1, /* one */ 2]}",
		},
		{
			name: "empty base",
			base: "",
			over: `{"a": [1]}`,
			want: "{\n  \"a\": [\n    1\n  ]\n}\n",
		},
		{
			name: "empty other",
			base: `{"a": 1}`,
			over: "// nothing\n",
			want: `{"a": 1}`,
		},
		{
			name: "duplicate keys: last wins on both sides",
			base: `{"a": 1, "a": {"x": 1}}`,
			over: `{"a": 5, "a": {"y": 2}}`,
			want: `{"a": 1, "a": {"x": 1, "y": 2}}`,
		},
		{
			name: "CRLF",
			base: "{\r\n  \"a\": 1\r\n}\r\n",
			over: `{"b": {"c": 1}}`,
			want: "{\r\n  \"a\": 1,\r\n  \"b\": {\r\n    \"c\": 1\r\n  }\r\n}\r\n",
		},
		{
			name: "block comment above new key is carried",
			base: "{\n\t\"a\": 1\n}",
			over: "{\n  /* about b */\n  \"b\": true\n}",
			want: "{\n\t\"a\": 1,\n\t/* about b */\n\t\"b\": true\n}",
		},
		{
			name: "multi-line comment is not carried",
			base: "{\n  \"a\": 1\n}",
			over: "{\n  /* two\n     lines */\n  \"b\": true\n}",
			want: "{\n  \"a\": 1,\n  \"b\": true\n}",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			a, _ := Parse([]byte(tt.base))
			b, _ := Parse([]byte(tt.over))
			if got := checkMerge(t, a, b); string(got) != tt.want {
				t.Errorf("got\n%q\nwant\n%q", got, tt.want)
			}
		})
	}
}

func TestMergeStrict(t *testing.T) {
	t.Parallel()
	a := mustParse(t, "{\n  \"a\": 1\n}", Strict())
	b := mustParse(t, "{\n  // c\n  \"b\": [1, /* x */ 2,],\n  \"a\": {\"k\": 1,},\n}")
	out := checkMerge(t, a, b)
	if strings.Contains(string(out), "/") || strings.Contains(string(out), ",]") {
		t.Fatalf("strict merge wrote comments or trailing commas: %q", out)
	}
}

func TestMergeRefuses(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, base, over string
		want             error
		msg              string
	}{
		{"invalid base", `{"a": }`, `{}`, ErrInvalid, "merge into"},
		{"invalid other", `{}`, `{"a"`, ErrInvalid, "merge from"},
		{"array other", `{}`, `[1]`, ErrType, "merged document's root is an array"},
		{"scalar base", `"s"`, `{"a": 1}`, ErrType, "target document's root is a string"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			a, _ := Parse([]byte(tt.base))
			b, _ := Parse([]byte(tt.over))
			err := a.Merge(b)
			if !errors.Is(err, tt.want) || !strings.Contains(err.Error(), tt.msg) {
				t.Fatalf("err = %v, want %v containing %q", err, tt.want, tt.msg)
			}
			if string(a.Render()) != tt.base {
				t.Fatalf("refused merge changed the document: %q", a.Render())
			}
		})
	}
	if err := mustParse(t, `{}`).Merge(nil); err != nil {
		t.Fatalf("Merge(nil) = %v", err)
	}
}

// TestMergeUntouched: merging one key changes only the window of the value
// it replaces, or the tail of the object it adds to.
func TestMergeUntouched(t *testing.T) {
	t.Parallel()
	base := "// head\n{\n  \"a\": 1, // one\n  /* b */\n  \"b\": {\n    \"c\": [1, 2], // c\n    \"d\": \"x\"\n  }\n} // tail\n"
	overs := []struct {
		over     string
		from, to string // the window is [index(from), index(to)) of base
	}{
		{`{"a": 2}`, "1, // one", ", // one"},
		{`{"b": {"c": {}}}`, "[1, 2]", ", // c"},
		{`{"b": {"d": [true]}}`, "\"x\"", "\n  }\n}"},
		{`{"b": {"e": 0}}`, "\n  }\n}", "\n  }\n}"},
		{`{"z": null}`, "\n} // tail", "\n} // tail"},
	}
	for _, tt := range overs {
		a := mustParse(t, base)
		out := checkMerge(t, a, mustParse(t, tt.over))
		from, to := strings.Index(base, tt.from), strings.Index(base, tt.to)
		texttest.CheckUntouched(t, []byte(base), out, from, to)
	}
}

// TestMergeLayouts: every edit layout merges to the reference result and
// keeps every one of its comments.
func TestMergeLayouts(t *testing.T) {
	t.Parallel()
	overs := []string{`{}`, `{"a": 2}`, `{"new": {"x": [1, {"y": null}]}}`, `{"a": {"b": 1}, "z": "s"}`, `{"x": {"y": false, "w": 1}}`}
	for _, base := range layouts {
		for _, over := range overs {
			a, err := Parse([]byte(base))
			if err != nil || a.root == nil || a.root.kind != KindObject {
				continue
			}
			var texts []string
			for _, c := range a.comments {
				texts = append(texts, string(a.src[c.start:c.end]))
			}
			out := checkMerge(t, a, mustParse(t, over))
			for _, c := range texts {
				if !strings.Contains(string(out), c) {
					t.Errorf("merge of %q into %q lost comment %q: %q", over, base, c, out)
				}
			}
		}
	}
}

func TestCloneIsDeep(t *testing.T) {
	t.Parallel()
	d := mustParse(t, "{\n  \"a\": {\"b\": 1} // c\n}", WithLookup(func(string) (string, bool) { return "", false }))
	d.SetPath("/tmp/orig.jsonc")
	before := string(d.Render())
	c := d.Clone()
	if string(c.Render()) != before || c.Path() != d.Path() || c.Mode() != d.Mode() {
		t.Fatal("clone differs from the original")
	}
	if c.root == d.root || &c.src[0] == &d.src[0] {
		t.Fatal("clone shares the tree or the source")
	}
	if err := c.Set("a.b", 2); err != nil {
		t.Fatal(err)
	}
	if err := c.Merge(mustParse(t, `{"x": 1}`)); err != nil {
		t.Fatal(err)
	}
	if err := c.SetComment("a", "changed"); err != nil {
		t.Fatal(err)
	}
	c.SetPath("/tmp/other.jsonc")
	c.opts.lookupers = append(c.opts.lookupers, funcLookuper{})
	if string(d.Render()) != before || d.Path() != "/tmp/orig.jsonc" || len(d.opts.lookupers) != 1 {
		t.Fatalf("editing the clone changed the original: %q %q %d", d.Render(), d.Path(), len(d.opts.lookupers))
	}
	if v, _ := d.GetInt("a.b"); v != 1 {
		t.Fatalf("original a.b = %d", v)
	}
}

// FuzzMerge: Merge never panics; on two valid object documents the result
// re-parses, equals the reference merge and is idempotent; otherwise it is
// refused (or a no-op) and leaves the target unchanged.
func FuzzMerge(f *testing.F) {
	pairs := [][2]string{
		{"{}", "{}"},
		{"{\n  \"a\": 1 // c\n}", "{\"a\": {\"b\": [1]}}"},
		{"{\"a\": {\"b\": 1}}", "{\n  // x\n  \"a\": {\"c\": 2},\n  \"d\": [],\n}"},
		{"", "{\"a\": 1}"},
		{"[1]", "{}"},
		{"{\"a\":1,\"a\":{}}", "{\"a\":{\"b\":1},\"a\":{\"c\":2}}"},
	}
	for _, l := range layouts {
		pairs = append(pairs, [2]string{l, `{"a": {"x": 1}, "n": [1, {"k": "v"}]}`}, [2]string{sample, l})
	}
	for _, p := range pairs {
		f.Add([]byte(p[0]), []byte(p[1]), false)
	}
	f.Fuzz(func(t *testing.T, x, y []byte, strict bool) {
		var opts []Option
		if strict {
			opts = append(opts, Strict())
		}
		a, errA := Parse(x, opts...)
		b, errB := Parse(y)
		ok := errA == nil && errB == nil &&
			(b.root == nil || b.root.kind == KindObject) &&
			(a.root == nil || a.root.kind == KindObject || b.root == nil)
		if !ok {
			err := a.Merge(b)
			if err == nil && b.root != nil {
				t.Fatalf("Merge(%q, %q) accepted", x, y)
			}
			if string(a.Render()) != string(x) {
				t.Fatalf("refused Merge changed the target")
			}
			return
		}
		checkMerge(t, a, b)
	})
}
