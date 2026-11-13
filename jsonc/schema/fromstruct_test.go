package schema_test

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ubgo/textdata/jsonc"
	"github.com/ubgo/textdata/jsonc/generate"
	"github.com/ubgo/textdata/jsonc/schema"
)

var update = flag.Bool("update", false, "rewrite golden files")

type Base struct {
	Name string `json:"name" default:"app" doc:"Application name."`
}

type DB struct {
	Host     string `json:"host" default:"localhost" doc:"Database host."`
	Port     uint16 `json:"port" default:"5432"`
	Password string `json:"password" env:"DB_PASSWORD,required" secret:"true" default:"never-shown"`
}

type TLS struct {
	Cert string `json:"cert"`
}

type Server struct {
	Addr        string        `json:"addr" env:"ADDR,notempty" default:":8080" doc:"Listen address."`
	ReadTimeout time.Duration `json:"readTimeout" default:"5s"`
	TLS         *TLS          `json:"tls"`
}

type Tree struct {
	Label    string `json:"label"`
	Children []Tree `json:"children"`
}

type Config struct {
	Base
	LogLevel string            `json:"logLevel" enum:"debug,info,warn,error" default:"info" doc:"Log verbosity."`
	Server   Server            `json:"server" doc:"HTTP server."`
	Primary  DB                `json:"primary"`
	Replica  *DB               `json:"replica"`
	Hosts    []string          `json:"hosts" default:"a,b"`
	Ports    [2]int            `json:"ports"`
	Limits   map[string]int    `json:"limits" default:"rps:100"`
	Ratio    float64           `json:"ratio" default:"0.5"`
	Debug    bool              `json:"debug"`
	Started  time.Time         `json:"started"`
	Workers  *int              `json:"workers" default:"4"`
	ID       int64             `json:"id,string" default:"42"`
	Blob     []byte            `json:"blob"`
	Tree     Tree              `json:"tree"`
	Labels   map[string]string `json:"labels,omitempty"`
	Extra    any               `json:"extra"`
	Token    string            `json:"token" required:"true"`
	Skip     string            `json:"-"`
	hidden   string
}

func TestFromStructGolden(t *testing.T) {
	t.Parallel()
	s, err := schema.FromStruct(Config{}, schema.WithID("https://example.com/config.schema.json"), schema.WithTitle("Config"))
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.JSON()
	if err != nil {
		t.Fatal(err)
	}
	golden := filepath.Join("testdata", "config.schema.json")
	if *update {
		if err := os.WriteFile(golden, got, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("output differs from %s (run with -update):\n%s", golden, got)
	}
	// What FromStruct writes, Parse reads back to the same JSON.
	again, err := schema.Parse(got)
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := again.JSON(); !bytes.Equal(b, got) {
		t.Fatalf("Parse(JSON()) changed the schema:\n%s", b)
	}
}

// TestGenerateValidatesClean: the file generate.FromStruct writes validates
// with no errors and no warnings against schema.FromStruct of the same
// struct, with and without comments, and with "$schema" set.
func TestGenerateValidatesClean(t *testing.T) {
	t.Parallel()
	s, err := schema.FromStruct(&Config{})
	if err != nil {
		t.Fatal(err)
	}
	for _, opts := range [][]generate.Option{
		nil,
		{generate.Comments(false)},
		{generate.SchemaURL("./config.schema.json"), generate.TrailingCommas(true)},
	} {
		f, err := generate.FromStruct(Config{Token: "t"}, opts...)
		if err != nil {
			t.Fatal(err)
		}
		doc, err := f.Doc()
		if err != nil {
			t.Fatal(err)
		}
		res := schema.Validate(doc, s)
		if !res.OK() || len(res.Warnings) > 0 {
			b, _ := f.Bytes()
			t.Fatalf("errors %v warnings %v\n%s", res.Errors, res.Warnings, b)
		}
	}
}

// TestGenerateThenBreak: the same pair catches what a user gets wrong.
func TestGenerateThenBreak(t *testing.T) {
	t.Parallel()
	s, err := schema.FromStruct(Config{})
	if err != nil {
		t.Fatal(err)
	}
	f, err := generate.FromStruct(Config{})
	if err != nil {
		t.Fatal(err)
	}
	doc, err := f.Doc(jsonc.WithFileName("config.jsonc"))
	if err != nil {
		t.Fatal(err)
	}
	for _, edit := range []struct {
		path string
		v    any
	}{
		{"logLevel", "loud"},
		{"primary.port", 70000},
		{"server.addr", ""},
		{"replica", nil},
		{"tpyo", 1},
	} {
		if err := doc.Set(edit.path, edit.v); err != nil {
			t.Fatal(err)
		}
	}
	if err := doc.Delete("token"); err != nil {
		t.Fatal(err)
	}
	res := schema.Validate(doc, s)
	var got []string
	for _, e := range res.Errors {
		got = append(got, e.Path+" "+e.Keyword)
	}
	joined := strings.Join(got, "|")
	for _, w := range []string{"logLevel enum", "primary.port maximum", "server.addr minLength", " required"} {
		if !strings.Contains(joined, w) {
			t.Errorf("missing %q in %q", w, joined)
		}
	}
	if strings.Contains(joined, "replica") {
		t.Errorf("a nil section (null) is valid: %q", joined)
	}
	if len(res.Warnings) != 1 || res.Warnings[0].Path != "tpyo" {
		t.Errorf("unknown key must be one warning: %v", res.Warnings)
	}
}

func TestFromStructShapes(t *testing.T) {
	t.Parallel()
	s, err := schema.FromStruct(Config{})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := s.JSON()
	out := string(b)
	for _, want := range []string{
		`"$schema": "https://json-schema.org/draft/2020-12/schema"`,
		`"$ref": "#/$defs/DB"`,
		`"$ref": "#/$defs/Tree"`,
		`"format": "date-time"`,
		`"contentEncoding": "base64"`,
		`"default": "42"`,
		`"default": 5000000000`,
		`"minItems": 2`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("schema lacks %s", want)
		}
	}
	if strings.Contains(out, "never-shown") {
		t.Error("a secret's default must not be written")
	}
	if strings.Contains(out, `"Skip"`) || strings.Contains(out, "hidden") {
		t.Error("skipped fields leaked")
	}
}

func TestFromStructErrors(t *testing.T) {
	t.Parallel()
	type badDefault struct {
		Port int `json:"port" default:"x"`
	}
	type badEnum struct {
		Wait time.Duration `json:"wait" enum:"1s,later"`
	}
	type unsupported struct {
		F func() `json:"f"`
	}
	for _, tc := range []struct {
		v    any
		want string
	}{
		{badDefault{}, "field Port: default"},
		{badEnum{}, "field Wait: enum \"later\""},
		{unsupported{}, "field F"},
		{"str", "needs a struct"},
		{nil, "needs a struct"},
	} {
		if _, err := schema.FromStruct(tc.v); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("FromStruct(%T) = %v, want %q", tc.v, err, tc.want)
		}
	}
}

func TestFromStructAllowAdditional(t *testing.T) {
	t.Parallel()
	s, err := schema.FromStruct(Base{}, schema.AllowAdditional())
	if err != nil {
		t.Fatal(err)
	}
	res := schema.Validate(mustDoc(t, `{"name":"x","other":1}`), s)
	if !res.OK() || len(res.Warnings) != 0 {
		t.Fatalf("AllowAdditional: %v %v", res.Errors, res.Warnings)
	}
}
