package schema_test

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/ubgo/textdata/jsonc"
	"github.com/ubgo/textdata/jsonc/schema"
	"github.com/ubgo/textdata/pos"
)

// summary renders issues as "line:col path keyword" for compact assertions.
func summary(l []schema.Issue) []string {
	out := []string{}
	for _, i := range l {
		out = append(out, fmt.Sprintf("%s %s %s", i.Pos, i.Path, i.Keyword))
	}
	return out
}

func mustSchema(t *testing.T, src string) *schema.Schema {
	t.Helper()
	s, err := schema.Parse([]byte(src))
	if err != nil {
		t.Fatalf("schema.Parse: %v", err)
	}
	return s
}

func mustDoc(t *testing.T, src string) *jsonc.Doc {
	t.Helper()
	d, err := jsonc.Parse([]byte(src), jsonc.WithFileName("cfg.jsonc"))
	if err != nil {
		t.Fatalf("jsonc.Parse: %v", err)
	}
	return d
}

func TestValidateKeywords(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, schema, doc string
		errs, warns       []string
	}{
		{"type pass", `{"type":"object"}`, `{}`, nil, nil},
		{"type fail", `{"type":"array"}`, `{}`, []string{"1:1  type"}, nil},
		{"type list", `{"type":["string","null"]}`, `null`, nil, nil},
		{"integer accepts 1.0", `{"type":"integer"}`, `1.0`, nil, nil},
		{"integer rejects 1.5", `{"type":"integer"}`, `1.5`, []string{"1:1  type"}, nil},
		{"number accepts integer", `{"type":"number"}`, `7`, nil, nil},
		{"big integer exact", `{"type":"integer","maximum":9007199254740993}`, `9007199254740993`, nil, nil},
		{"big integer over by one", `{"maximum":9007199254740992}`, `9007199254740993`, []string{"1:1  maximum"}, nil},
		{"huge exponent", `{"type":"number","maximum":1}`, `1e999999999`, []string{"1:1  maximum"}, nil},
		{"enum pass", `{"enum":["a",1,null,{"k":[1]}]}`, `{"k":[1.0]}`, nil, nil},
		{"enum fail", `{"enum":["a","b"]}`, `"c"`, []string{"1:1  enum"}, nil},
		{"const pass", `{"const":{"a":1,"b":2}}`, `{"b":2,/*c*/"a":1}`, nil, nil},
		{"const fail", `{"const":5}`, `6`, []string{"1:1  const"}, nil},
		{"minimum", `{"minimum":1}`, `0`, []string{"1:1  minimum"}, nil},
		{"minimum equal passes", `{"minimum":1}`, `1`, nil, nil},
		{"exclusiveMinimum", `{"exclusiveMinimum":1}`, `1`, []string{"1:1  exclusiveMinimum"}, nil},
		{"maximum", `{"maximum":1.5}`, `1.6`, []string{"1:1  maximum"}, nil},
		{"exclusiveMaximum", `{"exclusiveMaximum":2}`, `2`, []string{"1:1  exclusiveMaximum"}, nil},
		{"bounds ignore strings", `{"minimum":5}`, `"x"`, nil, nil},
		{"minLength counts runes", `{"minLength":2}`, `"é"`, []string{"1:1  minLength"}, nil},
		{"maxLength", `{"maxLength":2}`, `"abc"`, []string{"1:1  maxLength"}, nil},
		{"pattern unanchored", `{"pattern":"b+"}`, `"abbc"`, nil, nil},
		{"pattern fail", `{"pattern":"^[a-z]+$"}`, `"ab1"`, []string{"1:1  pattern"}, nil},
		{"items", `{"items":{"type":"integer"}}`, "[1,\n \"x\", 3]", []string{"2:2 [1] type"}, nil},
		{"minItems", `{"minItems":2}`, `[1]`, []string{"1:1  minItems"}, nil},
		{"maxItems", `{"maxItems":1}`, `[1,2]`, []string{"1:1  maxItems"}, nil},
		{"required", `{"required":["a","b"]}`, `{"a":1}`, []string{"1:1  required"}, nil},
		{"properties", `{"properties":{"a":{"type":"string"}}}`, "{\n  \"a\": 1\n}", []string{"2:8 a type"}, nil},
		{"additional false warns", `{"properties":{"a":{}},"additionalProperties":false}`, "{\n  \"a\": 1,\n  \"b\": 2\n}", nil, []string{"3:3 b additionalProperties"}},
		{"additional schema errors", `{"additionalProperties":{"type":"string"}}`, `{"x":1}`, []string{"1:6 x type"}, nil},
		{"false schema", `{"properties":{"a":false}}`, `{"a":1}`, []string{"1:6 a false"}, nil},
		{"ref into defs", `{"$defs":{"port":{"type":"integer","maximum":65535}},"properties":{"p":{"$ref":"#/$defs/port"}}}`, `{"p":70000}`, []string{"1:6 p maximum"}, nil},
		{"ref to root", `{"properties":{"child":{"$ref":"#"}},"required":["n"]}`, `{"n":1,"child":{"n":2,"child":{}}}`, []string{"1:31 child.child required"}, nil},
		{"ref siblings apply", `{"$defs":{"s":{"type":"string"}},"properties":{"a":{"$ref":"#/$defs/s","minLength":3}}}`, `{"a":"x"}`, []string{"1:6 a minLength"}, nil},
		{"anyOf pass", `{"anyOf":[{"type":"string"},{"type":"null"}]}`, `null`, nil, nil},
		{"anyOf fail", `{"anyOf":[{"type":"string"},{"type":"null"}]}`, `1`, []string{"1:1  anyOf"}, nil},
		{"anyOf reports the typed branch", `{"anyOf":[{"properties":{"p":{"maximum":1}}},{"type":"null"}]}`, `{"p":2}`, []string{"1:6 p maximum"}, nil},
		{"oneOf two match", `{"oneOf":[{"type":"integer"},{"type":"number"}]}`, `1`, []string{"1:1  oneOf"}, nil},
		{"oneOf one match", `{"oneOf":[{"type":"integer"},{"type":"string"}]}`, `1`, nil, nil},
		{"allOf", `{"allOf":[{"minimum":1},{"maximum":3}]}`, `5`, []string{"1:1  maximum"}, nil},
		{"not", `{"not":{"type":"null"}}`, `null`, []string{"1:1  not"}, nil},
		{"unsupported keyword warns once", `{"properties":{"a":{"uniqueItems":true}}}`, `{"a":[1,1]}`, nil, []string{"1:6 a uniqueItems"}},
		{"annotations ignored", `{"title":"t","description":"d","default":1,"examples":[1],"format":"date-time"}`, `"x"`, nil, nil},
		{"true schema", `true`, `[1]`, nil, nil},
		{"false root", `false`, `1`, []string{"1:1  false"}, nil},
		{"empty document", `{"type":"object"}`, ``, []string{"-  "}, nil},
		{"keys that look like pointers", `{"properties":{"/x":{"type":"string"},"a~b":{"type":"string"},"":{"type":"string"}}}`, `{"/x":1,"a~b":2,"":3}`, []string{`1:7 "/x" type`, `1:15 a~b type`, `1:20 "" type`}, nil},
		{"duplicate key: last wins", `{"properties":{"a":{"type":"string"}}}`, `{"a":1,"a":"x"}`, nil, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s := mustSchema(t, tc.schema)
			d, _ := jsonc.Parse([]byte(tc.doc))
			res := schema.Validate(d, s)
			if got := summary(res.Errors); !reflect.DeepEqual(got, orEmpty(tc.errs)) {
				t.Errorf("errors %q, want %q (%v)", got, orEmpty(tc.errs), res.Errors)
			}
			if got := summary(res.Warnings); !reflect.DeepEqual(got, orEmpty(tc.warns)) {
				t.Errorf("warnings %q, want %q", got, orEmpty(tc.warns))
			}
			if (res.Err() == nil) != (len(tc.errs) == 0) || res.OK() != (len(tc.errs) == 0) {
				t.Errorf("Err() = %v with %d errors", res.Err(), len(tc.errs))
			}
		})
	}
}

func orEmpty(l []string) []string {
	if l == nil {
		return []string{}
	}
	return l
}

func TestValidateMessages(t *testing.T) {
	t.Parallel()
	s := mustSchema(t, `{
	  "type": "object",
	  "required": ["name"],
	  "properties": {"server": {"properties": {"port": {"type": "integer", "maximum": 65535}}}},
	  "additionalProperties": false
	}`)
	d := mustDoc(t, "{\n  // port\n  \"server\": {\"port\": 70000},\n  \"extra\": true\n}\n")
	res := schema.Validate(d, s)
	var got []string
	for _, i := range res.Errors {
		got = append(got, i.Error())
	}
	want := []string{
		`cfg.jsonc:1:1: (root): missing required key "name"`,
		`cfg.jsonc:3:22: server.port: must be <= 65535`,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("errors\n%q\nwant\n%q", got, want)
	}
	if len(res.Warnings) != 1 || res.Warnings[0].Error() != `cfg.jsonc:4:3: extra: unknown key "extra"` {
		t.Fatalf("warnings %v", res.Warnings)
	}
	var list pos.ErrorList
	if !errors.As(res.Err(), &list) || len(list) != 2 || list[1].Pos.Line != 3 {
		t.Fatalf("Err() must be a pos.ErrorList: %#v", res.Err())
	}
	// An explicit name overrides the document's.
	res = schema.Validate(d, s, schema.WithFileName("other.jsonc"))
	if !strings.HasPrefix(res.Errors[0].Error(), "other.jsonc:1:1:") {
		t.Fatalf("WithFileName: %v", res.Errors[0])
	}
}

func TestValidateSyntaxErrors(t *testing.T) {
	t.Parallel()
	d, _ := jsonc.Parse([]byte("{\n  \"a\": 1\n  \"b\": 2\n}"), jsonc.WithFileName("bad.jsonc"))
	res := schema.Validate(d, mustSchema(t, `true`))
	if len(res.Errors) == 0 || res.Errors[0].Keyword != "syntax" || res.Errors[0].Pos.Line != 3 || res.Errors[0].File != "bad.jsonc" {
		t.Fatalf("syntax errors must come through with positions: %v", res.Errors)
	}
}

func TestValidateNilSchema(t *testing.T) {
	t.Parallel()
	if res := schema.Validate(mustDoc(t, `{"a":1}`), nil); !res.OK() || len(res.Warnings) != 0 {
		t.Fatalf("nil schema accepts everything: %v", res)
	}
}

func TestValidateRefLoop(t *testing.T) {
	t.Parallel()
	for _, src := range []string{
		`{"$defs":{"a":{"$ref":"#/$defs/b"},"b":{"$ref":"#/$defs/a"}},"$ref":"#/$defs/a"}`,
		`{"anyOf":[{"$ref":"#"},{"$ref":"#"}]}`,
		`{"allOf":[{"$ref":"#"}]}`,
		// Four branches per level: 4^16 paths within the hop bound, so only
		// the check budget stops it.
		`{"anyOf":[{"$ref":"#"},{"$ref":"#"},{"$ref":"#"},{"$ref":"#"}]}`,
	} {
		res := schema.Validate(mustDoc(t, `1`), mustSchema(t, src))
		if res.OK() {
			t.Fatalf("a reference loop must end in an error: %s", src)
		}
	}
}

func TestValidateDoesNotModify(t *testing.T) {
	t.Parallel()
	src := `{"properties":{"a":{"pattern":"x","$ref":"#/$defs/d"}},"$defs":{"d":{"minimum":1}}}`
	s := mustSchema(t, src)
	before, _ := s.JSON()
	d := mustDoc(t, `{"a":0}`)
	docBefore := d.Render()
	schema.Validate(d, s)
	after, _ := s.JSON()
	if string(before) != string(after) || string(docBefore) != string(d.Render()) {
		t.Fatal("Validate must not modify the schema or the document")
	}
}
