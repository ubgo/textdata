package jsonc

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	pathpkg "path"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/ubgo/textdata/plugin"
	"github.com/ubgo/textdata/pos"
)

// mapIncluder is an in-memory plugin.Includer with the same resolution rule as
// plugins/extends: ref relative to the folder of from. The jsonc module may
// not import a plugin module, so the tests carry their own.
type mapIncluder struct {
	name  string
	files map[string]string
	calls []string
}

func (m *mapIncluder) Name() string {
	if m.name == "" {
		return "mem"
	}
	return m.name
}

func (m *mapIncluder) Include(from, ref string) ([]byte, string, error) {
	m.calls = append(m.calls, ref)
	loc := pathpkg.Join(pathpkg.Dir(from), ref)
	if from == "" {
		loc = pathpkg.Clean(ref)
	}
	s, ok := m.files[loc]
	if !ok {
		return nil, "", fmt.Errorf("%s: %w", loc, plugin.ErrNotFound)
	}
	return []byte(s), loc, nil
}

func mem(files map[string]string) *mapIncluder { return &mapIncluder{files: files} }

func decodeMap(t *testing.T, d *Doc) map[string]any {
	t.Helper()
	var m map[string]any
	if err := d.Decode(&m); err != nil {
		t.Fatalf("Decode: %v\n%s", err, d.Render())
	}
	return m
}

func extendOK(t *testing.T, src string, inc plugin.Plugin, opts ...Option) (*Doc, map[string]any) {
	t.Helper()
	opts = append([]Option{WithFileName("app.jsonc"), WithPlugin(inc)}, opts...)
	d := mustParse(t, src, opts...)
	x, err := d.Extended()
	if err != nil {
		t.Fatalf("Extended: %v", err)
	}
	return x, decodeMap(t, x)
}

func TestExtendedSingle(t *testing.T) {
	inc := mem(map[string]string{
		"base.jsonc": `{
  // base settings
  "port": 80,
  "db": {"host": "localhost", "pool": 5},
  "tags": ["a", "b"]
}`,
	})
	src := `{
  "$extends": "base.jsonc", // inherit
  "port": 8080,
  "db": {"pool": 10},
  "tags": ["c"]
}`
	_, got := extendOK(t, src, inc)
	want := map[string]any{
		"port": 8080.0,
		"db":   map[string]any{"host": "localhost", "pool": 10.0},
		"tags": []any{"c"}, // arrays replace, like every other non-object
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v\nwant %v", got, want)
	}
}

func TestExtendedListOrder(t *testing.T) {
	inc := mem(map[string]string{
		"a.jsonc": `{"x": "a", "y": "a", "z": "a", "onlyA": 1}`,
		"b.jsonc": `{"x": "b", "y": "b"}`,
	})
	_, got := extendOK(t, `{"$extends": ["a.jsonc", "b.jsonc"], "x": "self"}`, inc)
	want := map[string]any{"x": "self", "y": "b", "z": "a", "onlyA": 1.0}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v want %v", got, want)
	}
	if !reflect.DeepEqual(inc.calls, []string{"a.jsonc", "b.jsonc"}) {
		t.Errorf("load order %v", inc.calls)
	}
}

func TestExtendedNestedRelativeToEachFile(t *testing.T) {
	inc := mem(map[string]string{
		"conf/app.jsonc":      `{"$extends": "../shared/base.jsonc", "app": 1}`,
		"shared/base.jsonc":   `{"$extends": ["root.jsonc", "./more/x.jsonc"], "base": 1, "lvl": "base"}`,
		"shared/root.jsonc":   `{"root": 1, "lvl": "root"}`,
		"shared/more/x.jsonc": `{"x": 1}`,
	})
	d := mustParse(t, `{"$extends": "conf/app.jsonc", "lvl": "top"}`, WithPlugin(inc))
	x, err := d.Extended()
	if err != nil {
		t.Fatal(err)
	}
	got := decodeMap(t, x)
	want := map[string]any{"app": 1.0, "base": 1.0, "root": 1.0, "x": 1.0, "lvl": "top"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v want %v", got, want)
	}
	if strings.Contains(string(x.Render()), ExtendsKey) {
		t.Errorf("$extends left in the result:\n%s", x.Render())
	}
}

func TestExtendedRenderUnchanged(t *testing.T) {
	inc := mem(map[string]string{"base.jsonc": `{"a": 1}`})
	src := "{\n  // keep me\n  \"$extends\": \"base.jsonc\",\n  \"b\": 2,\n}\n"
	d := mustParse(t, src, WithPlugin(inc))
	x, err := d.Extended()
	if err != nil {
		t.Fatal(err)
	}
	if string(d.Render()) != src {
		t.Errorf("receiver changed:\n%s", d.Render())
	}
	if refs, _ := d.Extends(); !reflect.DeepEqual(refs, []string{"base.jsonc"}) {
		t.Errorf("Extends = %v", refs)
	}
	// Editing the result never reaches the receiver.
	if err := x.Set("c", 3); err != nil {
		t.Fatal(err)
	}
	if string(d.Render()) != src {
		t.Error("edit of the result reached the receiver")
	}
}

func TestExtendedKeepsBaseText(t *testing.T) {
	// The first base is the starting text, so its comments survive in the
	// result; only members the document adds or replaces are rewritten.
	inc := mem(map[string]string{"base.jsonc": "{\n  // the port\n  \"port\": 80,\n  \"host\": \"h\"\n}\n"})
	x, _ := extendOK(t, `{"$extends": "base.jsonc", "port": 81}`, inc)
	want := "{\n  // the port\n  \"port\": 81,\n  \"host\": \"h\"\n}\n"
	if string(x.Render()) != want {
		t.Errorf("got\n%s\nwant\n%s", x.Render(), want)
	}
}

func TestExtendedCycles(t *testing.T) {
	cases := []struct {
		name  string
		files map[string]string
		start string
		chain string
	}{
		{"self", map[string]string{"a.jsonc": `{"$extends": "a.jsonc"}`}, "a.jsonc", "a.jsonc -> a.jsonc"},
		{"two files", map[string]string{
			"a.jsonc": `{"$extends": "b.jsonc"}`,
			"b.jsonc": `{"$extends": "./a.jsonc"}`,
		}, "a.jsonc", "a.jsonc -> b.jsonc -> a.jsonc"},
		{"three files", map[string]string{
			"a.jsonc":     `{"$extends": "sub/b.jsonc"}`,
			"sub/b.jsonc": `{"$extends": "../c.jsonc"}`,
			"c.jsonc":     `{"$extends": ["x.jsonc", "a.jsonc"]}`,
			"x.jsonc":     `{}`,
		}, "a.jsonc", "a.jsonc -> sub/b.jsonc -> c.jsonc -> a.jsonc"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			inc := mem(tc.files)
			d := mustParse(t, tc.files[tc.start], WithFileName(tc.start), WithPlugin(inc))
			_, err := d.Extended()
			if !errors.Is(err, ErrExtendsCycle) {
				t.Fatalf("err = %v, want ErrExtendsCycle", err)
			}
			if !strings.Contains(err.Error(), tc.chain) {
				t.Errorf("err = %v, want chain %q", err, tc.chain)
			}
			if pe := pos.First(err); pe == nil || pe.File != tc.start || pe.Pos.Line != 1 || pe.Pos.Col != 14 {
				t.Errorf("position = %+v, want %s:1:14", pe, tc.start)
			}
		})
	}
}

// A diamond (two bases sharing one) is not a cycle.
func TestExtendedDiamond(t *testing.T) {
	inc := mem(map[string]string{
		"b.jsonc": `{"$extends": "d.jsonc", "b": 1}`,
		"c.jsonc": `{"$extends": "d.jsonc", "c": 1}`,
		"d.jsonc": `{"d": 1}`,
	})
	_, got := extendOK(t, `{"$extends": ["b.jsonc", "c.jsonc"]}`, inc)
	if !reflect.DeepEqual(got, map[string]any{"b": 1.0, "c": 1.0, "d": 1.0}) {
		t.Errorf("got %v", got)
	}
}

func TestExtendedDepth(t *testing.T) {
	files := map[string]string{}
	for i := 0; i <= MaxExtendsDepth+1; i++ {
		files[fmt.Sprintf("f%d.jsonc", i)] = fmt.Sprintf(`{"$extends": "f%d.jsonc"}`, i+1)
	}
	files[fmt.Sprintf("f%d.jsonc", MaxExtendsDepth+2)] = `{}`
	d := mustParse(t, files["f0.jsonc"], WithFileName("f0.jsonc"), WithPlugin(mem(files)))
	if _, err := d.Extended(); !errors.Is(err, ErrExtendsDepth) {
		t.Fatalf("err = %v, want ErrExtendsDepth", err)
	}
	// Exactly at the bound is fine.
	files[fmt.Sprintf("f%d.jsonc", MaxExtendsDepth-1)] = `{"end": true}`
	if _, err := d.Extended(); err != nil {
		t.Fatalf("at the bound: %v", err)
	}
}

func TestExtendedMissingBase(t *testing.T) {
	d := mustParse(t, "{\n  \"$extends\": [\"ok.jsonc\", \"nope.jsonc\"]\n}", WithFileName("app.jsonc"), WithPlugin(mem(map[string]string{"ok.jsonc": "{}"})))
	_, err := d.Extended()
	if !errors.Is(err, plugin.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
	var pe *plugin.Error
	if !errors.As(err, &pe) || pe.Plugin != "mem" {
		t.Errorf("err does not name the plugin: %v", err)
	}
	if !strings.HasPrefix(err.Error(), "app.jsonc:2:28: $extends \"nope.jsonc\": jsonc: plugin \"mem\"") {
		t.Errorf("err = %q", err)
	}
}

func TestExtendedIncluderChain(t *testing.T) {
	first := &mapIncluder{name: "first", files: map[string]string{}}
	second := &mapIncluder{name: "second", files: map[string]string{"b.jsonc": `{"b": 1}`}}
	_, got := extendOK(t, `{"$extends": "b.jsonc"}`, first, WithPlugin(second))
	if got["b"] != 1.0 {
		t.Errorf("not-found should fall through to the next includer: %v", got)
	}

	denied := errorIncluder{err: plugin.ErrDenied}
	d := mustParse(t, `{"$extends": "b.jsonc"}`, WithPlugin(denied, second))
	if _, err := d.Extended(); !errors.Is(err, plugin.ErrDenied) {
		t.Errorf("a refusal must stop the chain: %v", err)
	}
}

type errorIncluder struct{ err error }

func (errorIncluder) Name() string { return "deny" }
func (e errorIncluder) Include(string, string) ([]byte, string, error) {
	return nil, "", e.err
}

func TestExtendedBadValues(t *testing.T) {
	for src, at := range map[string]string{
		`{"$extends": 1}`:               "1:14",
		`{"$extends": {"a": 1}}`:        "1:14",
		`{"$extends": ["a.jsonc", 2]}`:  "1:26",
		`{"$extends": ["a.jsonc", []]}`: "1:26",
	} {
		d := mustParse(t, src, WithFileName("f.jsonc"), WithPlugin(mem(map[string]string{"a.jsonc": "{}"})))
		_, err := d.Extended()
		if !errors.Is(err, ErrExtendsValue) || !strings.HasPrefix(err.Error(), "f.jsonc:"+at+":") {
			t.Errorf("%s: %v, want ErrExtendsValue at %s", src, err, at)
		}
		if _, err := d.Extends(); !errors.Is(err, ErrExtendsValue) {
			t.Errorf("%s: Extends: %v", src, err)
		}
	}
}

func TestExtendedBadBase(t *testing.T) {
	inc := mem(map[string]string{
		"broken.jsonc": "{\n  \"a\": 1\n  \"b\": 2\n}",
		"array.jsonc":  `[1, 2]`,
		"empty.jsonc":  ``,
	})
	d := mustParse(t, `{"$extends": "broken.jsonc"}`, WithFileName("app.jsonc"), WithPlugin(inc))
	_, err := d.Extended()
	if err == nil || !strings.Contains(err.Error(), "app.jsonc:1:14") || !strings.Contains(err.Error(), "broken.jsonc:3:") {
		t.Errorf("syntax error in base: %v", err)
	}
	d = mustParse(t, `{"$extends": "array.jsonc"}`, WithPlugin(inc))
	if _, err := d.Extended(); !errors.Is(err, ErrType) {
		t.Errorf("array base: %v, want ErrType", err)
	}
	// An empty base contributes nothing.
	_, got := extendOK(t, `{"$extends": "empty.jsonc", "a": 1}`, inc)
	if !reflect.DeepEqual(got, map[string]any{"a": 1.0}) {
		t.Errorf("empty base: %v", got)
	}
}

func TestExtendedOff(t *testing.T) {
	src := `{"$extends": "base.jsonc", "a": 1}`
	d := mustParse(t, src)
	// Without an Includer, $extends is an ordinary key for every reader.
	if v, _ := d.Get(`"$extends"`); v != "base.jsonc" {
		t.Errorf("Get = %v", v)
	}
	if got, err := d.extendIfInstalled(); err != nil || got != d {
		t.Errorf("extendIfInstalled without an Includer = %v, %v", got, err)
	}
	if _, err := d.Extended(); !errors.Is(err, ErrNoIncluder) {
		t.Errorf("Extended = %v, want ErrNoIncluder", err)
	}

	inc := mem(map[string]string{"base.jsonc": `{"b": 2}`})
	d = mustParse(t, src, WithPlugin(inc))
	x, err := d.extendIfInstalled()
	if err != nil || x == d || !reflect.DeepEqual(decodeMap(t, x), map[string]any{"a": 1.0, "b": 2.0}) {
		t.Errorf("extendIfInstalled with an Includer = %v, %v", x, err)
	}
}

func TestExtendedNoExtends(t *testing.T) {
	for _, src := range []string{`{"a": 1}`, `[1]`, `"s"`, ``, `{"x": {"$extends": "nested is data"}}`} {
		d := mustParse(t, src, WithPlugin(mem(nil)))
		x, err := d.Extended()
		if err != nil {
			t.Fatalf("%q: %v", src, err)
		}
		if x == d || string(x.Render()) != src {
			t.Errorf("%q: want an equal clone, got %q", src, x.Render())
		}
	}
	d, _ := Parse([]byte(`{"a" 1}`), WithPlugin(mem(nil)))
	if _, err := d.Extended(); !errors.Is(err, ErrInvalid) {
		t.Errorf("invalid doc: %v", err)
	}
}

func TestExtendedDuplicateKeys(t *testing.T) {
	inc := mem(map[string]string{"one.jsonc": `{"one": 1}`, "two.jsonc": `{"two": 2}`})
	x, got := extendOK(t, `{"$extends": "one.jsonc", "a": 1, "$extends": "two.jsonc"}`, inc)
	if !reflect.DeepEqual(got, map[string]any{"two": 2.0, "a": 1.0}) {
		t.Errorf("last $extends wins: %v", got)
	}
	if strings.Contains(string(x.Render()), ExtendsKey) {
		t.Errorf("every $extends removed: %s", x.Render())
	}
}

func TestExtendedResultCarriesReceiver(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "app.jsonc")
	if err := os.WriteFile(p, []byte(`{"$extends": "base.jsonc", "a": "${NAME}"}`), 0o640); err != nil {
		t.Fatal(err)
	}
	inc := mem(map[string]string{filepath.ToSlash(filepath.Join(dir, "base.jsonc")): `{"b": "${NAME}-base"}`})
	d, err := Open(p, WithPlugin(inc), WithLookup(func(string) (string, bool) { return "v", true }))
	if err != nil {
		t.Fatal(err)
	}
	x, err := d.Extended()
	if err != nil {
		t.Fatal(err)
	}
	if x.Path() != p || x.Mode() != d.Mode() || !x.Existed() {
		t.Errorf("result path/mode = %q %v", x.Path(), x.Mode())
	}
	// Expansion after extension reads the merged values with the
	// receiver's lookups, bases included.
	e, err := x.Expanded()
	if err != nil {
		t.Fatal(err)
	}
	if got := decodeMap(t, e); !reflect.DeepEqual(got, map[string]any{"a": "v", "b": "v-base"}) {
		t.Errorf("expanded = %v", got)
	}
}

func TestExtendedStrict(t *testing.T) {
	inc := mem(map[string]string{"c.json": "{// no\n}", "ok.json": `{"a":1}`})
	d := mustParse(t, `{"$extends": "c.json"}`, Strict(), WithPlugin(inc))
	if _, err := d.Extended(); err == nil {
		t.Error("a strict document's base is parsed strictly too")
	}
	d = mustParse(t, `{"$extends": "ok.json", "b": 2}`, Strict(), WithPlugin(inc))
	x, err := d.Extended()
	if err != nil || !reflect.DeepEqual(decodeMap(t, x), map[string]any{"a": 1.0, "b": 2.0}) {
		t.Errorf("strict: %v", err)
	}
}

// diskIncluder mirrors plugins/extends.New for an end-to-end disk test.
type diskIncluder struct{}

func (diskIncluder) Name() string { return "disk" }
func (diskIncluder) Include(from, ref string) ([]byte, string, error) {
	loc := filepath.Join(filepath.Dir(from), ref)
	b, err := os.ReadFile(loc)
	if errors.Is(err, fs.ErrNotExist) {
		err = fmt.Errorf("%w: %w", plugin.ErrNotFound, err)
	}
	return b, loc, err
}

func TestExtendedOnDisk(t *testing.T) {
	dir := t.TempDir()
	must := func(name, s string) {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), []byte(s), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	must("base.jsonc", `{"a": 1, "b": 1}`)
	must("app/app.jsonc", `{"$extends": "../base.jsonc", "b": 2}`)
	must("app/loop.jsonc", `{"$extends": "../app/loop.jsonc"}`)
	d, err := Open(filepath.Join(dir, "app", "app.jsonc"), WithPlugin(diskIncluder{}))
	if err != nil {
		t.Fatal(err)
	}
	x, err := d.Extended()
	if err != nil {
		t.Fatal(err)
	}
	if got := decodeMap(t, x); !reflect.DeepEqual(got, map[string]any{"a": 1.0, "b": 2.0}) {
		t.Errorf("got %v", got)
	}
	d, err = Open(filepath.Join(dir, "app", "loop.jsonc"), WithPlugin(diskIncluder{}))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.Extended(); !errors.Is(err, ErrExtendsCycle) {
		t.Errorf("a cycle spelled differently on disk: %v", err)
	}
}

func TestExtendsPluginInfo(t *testing.T) {
	d := mustParse(t, `{}`, WithPlugin(mem(nil)))
	if got := d.Plugins()[0].String(); got != "mem: Includer" {
		t.Errorf("Plugins = %q", got)
	}
}

// FuzzExtends: any $extends value, against a small set of files including
// cycles and broken ones, never panics, never changes the receiver, and a
// successful result has no root $extends left.
func FuzzExtends(f *testing.F) {
	for _, s := range []string{
		`"a.jsonc"`, `["a.jsonc", "b.jsonc"]`, `"loop1.jsonc"`, `"bad.jsonc"`, `"arr.jsonc"`,
		`1`, `null`, `[]`, `[1]`, `"../../x"`, `"nested.jsonc"`, `""`, `{"a":1}`,
	} {
		f.Add(s)
	}
	files := map[string]string{
		"a.jsonc":      `{"a": 1, "o": {"x": 1}}`,
		"b.jsonc":      `{"$extends": "a.jsonc", "o": {"y": 2}} // b`,
		"loop1.jsonc":  `{"$extends": "loop2.jsonc"}`,
		"loop2.jsonc":  `{"$extends": ["a.jsonc", "loop1.jsonc"]}`,
		"bad.jsonc":    `{"a" 1}`,
		"arr.jsonc":    `[1]`,
		"nested.jsonc": `{"$extends": "dir/../b.jsonc"}`,
	}
	f.Fuzz(func(t *testing.T, value string) {
		src := "{\n  \"$extends\": " + value + ",\n  \"self\": true\n}"
		d, err := Parse([]byte(src), WithPlugin(mem(files)))
		if err != nil {
			return
		}
		x, err := d.Extended()
		if string(d.Render()) != src {
			t.Fatalf("receiver changed")
		}
		if err != nil {
			return
		}
		if x.Err() != nil {
			t.Fatalf("result invalid: %v", x.Err())
		}
		if refs, err := x.Extends(); err != nil || len(refs) != 0 {
			t.Fatalf("$extends left: %v %v", refs, err)
		}
		if v, err := x.Get("self"); err != nil || v != true {
			t.Fatalf("self lost: %v %v\n%s", v, err, x.Render())
		}
	})
}
