package jsonc

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestJSONTestSuite runs every JSONTestSuite parsing case.
//
//   - Strict(): the verdict equals encoding/json.Valid for every file, y_, n_
//     and i_ alike.
//   - Default (JSONC): every y_ file parses; an n_ file is accepted only when
//     it is valid JSONC (a trailing comma or a comment is the only reason
//     JSON rejects it), and then JSON() is valid JSON.
//   - Always: Render returns the file byte for byte, valid or not.
func TestJSONTestSuite(t *testing.T) {
	t.Parallel()
	files, err := filepath.Glob("testdata/JSONTestSuite/*.json")
	if err != nil || len(files) < 300 {
		t.Fatalf("suite missing: %d files, %v", len(files), err)
	}
	for _, f := range files {
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		name := filepath.Base(f)

		sd, serr := Parse(src, Strict())
		if got, want := serr == nil, json.Valid(src); got != want {
			t.Errorf("%s: Strict accepted=%v, encoding/json.Valid=%v (err: %v)", name, got, want, serr)
		}
		if !bytes.Equal(sd.Render(), src) {
			t.Errorf("%s: strict Render changed bytes", name)
		}

		d, err := Parse(src)
		if !bytes.Equal(d.Render(), src) {
			t.Errorf("%s: Render changed bytes", name)
		}
		switch {
		case strings.HasPrefix(name, "y_") && err != nil:
			t.Errorf("%s: valid JSON rejected: %v", name, err)
		case err == nil && d.root != nil:
			j, jerr := d.JSON()
			if jerr != nil || !json.Valid(j) {
				t.Errorf("%s: accepted, but JSON() is not valid JSON: %q (%v)", name, j, jerr)
			}
		}
		if strings.HasPrefix(name, "n_") && err == nil && json.Valid(src) == false && !jsoncOnly(d) {
			t.Errorf("%s: invalid JSON accepted for no JSONC reason", name)
		}
	}
}

// jsoncOnly reports whether d uses a JSONC extension (comments, a trailing
// comma, a BOM) or is empty — the only reasons JSONC may accept what JSON
// rejects.
func jsoncOnly(d *Doc) bool {
	if d.root == nil || len(d.comments) > 0 || bytes.HasPrefix(d.src, []byte(bom)) {
		return true
	}
	trailing := false
	walk(d.root, func(n *node) {
		if l := lastItem(n); l != nil && l.comma >= 0 {
			trailing = true
		}
	})
	return trailing
}

// TestJSONCCases is the JSONC case set: each input parses, renders byte for
// byte, and JSON() decodes to the same value as the expected plain JSON.
func TestJSONCCases(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, src, want string
	}{
		{"line comments everywhere", "// head\n{ // after brace\n  \"a\" // before colon\n  : // after colon\n  1 // after value\n  , // after comma\n  \"b\": 2 // last\n} // tail", `{"a":1,"b":2}`},
		{"block comments everywhere", "/*h*/{/*1*/\"a\"/*2*/:/*3*/[/*4*/1/*5*/,/*6*/2/*7*/]/*8*/}/*t*/", `{"a":[1,2]}`},
		{"trailing commas", "{\"a\": [1, 2,], \"b\": {\"c\": 3,},}", `{"a":[1,2],"b":{"c":3}}`},
		{"CRLF", "{\r\n  // c\r\n  \"a\": 1,\r\n  \"b\": \"x\"\r\n}\r\n", `{"a":1,"b":"x"}`},
		{"BOM", "\xef\xbb\xbf{\"a\": true}", `{"a":true}`},
		{"comment-like strings", `{"url": "http://x//y", "c": "/* not */"}`, `{"url":"http://x//y","c":"/* not */"}`},
		{"empty containers", "{\"o\": {}, \"a\": [], \"n\": null}", `{"o":{},"a":[],"n":null}`},
		{"scalar root", "// only a number\n42\n", `42`},
		{"block comment with stars", "{/** doc **/\"a\": 1 /* a * b */}", `{"a":1}`},
		{"unicode", "{\"ключ\": \"значение\", \"e\": \"\\u00e9\\ud83d\\ude00\"}", `{"ключ":"значение","e":"é😀"}`},
		{"duplicate keys: last wins", `{"a": 1, "a": 2}`, `{"a":2}`},
	}
	for _, c := range cases {
		d, err := Parse([]byte(c.src))
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		if string(d.Render()) != c.src {
			t.Errorf("%s: Render changed bytes", c.name)
		}
		j, err := d.JSON()
		if err != nil || !json.Valid(j) {
			t.Errorf("%s: JSON() = %q, %v", c.name, j, err)
			continue
		}
		var got, want any
		_ = json.Unmarshal(j, &got)
		_ = json.Unmarshal([]byte(c.want), &want)
		gb, _ := json.Marshal(got)
		wb, _ := json.Marshal(want)
		if !bytes.Equal(gb, wb) {
			t.Errorf("%s: decoded %s, want %s", c.name, gb, wb)
		}
		if _, err := Parse([]byte(c.src), Strict()); err == nil && c.name != "unicode" && c.name != "comment-like strings" && c.name != "empty containers" && c.name != "duplicate keys: last wins" {
			t.Errorf("%s: Strict accepted a JSONC-only input", c.name)
		}
	}
}

// TestErrors pins messages and file:line:col for each class of mistake.
func TestErrors(t *testing.T) {
	t.Parallel()
	cases := []struct {
		src, want string
		strict    bool
	}{
		{"{\n  \"a\": 1\n  \"b\": 2\n}", `config.jsonc:3:3: missing comma before '"'`, false},
		{"{\n  port: 1\n}", "config.jsonc:2:3: object keys must be double-quoted (unquoted keys are JSON5)", false},
		{"{'a': 1}", "config.jsonc:1:2: single-quoted keys are not JSON (that is JSON5); use double quotes", false},
		{"['a']", "config.jsonc:1:2: single-quoted strings are not JSON (that is JSON5); use double quotes", false},
		{"[0x1F]", "config.jsonc:1:2: hex numbers are not JSON (that is JSON5)", false},
		{"[NaN]", "config.jsonc:1:2: NaN is not JSON (that is JSON5)", false},
		{"[-Infinity]", "config.jsonc:1:2: Infinity is not JSON (that is JSON5)", false},
		{"[01]", "config.jsonc:1:2: invalid number: leading zeros are not allowed", false},
		{"[1.]", "config.jsonc:1:2: invalid number: '.' needs digits after it", false},
		{"[1e]", "config.jsonc:1:2: invalid number: exponent needs digits", false},
		{"{\"a\" 1}", `config.jsonc:1:6: missing ':' after key "a"`, false},
		{"{\"a\": 1", `config.jsonc:1:1: unclosed object: missing '}'`, false},
		{"[1, 2", `config.jsonc:1:1: unclosed array: missing ']'`, false},
		{"[1,,2]", `config.jsonc:1:4: unexpected ',': expected a value or ']'`, false},
		{"\"abc", "config.jsonc:1:1: unterminated string", false},
		{"\"a\tb\"", "config.jsonc:1:3: control character 0x09 in string: escape it", false},
		{`"\x"`, `config.jsonc:1:2: invalid escape "\\x" in string`, false},
		{`"\u12"`, `config.jsonc:1:2: invalid \u escape: need four hex digits`, false},
		{"{} x", `config.jsonc:1:4: unexpected 'x' after the end of the document`, false},
		{"[tru]", `config.jsonc:1:2: unexpected word "tru": expected a value (strings need double quotes)`, false},
		{"/* open", "config.jsonc:1:1: unterminated block comment", false},
		{"[1] // c", "config.jsonc:1:5: comments are not allowed in strict JSON", true},
		{"[1,]", "config.jsonc:1:3: trailing comma is not allowed in strict JSON", true},
		{"", "config.jsonc:1:1: empty document: expected a JSON value", true},
		{"\xef\xbb\xbf1", "config.jsonc:1:1: byte order mark is not allowed in strict JSON", true},
		{"[\xff]", "config.jsonc:1:2: unexpected byte 0xff: expected a value", false},
		{strings.Repeat("[", maxDepth+1), "config.jsonc:1:10001: nesting deeper than 10000 levels", false},
	}
	for _, c := range cases {
		opts := []Option{WithFileName("config.jsonc")}
		if c.strict {
			opts = append(opts, Strict())
		}
		d, err := Parse([]byte(c.src), opts...)
		if err == nil || err.Error() != c.want {
			t.Errorf("Parse(%q) error = %v\nwant %s", c.src, err, c.want)
		}
		if string(d.Render()) != c.src {
			t.Errorf("Parse(%q): Render changed bytes", c.src)
		}
		if d.Err() == nil {
			t.Errorf("Parse(%q): Err() = nil", c.src)
		}
	}
}

// TestTolerant: a broken file still yields the tree up to the error, readable
// by an editor, while edits and decoding refuse.
func TestTolerant(t *testing.T) {
	t.Parallel()
	src := "{\n  \"ok\": 1,\n  \"also\": [true],\n  broken\n}"
	d, err := Parse([]byte(src))
	if err == nil {
		t.Fatal("no error")
	}
	if d.root == nil || len(d.root.items) != 2 || d.root.items[1].key != "also" {
		t.Fatalf("partial tree not kept: %+v", d.root)
	}
	if string(d.Render()) != src {
		t.Error("Render changed bytes")
	}
	for name, err := range map[string]error{
		"Set":    d.Set("ok", 2),
		"Delete": d.Delete("ok"),
		"Decode": d.Decode(&map[string]any{}),
	} {
		if !errorsIs(err, ErrInvalid) {
			t.Errorf("%s on an invalid doc = %v, want ErrInvalid", name, err)
		}
	}
	if _, err := d.JSON(); !errorsIs(err, ErrInvalid) {
		t.Errorf("JSON = %v", err)
	}
}
