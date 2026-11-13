package schema_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ubgo/textdata/jsonc"
	"github.com/ubgo/textdata/jsonc/schema"
	"github.com/ubgo/textdata/plugin"
	"github.com/ubgo/textdata/pos"
)

func TestParseErrors(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, src, want string
		line            int
	}{
		{"not a schema", `[1]`, "must be an object or a boolean", 1},
		{"bad type name", "{\n  \"type\": \"text\"\n}", `unknown type "text"`, 2},
		{"type not string", `{"type": 7}`, "type: must be a string or an array of strings", 1},
		{"type element not string", `{"type": ["string", 7]}`, "type/1: must be a string", 1},
		{"required not array", `{"required": "name"}`, "required: must be an array of strings", 1},
		{"negative minLength", "{\n\"minLength\": -1}", "non-negative integer", 2},
		{"minimum not number", `{"minimum": "1"}`, "must be a number", 1},
		{"properties not object", `{"properties": []}`, "must be an object", 1},
		{"lookahead pattern", `{"pattern": "a(?=b)"}`, "RE2", 0},
		{"remote ref", `{"$ref": "https://example.com/s.json"}`, "only local references", 0},
		{"anchor ref", `{"$ref": "#foo"}`, "anchors are not supported", 0},
		{"missing def", `{"$ref": "#/$defs/nope"}`, "does not resolve", 0},
		{"empty anyOf", `{"anyOf": []}`, "non-empty array", 1},
		{"syntax", `{"type": }`, "unexpected", 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := schema.Parse([]byte(tc.src), jsonc.WithFileName("s.json"))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
			if tc.line > 0 {
				pe := pos.First(err)
				if pe == nil || pe.Pos.Line != tc.line || pe.File != "s.json" {
					t.Fatalf("want s.json line %d, got %#v", tc.line, pe)
				}
			}
		})
	}
	if _, err := schema.Parse([]byte(`{"type": 1}`)); !errors.Is(err, schema.ErrSchema) {
		t.Fatalf("errors must wrap ErrSchema: %v", err)
	}
}

func TestParseKeepsOrderAndExtras(t *testing.T) {
	t.Parallel()
	src := "{\n  // comments are fine\n  \"properties\": {\"z\": {}, \"a\": {\"x-custom\": [1, {\"b\": 2, \"a\": 1}]}},\n  \"if\": {\"type\": \"string\"},\n}"
	s, err := schema.Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(s.PropertyOrder, ",") != "z,a" {
		t.Fatalf("order %v", s.PropertyOrder)
	}
	b, _ := s.MarshalJSON()
	want := `{"properties":{"z":{},"a":{"x-custom":[1,{"b":2,"a":1}]}},"if":{"type":"string"}}`
	if string(b) != want {
		t.Fatalf("got  %s\nwant %s", b, want)
	}
	// The unsupported "if" is warned about, never silently passed.
	res := schema.Validate(mustDoc(t, `1`), s)
	if !res.OK() || len(res.Warnings) != 1 || res.Warnings[0].Keyword != "if" {
		t.Fatalf("warnings %v", res.Warnings)
	}
}

func TestLoadAndUnmarshal(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	file := filepath.Join(dir, "s.json")
	if err := os.WriteFile(file, []byte(`{"type": "string", "maxLength": 1}`), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := schema.Load(file)
	if err != nil {
		t.Fatal(err)
	}
	if res := schema.Validate(mustDoc(t, `"ab"`), s); res.OK() {
		t.Fatal("maxLength not loaded")
	}
	if _, err := schema.Load(filepath.Join(dir, "missing.json")); err == nil {
		t.Fatal("a missing schema file is an error")
	}
	var wrapped struct {
		S *schema.Schema `json:"s"`
	}
	d := mustDoc(t, `{"s": {"type": "integer"}}`)
	if err := d.Decode(&wrapped); err != nil || wrapped.S == nil || wrapped.S.Types[0] != "integer" {
		t.Fatalf("UnmarshalJSON: %v %+v", err, wrapped.S)
	}
}

// otherDoc is a plugin.Document that is not a *jsonc.Doc.
type otherDoc struct{ src string }

func (o otherDoc) FormatName() string            { return "other" }
func (o otherDoc) Location() string              { return "other.jsonc" }
func (o otherDoc) ValuePaths() []string          { return nil }
func (o otherDoc) ValueAt(string) (string, bool) { return "", false }
func (o otherDoc) Bytes() []byte                 { return []byte(o.src) }

func TestPlugin(t *testing.T) {
	t.Parallel()
	s := mustSchema(t, `{"properties":{"port":{"type":"integer"}},"additionalProperties":false}`)
	var warned []schema.Issue
	p := schema.Plugin(s, schema.OnWarning(func(i schema.Issue) { warned = append(warned, i) }))
	var _ plugin.Validator = p
	if p.Name() != schema.PluginName {
		t.Fatal(p.Name())
	}

	doc, err := jsonc.Parse([]byte(`{"port": "x", "extra": 1}`), jsonc.WithFileName("c.jsonc"), jsonc.WithPlugin(p))
	if err != nil {
		t.Fatal(err)
	}
	err = doc.Validate()
	if err == nil || !strings.Contains(err.Error(), `c.jsonc:1:10: port: must be integer, got string`) || !strings.Contains(err.Error(), `"schema"`) {
		t.Fatalf("doc.Validate() = %v", err)
	}
	if len(warned) != 1 || warned[0].Path != "extra" {
		t.Fatalf("warnings %v", warned)
	}

	if err := p.Validate(otherDoc{`{"port": 1.5}`}); err == nil || !strings.HasPrefix(pos.First(err).Error(), "other.jsonc:1:10:") {
		t.Fatalf("non-jsonc document: %v", err)
	}
	if err := p.Validate(otherDoc{`{"port": 1}`}); err != nil {
		t.Fatalf("valid document: %v", err)
	}
	if err := p.Validate(otherDoc{`{"port": `}); err == nil {
		t.Fatal("a document that does not parse is an error")
	}
}
