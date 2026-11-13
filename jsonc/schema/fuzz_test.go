package schema_test

import (
	"testing"

	"github.com/ubgo/textdata/jsonc"
	"github.com/ubgo/textdata/jsonc/schema"
)

// FuzzParseValidate: any schema text and any document never panic or hang;
// a schema that parses re-parses from its own JSON; issues carry positions
// inside the document.
func FuzzParseValidate(f *testing.F) {
	for _, seed := range [][2]string{
		{`{"type":"object","properties":{"a":{"type":"integer","maximum":3}},"required":["a"]}`, `{"a": 4}`},
		{`{"$defs":{"x":{"$ref":"#/$defs/x"}},"$ref":"#/$defs/x"}`, `1`},
		{`{"anyOf":[{"$ref":"#"},{"$ref":"#"}]}`, `[[[]]]`},
		{`{"items":{"pattern":"^a"},"minItems":1e400}`, `["b"]`},
		{`{"enum":[1e999999999,{"a":[]}],"const":-0}`, `-0.0`},
		{`{"additionalProperties":false,"oneOf":[true,false],"not":{}}`, "{\"a\":1, // c\n}"},
		{`true`, ``},
		{`{"properties":{"a.b":{"$ref":"#/properties/a.b"}}}`, `{"a.b":1}`},
	} {
		f.Add([]byte(seed[0]), []byte(seed[1]))
	}
	f.Fuzz(func(t *testing.T, schemaSrc, docSrc []byte) {
		s, err := schema.Parse(schemaSrc)
		if err != nil {
			return
		}
		b, err := s.JSON()
		if err != nil {
			t.Fatalf("JSON: %v", err)
		}
		if _, err := schema.Parse(b); err != nil {
			t.Fatalf("re-parse of %s: %v", b, err)
		}
		doc, _ := jsonc.Parse(docSrc)
		res := schema.Validate(doc, s)
		lines := 1
		for _, c := range docSrc {
			if c == '\n' {
				lines++
			}
		}
		for _, l := range [][]schema.Issue{res.Errors, res.Warnings} {
			for _, i := range l {
				if i.Pos.Line > lines {
					t.Fatalf("issue past the end of the document: %v", i)
				}
			}
		}
	})
}
