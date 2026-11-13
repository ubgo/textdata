// Command generate-from-struct writes a commented config file the way an
// `app init` command would: once straight from the app's config struct
// (generate.FromStruct, driven by the struct tags cfgkit reads), and once by
// hand with the builder API. It then reads the written file back to show
// that decoding it yields the struct with every default filled in.
//
// Run it with `go run .`; it writes into a temporary directory and prints
// each stage to stdout.
package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/ubgo/textdata/jsonc"
	"github.com/ubgo/textdata/jsonc/generate"
)

// Names the program writes and points the file at.
const (
	configName = "config.jsonc"
	schemaURL  = "./config.schema.json"
)

// filePerm is the mode of the generated file: a config is not a secret
// store, but the password key in it should never hold a real value.
const filePerm = 0o644

// Config is the application's config struct. The tags are the ones cfgkit
// reads; FromStruct turns them into keys, values and comments.
//
//ds:def id=jt-gen-config-2tsxk8dz
type Config struct {
	LogLevel string        `json:"logLevel" enum:"debug,info,warn,error" default:"info" doc:"Log verbosity."`
	Addr     string        `json:"addr" env:"ADDR,notempty" default:":8080" doc:"Listen address."`
	Timeout  time.Duration `json:"timeout" default:"5s" doc:"Request timeout."`
	Hosts    []string      `json:"hosts" default:"a.example,b.example"`
	Password string        `json:"password" env:"DB_PASSWORD,required" secret:"true" default:"changeme"`
	DB       DB            `json:"db" doc:"Primary database."`
}

// DB is a nested section: it becomes a nested object.
type DB struct {
	Host string `json:"host" default:"localhost"`
	Port int    `json:"port" default:"5432"`
}

func main() {
	dir, err := os.MkdirTemp("", "generate-from-struct-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer os.RemoveAll(dir)
	if err := run(os.Stdout, dir); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// run writes the generated file into dir and prints every stage under a
// heading so the test can pin the output.
func run(w io.Writer, dir string) error {
	name := filepath.Join(dir, configName)
	if err := writeTemplate(name); err != nil {
		return err
	}
	b, err := os.ReadFile(name)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "== FromStruct -> %s\n%s", configName, b)

	cfg, err := readBack(name)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "== decoded\n%+v\n", cfg)

	hand, err := buildByHand()
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "== by hand, tabs, trailing commas\n%s", hand)

	plain, err := plainJSON()
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "== Comments(false)\n%s", plain)
	return nil
}

// writeTemplate is the body of an `init` command: build the file from the
// zero Config, so every key shows its default, and write it out.
//
//ds:def id=jt-gen-template-43vpkj7x
func writeTemplate(name string) error {
	f, err := generate.FromStruct(Config{}, generate.SchemaURL(schemaURL))
	if err != nil {
		return err // a default or enum tag that does not parse names the field
	}
	b, err := f.Bytes()
	if err != nil {
		return err
	}
	return os.WriteFile(name, b, filePerm)
}

// readBack opens the written file and decodes it with encoding/json rules:
// the result is Config with the defaults filled in.
//
//ds:def id=jt-gen-readback-bvrqdekr
func readBack(name string) (Config, error) {
	var cfg Config
	d, err := jsonc.Open(name)
	if err != nil {
		return cfg, err
	}
	err = d.Decode(&cfg)
	return cfg, err
}

// buildByHand uses the builder API for a file that has no struct behind it:
// a header, sections with comments, a blank line, a verbatim JSONC value.
//
//ds:def id=jt-gen-byhand-6vwrn8qr
func buildByHand() ([]byte, error) {
	f := generate.New(generate.Tabs(), generate.TrailingCommas(true))
	f.Header("Created by `app init`. Edit freely.")
	srv := f.Object("server", generate.Doc("HTTP server"))
	srv.Set("port", 8080, generate.Trailing("default"))
	srv.Set("host", "0.0.0.0")
	f.Blank()
	f.Comment("Hosts allowed to connect.")
	f.Blank()
	f.Array("hosts").Add("a.example").Add("b.example")
	f.Set("retry", jsonc.Raw(`[1, /* then */ 5]`), generate.BlankBefore())
	return f.Bytes()
}

// plainJSON turns comments off: with no trailing commas the output is plain
// JSON that jsonc.Strict() and encoding/json accept.
//
//ds:def id=jt-gen-plain-2w8drnge
func plainJSON() ([]byte, error) {
	f, err := generate.FromStruct(DB{}, generate.Comments(false), generate.Spaces(4))
	if err != nil {
		return nil, err
	}
	return f.Bytes()
}
