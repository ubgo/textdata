package format

import (
	"bytes"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"testing"

	"github.com/ubgo/textdata/jsonc"
	"github.com/ubgo/textdata/texttest"
)

// commentTexts lists every comment of a valid document, trailing blanks of
// line comments trimmed, sorted.
func commentTexts(src []byte) []string {
	var out []string
	for _, t := range lex(bytes.TrimPrefix(src, []byte(bom))) {
		if t.kind == tokComment {
			out = append(out, t.text)
		}
	}
	sort.Strings(out)
	return out
}

func compactJSON(t testing.TB, d *jsonc.Doc) []byte {
	t.Helper()
	j, err := d.JSON()
	if err != nil {
		t.Fatal(err)
	}
	var b bytes.Buffer
	if err := json.Compact(&b, j); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// checkFormat asserts every Format promise for d in style s.
func checkFormat(t testing.TB, d *jsonc.Doc, s Style) []byte {
	t.Helper()
	var opts []jsonc.Option
	if d.IsStrict() {
		opts = append(opts, jsonc.Strict())
	}
	out, err := Format(d, s)
	if err != nil {
		t.Fatalf("Format: %v", err)
	}
	f, err := jsonc.Parse(out, opts...)
	if err != nil {
		t.Fatalf("output does not re-parse: %v\nin:  %q\nout: %q", err, d.Render(), out)
	}
	if a, b := compactJSON(t, d), compactJSON(t, f); !bytes.Equal(a, b) {
		t.Fatalf("meaning changed:\n%s\n%s\nout: %q", a, b, out)
	}
	if a, b := commentTexts(d.Render()), commentTexts(out); strings.Join(a, "\x00") != strings.Join(b, "\x00") {
		t.Fatalf("comments changed:\n%q\n%q\nin:  %q\nout: %q", a, b, d.Render(), out)
	}
	again, err := Format(f, s)
	if err != nil || !bytes.Equal(again, out) {
		t.Fatalf("not idempotent:\n%q\n%q", out, again)
	}
	// A Strict() document re-parses with Strict() above, which rejects any
	// comment or trailing comma: the output gained none.
	return out
}

func TestFormatExamples(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, src string
		style     Style
		want      string
	}{
		{"compact object", `{"a":1,"b":[1,2],"c":{},"d":[]}`, Style{},
			"{\n  \"a\": 1,\n  \"b\": [\n    1,\n    2\n  ],\n  \"c\": {},\n  \"d\": []\n}\n"},
		{"comments kept", "// head\n{\n    // about a\n    \"a\":1, // one\n\n\n    \"b\":2 /* two */\n    // end\n} // tail\n", Style{},
			"// head\n{\n  // about a\n  \"a\": 1, // one\n\n  \"b\": 2 /* two */\n  // end\n} // tail\n"},
		{"tabs and trailing commas", `{"a":{"b":[true,null]}}`, Style{Tabs: true, TrailingCommas: true},
			"{\n\t\"a\": {\n\t\t\"b\": [\n\t\t\ttrue,\n\t\t\tnull,\n\t\t],\n\t},\n}\n"},
		{"trailing commas removed", "{\"a\": [1,],}", Style{Indent: 4},
			"{\n    \"a\": [\n        1\n    ]\n}\n"},
		{"odd comments move", "{\"a\" /* k */ : // v\n 1 /* c */ , \"b\": 2}", Style{},
			"{\n  /* k */\n  // v\n  \"a\": 1, /* c */\n  \"b\": 2\n}\n"},
		{"comment-only containers", "{ /* x */ }", Style{}, "{ /* x */\n}\n"},
		{"CRLF and BOM", "\xef\xbb\xbf{\r\n\"a\":1}\r\n", Style{}, "\xef\xbb\xbf{\r\n  \"a\": 1\r\n}\r\n"},
		{"no final newline", `[1]`, Style{NoFinalNewline: true}, "[\n  1\n]"},
		{"scalar root", ` "s" `, Style{}, "\"s\"\n"},
		{"empty", "", Style{}, ""},
		{"comment only", "  // c\n\n/* d */", Style{}, "// c\n\n/* d */\n"},
		{"line comment before comma", "[1 // a\n /* b */ , 2]", Style{}, "[\n  1, // a\n  /* b */\n  2\n]\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			d, err := jsonc.Parse([]byte(tt.src))
			if err != nil {
				t.Fatal(err)
			}
			if got := checkFormat(t, d, tt.style); string(got) != tt.want {
				t.Errorf("got\n%q\nwant\n%q", got, tt.want)
			}
			if string(d.Render()) != tt.src {
				t.Error("Format changed the document")
			}
		})
	}
}

func TestFormatStrict(t *testing.T) {
	t.Parallel()
	d, err := jsonc.Parse([]byte(`{"a":[1,{"b":2}]}`), jsonc.Strict())
	if err != nil {
		t.Fatal(err)
	}
	out := checkFormat(t, d, Style{TrailingCommas: true})
	if !json.Valid(out) {
		t.Fatalf("strict output is not JSON: %q", out)
	}
}

func TestRefusesInvalid(t *testing.T) {
	t.Parallel()
	d, _ := jsonc.Parse([]byte(`{"a":`))
	if _, err := Format(d, Style{}); !errors.Is(err, jsonc.ErrInvalid) {
		t.Fatalf("Format err = %v", err)
	}
	if _, err := Minify(d); !errors.Is(err, jsonc.ErrInvalid) {
		t.Fatalf("Minify err = %v", err)
	}
}

func TestMinify(t *testing.T) {
	t.Parallel()
	d, _ := jsonc.Parse([]byte("// c\n{\n  \"a\": [1, 2,], /* x */ \"b\": \"s p\"\n}\n"))
	got, err := Minify(d)
	if err != nil || string(got) != `{"a":[1,2],"b":"s p"}` {
		t.Fatalf("Minify = %q, %v", got, err)
	}
}

var seeds = []string{
	"{}", "[]", "{ }", "{\n}", "{\n  // only\n}", `{"a": 1,}`, "{\"a\": 1 /* c */}",
	"{\n  \"a\": 1, // note\n  \"b\": [1,\n    2]\n}\n", "{\r\n\t\"a\": 1\r\n}\r\n",
	"// head\n{\n  \"x\": {\n    \"y\": true\n  }\n} // tail\n", "\xef\xbb\xbf{\"a\":[]}",
	"{\"a\" /* k */ : // v\n 1 /* c */ , \"b\": 2}", "[1 // a\n /* b */ , 2]",
	"/*a\nb*/\r\n{\"a\": /* x\n y */ [{}, [], \"\\\"//\"]}", "\n\n// a\n\n\n// b\n[\n\n1\n\n,\n\n2\n\n]\n\n// c\n",
}

func FuzzFormat(f *testing.F) {
	texttest.Seed(f, seeds...)
	f.Fuzz(func(t *testing.T, b []byte) {
		for _, strict := range []bool{false, true} {
			var opts []jsonc.Option
			if strict {
				opts = append(opts, jsonc.Strict())
			}
			d, err := jsonc.Parse(b, opts...)
			if err != nil {
				if _, ferr := Format(d, Style{}); ferr == nil {
					t.Fatal("Format accepted an invalid document")
				}
				continue
			}
			checkFormat(t, d, Style{})
			checkFormat(t, d, Style{Tabs: true, TrailingCommas: true, NoFinalNewline: true})
		}
	})
}

func FuzzMinify(f *testing.F) {
	texttest.Seed(f, seeds...)
	f.Fuzz(func(t *testing.T, b []byte) {
		d, err := jsonc.Parse(b)
		got, merr := Minify(d)
		if err != nil {
			if merr == nil {
				t.Fatal("Minify accepted an invalid document")
			}
			return
		}
		if merr != nil || !bytes.Equal(got, compactJSON(t, d)) || !json.Valid(got) {
			t.Fatalf("Minify(%q) = %q, %v", b, got, merr)
		}
	})
}
