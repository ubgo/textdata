package jsonc

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/ubgo/textdata/texttest"
)

// layouts are object/array shapes every insert and delete must handle.
var layouts = []string{
	"{}",
	"{ }",
	"{\n}",
	"{\r\n}",
	"{\n  // only a comment\n}",
	"{\"a\": 1}",
	"{\"a\": 1,}",
	"{\"a\": 1 /* c */}",
	"{\"a\":1, \"b\":2}",
	"{\n  \"a\": 1\n}",
	"{\n  \"a\": 1,\n}",
	"{\n  \"a\": 1 // note\n}",
	"{\n  \"a\": 1,   \n  \"b\": [1,\n    2]\n}\n",
	"{\r\n\t\"a\": 1\r\n}\r\n",
	"{\n  \"a\": 1, \"b\": 2\n}",
	"// head\n{\n  \"x\": {\n    \"y\": true\n  }\n} // tail\n",
	"\xef\xbb\xbf{\"a\":[]}",
}

// TestSetDeleteInverse: Set of a new key then Delete of it returns the
// original bytes, and the Set touches only one window of the input.
func TestSetDeleteInverse(t *testing.T) {
	t.Parallel()
	values := []any{1, "s", true, nil, map[string]any{"k": []any{1, "two"}}, []any{}, Raw(`{"raw": /* kept */ 1}`)}
	for _, src := range layouts {
		for _, v := range values {
			d := mustParse(t, src)
			if err := d.Set("zz", v); err != nil {
				t.Fatalf("Set on %q: %v", src, err)
			}
			got := d.Render()
			start, endA, _ := texttest.DiffWindow([]byte(src), got)
			texttest.CheckUntouched(t, []byte(src), got, start, endA)
			if back, err := d.Get("zz"); err != nil || (v != nil && back == nil && v != any(nil)) {
				t.Errorf("Get after Set on %q = %v, %v", src, back, err)
			}
			if err := d.Delete("zz"); err != nil {
				t.Fatalf("Delete on %q: %v", got, err)
			}
			if string(d.Render()) != src {
				t.Errorf("Set+Delete on %q\nafter Set:    %q\nafter Delete: %q", src, got, d.Render())
			}
		}
	}
}

func TestSetLayouts(t *testing.T) {
	t.Parallel()
	cases := []struct{ src, path, want string }{
		{"{\n  \"a\": 1\n}", "b", "{\n  \"a\": 1,\n  \"b\": 2\n}"},
		{"{\n  \"a\": 1, // note\n}", "b", "{\n  \"a\": 1, // note\n  \"b\": 2,\n}"},
		{"{\n  \"a\": 1 // note\n}", "b", "{\n  \"a\": 1, // note\n  \"b\": 2\n}"},
		{"{\"a\": 1}", "b", "{\"a\": 1, \"b\": 2}"},
		{"{\"a\":1,}", "b", "{\"a\":1, \"b\":2,}"},
		{"{}", "b", "{\"b\": 2}"},
		{"{\n}", "b", "{\n  \"b\": 2\n}"},
		{"{\r\n\t\"a\": 1\r\n}", "b", "{\r\n\t\"a\": 1,\r\n\t\"b\": 2\r\n}"},
		{"{\"a\":1}", "a", "{\"a\":2}"},
		{"{\"a\": {\"x\": 1}}", "a.y", "{\"a\": {\"x\": 1, \"y\": 2}}"},
		{"{\n  \"a\": {}\n}", "a.b.c", "{\n  \"a\": {\"b\": {\"c\":2}}\n}"},
		{"{\n  \"a\": 1\n}", "n.m", "{\n  \"a\": 1,\n  \"n\": {\n    \"m\": 2\n  }\n}"},
		{"", "a", "{\n  \"a\": 2\n}\n"},
		{"// new file", "a", "// new file\n{\n  \"a\": 2\n}\n"},
		{"[1, 2]", "[1]", "[1, 2]"},
		{"{\"k\" : 1}", "z", "{\"k\" : 1, \"z\" : 2}"},
	}
	for _, c := range cases {
		d := mustParse(t, c.src)
		if err := d.Set(c.path, 2); err != nil {
			t.Errorf("Set(%q) on %q: %v", c.path, c.src, err)
			continue
		}
		if got := string(d.Render()); got != c.want {
			t.Errorf("Set(%q) on %q\n got %q\nwant %q", c.path, c.src, got, c.want)
		}
	}

	d := mustParse(t, "{\n  \"server\": {\n    \"port\": 1\n  }\n}")
	if err := d.Set("server", map[string]any{"port": 2, "tls": true}); err != nil {
		t.Fatal(err)
	}
	if got, want := string(d.Render()), "{\n  \"server\": {\n    \"port\": 2,\n    \"tls\": true\n  }\n}"; got != want {
		t.Errorf("replace container\n got %q\nwant %q", got, want)
	}
	if err := d.Set("", Raw("[1, /* c */ 2]")); err != nil || string(d.Render()) != "[1, /* c */ 2]" {
		t.Errorf("replace root = %q, %v", d.Render(), err)
	}
}

func TestSetErrors(t *testing.T) {
	t.Parallel()
	d := mustParse(t, `{"s": "x", "arr": [1]}`)
	for p, want := range map[string]error{
		"s.x":      ErrType,
		"arr[5]":   ErrNotFound,
		"arr.k":    ErrType,
		"new[0]":   ErrNotFound,
		"new.a[1]": ErrNotFound,
	} {
		if err := d.Set(p, 1); !errors.Is(err, want) {
			t.Errorf("Set(%q) = %v, want %v", p, err, want)
		}
	}
	if err := d.Set("a", Raw("{bad}")); err == nil {
		t.Error("invalid Raw accepted")
	}
	if err := d.Set("a", make(chan int)); err == nil {
		t.Error("unencodable value accepted")
	}
	if err := d.Set("a..b", 1); err == nil {
		t.Error("bad path accepted")
	}
	if err := mustParse(t, `{"o": {}}`).Set("o[0]", 1); !errors.Is(err, ErrType) {
		t.Errorf("index into object = %v", err)
	}
	if string(d.Render()) != `{"s": "x", "arr": [1]}` {
		t.Errorf("failed edits changed the doc: %q", d.Render())
	}
}

func TestSetAfterBefore(t *testing.T) {
	t.Parallel()
	src := "{\n  \"a\": 1,\n  // about c\n  \"c\": 3\n}"
	d := mustParse(t, src)
	if err := d.SetAfter("a", "b", 2); err != nil {
		t.Fatal(err)
	}
	if got, want := string(d.Render()), "{\n  \"a\": 1,\n  \"b\": 2,\n  // about c\n  \"c\": 3\n}"; got != want {
		t.Errorf("SetAfter\n got %q\nwant %q", got, want)
	}
	if err := d.SetBefore("a", "first", 0); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(d.Render()), "{\n  \"first\": 0,\n  \"a\": 1,") {
		t.Errorf("SetBefore = %q", d.Render())
	}
	if err := d.SetAfter("c", "z", 9); err != nil || !strings.HasSuffix(string(d.Render()), "\"c\": 3,\n  \"z\": 9\n}") {
		t.Errorf("SetAfter last = %q, %v", d.Render(), err)
	}
	if err := d.SetAfter("a", "c", 33); err != nil || !strings.Contains(string(d.Render()), "\"c\": 33,") {
		t.Errorf("SetAfter on an existing key updates in place: %q, %v", d.Render(), err)
	}

	inline := mustParse(t, `{"a": 1, "c": 3}`)
	if err := inline.SetAfter("a", "b", 2); err != nil || string(inline.Render()) != `{"a": 1, "b": 2, "c": 3}` {
		t.Errorf("inline SetAfter = %q, %v", inline.Render(), err)
	}
	if err := inline.SetBefore("a", "z", 0); err != nil || string(inline.Render()) != `{"z": 0, "a": 1, "b": 2, "c": 3}` {
		t.Errorf("inline SetBefore = %q, %v", inline.Render(), err)
	}
	for _, c := range []struct {
		anchor, p string
		want      error
	}{
		{"nope", "x", ErrNotFound},
		{"a", "q.r", ErrNotFound},
		{"a", "[0]", ErrType},
		{"a", "a.b", ErrType},
	} {
		if err := mustParse(t, `{"a": 1}`).SetAfter(c.anchor, c.p, 1); !errors.Is(err, c.want) {
			t.Errorf("SetAfter(%q, %q) = %v, want %v", c.anchor, c.p, err, c.want)
		}
	}
	if err := mustParse(t, `[1]`).SetAfter("a", "b", 1); !errors.Is(err, ErrType) {
		t.Errorf("SetAfter on array root = %v", err)
	}
}

func TestDelete(t *testing.T) {
	t.Parallel()
	cases := []struct{ src, path, want string }{
		{"{\n  // doc for a\n  \"a\": 1, // trailing\n  \"b\": 2\n}", "a", "{\n  \"b\": 2\n}"},
		{"{\n  \"a\": 1,\n  // doc for b\n  \"b\": 2 // t\n}", "b", "{\n  \"a\": 1\n}"},
		{"{\"a\": 1, \"b\": 2, \"c\": 3}", "b", "{\"a\": 1, \"c\": 3}"},
		{"{\"a\": 1, \"b\": 2}", "a", "{\"b\": 2}"},
		{"{\"a\": 1, \"b\": 2}", "b", "{\"a\": 1}"},
		{"{\"a\": 1}", "a", "{}"},
		{"{\"a\": [1, 2, 3]}", "a[1]", "{\"a\": [1, 3]}"},
		{"{\"a\": {\"x\": {\"y\": 1}}}", "a.x.y", "{\"a\": {\"x\": {}}}"},
		{"{\"d\": 1, \"d\": 2}", "d", "{\"d\": 1}"},
	}
	for _, c := range cases {
		d := mustParse(t, c.src)
		if err := d.Delete(c.path); err != nil {
			t.Errorf("Delete(%q) on %q: %v", c.path, c.src, err)
			continue
		}
		if got := string(d.Render()); got != c.want {
			t.Errorf("Delete(%q) on %q\n got %q\nwant %q", c.path, c.src, got, c.want)
		}
	}
	d := mustParse(t, `{"a": 1}`)
	if err := d.Delete(""); !errors.Is(err, ErrType) {
		t.Errorf("Delete root = %v", err)
	}
	if err := d.Delete("zz"); !errors.Is(err, ErrNotFound) {
		t.Errorf("Delete missing = %v", err)
	}
}

func TestArrays(t *testing.T) {
	t.Parallel()
	d := mustParse(t, "{\n  \"list\": [\n    \"a\",\n    \"c\"\n  ],\n  \"flat\": [1, 2]\n}")
	steps := []struct {
		name string
		op   func() error
	}{
		{"insert", func() error { return d.Insert("list", 1, "b") }},
		{"append", func() error { return d.Append("list", "d") }},
		{"insert first", func() error { return d.Insert("list", 0, "0") }},
		{"append flat", func() error { return d.Append("flat", 3) }},
		{"insert flat", func() error { return d.Insert("flat", 0, 0) }},
		{"remove", func() error { return d.Remove("flat", 1) }},
	}
	for _, s := range steps {
		if err := s.op(); err != nil {
			t.Fatalf("%s: %v", s.name, err)
		}
	}
	want := "{\n  \"list\": [\n    \"0\",\n    \"a\",\n    \"b\",\n    \"c\",\n    \"d\"\n  ],\n  \"flat\": [0, 2, 3]\n}"
	if got := string(d.Render()); got != want {
		t.Errorf("arrays\n got %q\nwant %q", got, want)
	}
	for name, err := range map[string]error{
		"insert range": d.Insert("list", 99, 1),
		"remove range": d.Remove("list", -1),
		"not array":    d.Append("", 1),
		"missing":      d.Append("nope", 1),
	} {
		if err == nil {
			t.Errorf("%s: no error", name)
		}
	}
	if err := mustParse(t, "[]").Append("", 1); err != nil {
		t.Error(err)
	}
}

func TestDisableRestore(t *testing.T) {
	t.Parallel()
	src := "{\n  \"a\": 1,\n  // the db\n  \"db\": {\n    \"host\": \"x\"\n  }, // keep\n  \"z\": true\n}\n"
	d := mustParse(t, src)
	if err := d.Disable("db"); err != nil {
		t.Fatal(err)
	}
	want := "{\n  \"a\": 1,\n  // the db\n  // \"db\": {\n    // \"host\": \"x\"\n  // }, // keep\n  \"z\": true\n}\n"
	if got := string(d.Render()); got != want {
		t.Fatalf("Disable\n got %q\nwant %q", got, want)
	}
	if d.Has("db") {
		t.Error("disabled key still live")
	}
	if err := d.Restore("db"); err != nil {
		t.Fatal(err)
	}
	if string(d.Render()) != src {
		t.Errorf("Restore not byte-identical:\n%q", d.Render())
	}

	// Disable the last member, then Set a new key (it goes after the last
	// live member, above the disabled block): Restore adds the comma the
	// member before it now needs.
	d = mustParse(t, "{\n  \"a\": 1,\n  \"b\": 2\n}")
	_ = d.Disable("b")
	_ = d.Set("c", 3)
	if err := d.Restore("b"); err != nil {
		t.Fatal(err)
	}
	if got := string(d.Render()); got != "{\n  \"a\": 1,\n  \"c\": 3,\n  \"b\": 2\n}" {
		t.Errorf("restore after a live member = %q", got)
	}

	for name, err := range map[string]error{
		"restore live":    d.Restore("a"),
		"restore missing": d.Restore("nope"),
		"disable missing": d.Disable("nope"),
		"disable inline":  mustParse(t, `{"a": 1}`).Disable("a"),
		"disable strict":  mustParse(t, "{\n  \"a\": 1\n}", Strict()).Disable("a"),
		"restore root":    d.Restore(""),
		"restore in arr":  mustParse(t, "[1]").Restore("[0].x"),
	} {
		if err == nil {
			t.Errorf("%s: no error", name)
		}
	}
	if err := d.Restore("a"); !errors.Is(err, ErrExists) {
		t.Errorf("restore live = %v", err)
	}
	if err := mustParse(t, `{"a": 1}`).Disable("a"); !errors.Is(err, ErrLayout) {
		t.Errorf("disable inline = %v", err)
	}
}

func TestComments(t *testing.T) {
	t.Parallel()
	d := mustParse(t, "{\n  // old\n  \"a\": 1, // tail\n  \"b\": 2\n}")
	if err := d.SetComment("a", "new\ntwo lines"); err != nil {
		t.Fatal(err)
	}
	if got, want := string(d.Render()), "{\n  // new\n  // two lines\n  \"a\": 1, // tail\n  \"b\": 2\n}"; got != want {
		t.Errorf("SetComment replace\n got %q\nwant %q", got, want)
	}
	if c, _ := d.Comment("a"); c != "new\ntwo lines" {
		t.Errorf("Comment = %q", c)
	}
	if err := d.SetComment("b", "about b"); err != nil || !strings.Contains(string(d.Render()), "  // about b\n  \"b\"") {
		t.Errorf("SetComment add = %q, %v", d.Render(), err)
	}
	if err := d.RemoveComment("a"); err != nil {
		t.Fatal(err)
	}
	if got, want := string(d.Render()), "{\n  \"a\": 1,\n  // about b\n  \"b\": 2\n}"; got != want {
		t.Errorf("RemoveComment\n got %q\nwant %q", got, want)
	}
	if err := d.RemoveComment("a"); err != nil {
		t.Errorf("RemoveComment with none = %v", err)
	}
	inline := mustParse(t, `{"a": 1}`)
	if err := inline.SetComment("a", "x"); err != nil || string(inline.Render()) != `{/* x */ "a": 1}` {
		t.Errorf("inline SetComment = %q, %v", inline.Render(), err)
	}
	if err := inline.SetComment("a", "a */ b"); err == nil {
		t.Error("*/ in a block comment accepted")
	}
	if err := mustParse(t, `{"a": 1}`, Strict()).SetComment("a", "x"); !errors.Is(err, ErrStrict) {
		t.Errorf("strict SetComment = %v", err)
	}
	if err := mustParse(t, `{}`).SetComment("", "x"); !errors.Is(err, ErrType) {
		t.Errorf("SetComment root = %v", err)
	}
	if err := mustParse(t, `{}`).RemoveComment("x"); !errors.Is(err, ErrNotFound) {
		t.Errorf("RemoveComment missing = %v", err)
	}
}

// TestEditsKeepJSONMeaning: after each edit, JSON() decodes to what the edit
// promised, and the document stays valid under its own mode.
func TestEditsKeepJSONMeaning(t *testing.T) {
	t.Parallel()
	d := mustParse(t, sample)
	_ = d.Set("sync.interval", 60)
	_ = d.Set("new.deep.key", []any{"x"})
	_ = d.Delete("list")
	_ = d.Append(`"a.b".c`, 30)
	var got map[string]any
	if err := d.Decode(&got); err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(got)
	for _, want := range []string{`"interval":60`, `"new":{"deep":{"key":["x"]}}`, `"c":[10,20,{"deep":"yes"},30]`} {
		if !bytes.Contains(b, []byte(want)) {
			t.Errorf("decoded %s lacks %s", b, want)
		}
	}
	if bytes.Contains(b, []byte(`"list"`)) {
		t.Errorf("deleted key still decodes: %s", b)
	}
	if !strings.Contains(string(d.Render()), "// Sync settings") || !strings.Contains(string(d.Render()), "// seconds") {
		t.Error("an edit dropped a comment")
	}
}

func TestStrictEdits(t *testing.T) {
	t.Parallel()
	d := mustParse(t, "{\n  \"a\": 1\n}", Strict())
	if err := d.Set("b", 2); err != nil {
		t.Fatal(err)
	}
	if err := d.Delete("b"); err != nil || string(d.Render()) != "{\n  \"a\": 1\n}" {
		t.Errorf("strict Set/Delete = %q, %v", d.Render(), err)
	}
	if err := d.Set("c", Raw("1 // no")); err == nil {
		t.Error("a comment entered a strict document")
	}
}
