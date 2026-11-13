package generate_test

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ubgo/textdata/jsonc"
	"github.com/ubgo/textdata/jsonc/generate"
)

var update = flag.Bool("update", false, "rewrite golden files")

func render(t *testing.T, v any, opts ...generate.Option) []byte {
	t.Helper()
	f, err := generate.FromStruct(v, opts...)
	if err != nil {
		t.Fatalf("FromStruct: %v", err)
	}
	b, err := f.Bytes()
	if err != nil {
		t.Fatalf("Bytes: %v", err)
	}
	return b
}

// wantConfig is Config{} with every default applied, as encoding/json reads
// the generated file back.
func wantConfig() Config {
	workers := 4
	return Config{
		Base:     Base{Name: "app"},
		LogLevel: "info",
		Server:   Server{Addr: ":8080", ReadTimeout: 5 * time.Second, TLS: &TLS{}},
		Primary:  DB{Host: "localhost", Port: 5432},
		Replica:  &DB{Host: "localhost", Port: 5432},
		Hosts:    []string{"a.example", "b.example"},
		Limits:   map[string]int{"rps": 100, "burst": 20},
		Ratio:    0.5,
		Started:  time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
		Workers:  &workers,
		ID:       42,
		Labels:   map[string]string{},
	}
}

func TestFromStructGolden(t *testing.T) {
	t.Parallel()
	got := render(t, Config{}, generate.SchemaURL("./config.schema.json"))
	golden := filepath.Join("testdata", "config.golden.jsonc")
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
}

// TestFromStructRoundTrip pins the promises: the output parses with zero
// errors, and decoding it yields the struct with its defaults.
func TestFromStructRoundTrip(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		opts []generate.Option
		mode []jsonc.Option
	}{
		{"comments", nil, nil},
		{"tabs crlf trailing commas", []generate.Option{generate.Tabs(), generate.CRLF(), generate.TrailingCommas(true)}, nil},
		{"strict", []generate.Option{generate.Comments(false)}, []jsonc.Option{jsonc.Strict()}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			b := render(t, &Config{}, tc.opts...)
			doc, err := jsonc.Parse(b, tc.mode...)
			if err != nil {
				t.Fatalf("parse: %v\n%s", err, b)
			}
			var got Config
			if err := doc.Decode(&got); err != nil {
				t.Fatal(err)
			}
			if want := wantConfig(); !reflect.DeepEqual(got, want) {
				t.Fatalf("decoded\n%+v\nwant\n%+v", got, want)
			}
		})
	}
}

func TestFromStructStrictIsPlainJSON(t *testing.T) {
	t.Parallel()
	b := render(t, Config{}, generate.Comments(false))
	if !json.Valid(b) {
		t.Fatalf("not valid JSON:\n%s", b)
	}
	if bytes.Contains(b, []byte("//")) {
		t.Fatalf("comment left in:\n%s", b)
	}
}

func TestFromStructComments(t *testing.T) {
	t.Parallel()
	doc, err := jsonc.Parse(render(t, Config{}))
	if err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]string{
		"logLevel":           "Log verbosity.\nOne of: debug, info, warn, error.",
		"server.addr":        "Listen address.\nRequired, must not be empty.",
		"primary.password":   "Database password.\nRequired. Secret: do not commit a real value.",
		"server.readTimeout": "How long to wait for a request.\nDuration in nanoseconds.",
		"replica":            "Read replica.\nLeave as is to use the defaults.",
	} {
		got, err := doc.Comment(path)
		if err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Errorf("Comment(%q) = %q, want %q", path, got, want)
		}
	}
}

func TestFromStructValuesWin(t *testing.T) {
	const fakeSecret = "fake-value-1"
	t.Parallel()
	in := Config{LogLevel: "debug", Primary: DB{Port: 6543}}
	secretField := &in.Primary.Password
	*secretField = fakeSecret
	doc, err := jsonc.Parse(render(t, in))
	if err != nil {
		t.Fatal(err)
	}
	var got Config
	if err := doc.Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.LogLevel != "debug" || got.Primary.Port != 6543 || got.Primary.Host != "localhost" {
		t.Fatalf("v's values must win, defaults fill the rest: %+v", got)
	}
	if got.Primary.Password != "" {
		t.Fatal("a secret value must never be written")
	}
}

func TestFromStructErrors(t *testing.T) {
	t.Parallel()
	type badDefault struct {
		Port int `json:"port" default:"eighty"`
	}
	type badDefaultSet struct {
		Wait time.Duration `json:"wait" default:"soon"`
	}
	type badEnum struct {
		Level int `json:"level" enum:"1,two"`
	}
	type nested struct {
		Inner badDefault `json:"inner"`
	}
	type unsupported struct {
		C chan int `json:"c"`
	}
	for _, tc := range []struct {
		name string
		v    any
		want string
	}{
		{"bad default", badDefault{}, "field Port: default \"eighty\""},
		{"bad default even when set", badDefaultSet{Wait: time.Second}, "field Wait"},
		{"bad enum", badEnum{}, "field Level: enum \"two\""},
		{"nested path", nested{}, "field Inner.Port"},
		{"unsupported type", unsupported{}, "field C"},
		{"not a struct", 42, "needs a struct"},
		{"nil", nil, "needs a struct"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := generate.FromStruct(tc.v)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want it to contain %q", err, tc.want)
			}
		})
	}
}

type node struct {
	Name     string  `json:"name" default:"root"`
	Child    *node   `json:"child"`
	Children []*node `json:"children"`
}

func TestFromStructRecursive(t *testing.T) {
	t.Parallel()
	doc, err := jsonc.Parse(render(t, node{}))
	if err != nil {
		t.Fatal(err)
	}
	if k, _ := doc.Kind("child"); k != jsonc.KindNull {
		t.Fatalf("a nil pointer back to the type being walked must be null, got %v", k)
	}
}

type Inner struct {
	A string `json:"a"`
	B string
}

type clash struct {
	Inner
	B      string `json:"B"`
	Hidden string `json:"-"`
	*Other
}

type Other struct {
	A string `json:"a"`
	C int
}

// TestFromStructKeysMatchEncodingJSON: the key set is exactly the one
// encoding/json writes, embedded structs and name clashes included.
func TestFromStructKeysMatchEncodingJSON(t *testing.T) {
	t.Parallel()
	v := clash{Other: &Other{}}
	doc, err := jsonc.Parse(render(t, v))
	if err != nil {
		t.Fatal(err)
	}
	got, _ := doc.Keys("")
	b, _ := json.Marshal(v)
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{}
	for k := range m {
		want[k] = true
	}
	if len(got) != len(want) {
		t.Fatalf("keys %v, encoding/json writes %v", got, m)
	}
	for _, k := range got {
		if !want[k] {
			t.Fatalf("key %q is not one encoding/json writes (%v)", k, m)
		}
	}
}

func TestFromStructDoc(t *testing.T) {
	t.Parallel()
	f, err := generate.FromStruct(Config{})
	if err != nil {
		t.Fatal(err)
	}
	doc, err := f.Doc(jsonc.WithFileName("config.jsonc"))
	if err != nil {
		t.Fatal(err)
	}
	if err := doc.Set("server.addr", ":9090"); err != nil {
		t.Fatal(err)
	}
	if got, _ := doc.GetString("server.addr"); got != ":9090" {
		t.Fatalf("edit after generate: %q", got)
	}
}
