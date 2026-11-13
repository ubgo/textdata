// Command schema-plugin validates a JSONC config against a JSON Schema built
// from the app's config struct: it writes the schema for editors, loads it
// back, validates a file (errors and warnings with file:line:col), runs the
// schema as a plugin through doc.Validate, and refuses to save an edit that
// breaks the schema.
//
// Run it with `go run .`; it writes into a temporary directory and prints
// each stage to stdout.
package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/ubgo/textdata/jsonc"
	"github.com/ubgo/textdata/jsonc/schema"
)

// File names inside the work directory.
const (
	schemaName = "config.schema.json"
	configName = "config.jsonc"
)

// filePerm is the mode of every file the program writes.
const filePerm = 0o644

// Config is the application's config struct; its tags drive the schema.
//
//ds:def id=jt-schema-config-zyjqzp98
type Config struct {
	Name     string `json:"name" env:"NAME,required" doc:"Service name."`
	LogLevel string `json:"logLevel" enum:"debug,info,warn,error" default:"info"`
	Port     uint16 `json:"port" default:"8080"`
}

// config is the file on disk: the port is out of range, the log level is
// not in the enum, and "tpyo" is an unknown key.
const config = `{
  // service
  "name": "api",
  "logLevel": "verbose",
  "port": 70000,
  "tpyo": true
}
`

// handSchema is a schema written by hand, as JSONC: comments are allowed.
const handSchema = `{
  // only a lowercase name is accepted
  "type": "object",
  "properties": {"name": {"type": "string", "pattern": "^[a-z]+$"}},
  "required": ["name"]
}`

func main() {
	dir, err := os.MkdirTemp("", "schema-plugin-")
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

// run performs every step in dir and prints each under a heading so the
// test can pin the output.
func run(w io.Writer, dir string) error {
	schemaPath := filepath.Join(dir, schemaName)
	configPath := filepath.Join(dir, configName)
	if err := os.WriteFile(configPath, []byte(config), filePerm); err != nil {
		return err
	}
	if err := writeSchema(schemaPath); err != nil {
		return err
	}
	b, err := os.ReadFile(schemaPath)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "== %s\n%s", schemaName, b)
	s, err := schema.Load(schemaPath)
	if err != nil {
		return err
	}
	fmt.Fprintln(w, "== Validate")
	if err := validateFile(w, configPath, s); err != nil {
		return err
	}
	fmt.Fprintln(w, "== Parse a hand-written schema")
	if err := validateWithHandSchema(w); err != nil {
		return err
	}
	fmt.Fprintln(w, "== Plugin")
	if err := validateAsPlugin(w, configPath, s); err != nil {
		return err
	}
	fmt.Fprintln(w, "== SaveGuard")
	return guardSave(w, configPath, s)
}

// writeSchema builds the schema from the struct and writes it next to the
// config, where editors pick it up through "$schema" (a go:generate step).
//
//ds:def id=jt-schema-write-zcrt59ud
func writeSchema(name string) error {
	s, err := schema.FromStruct(Config{}, schema.WithTitle("app config"))
	if err != nil {
		return err // a default or enum tag that does not parse names the field
	}
	b, err := s.JSON()
	if err != nil {
		return err
	}
	return os.WriteFile(name, b, filePerm)
}

// validateFile checks a file against s. Errors fail validation; warnings
// (unknown keys) only point something out. Both carry file:line:col.
//
//ds:def id=jt-schema-validate-jtqgg2uk
func validateFile(w io.Writer, name string, s *schema.Schema) error {
	d, err := jsonc.Open(name)
	if err != nil {
		return err
	}
	res := schema.Validate(d, s, schema.WithFileName(configName))
	for _, e := range res.Errors {
		fmt.Fprintf(w, "error [%s]: %v\n", e.Keyword, e)
	}
	for _, x := range res.Warnings {
		fmt.Fprintf(w, "warning [%s]: %v\n", x.Keyword, x)
	}
	fmt.Fprintf(w, "ok: %t\n", res.OK())
	return nil
}

// validateWithHandSchema parses a schema from JSONC text. Parse checks the
// schema itself (keyword types, $ref, pattern); errors wrap ErrSchema.
//
//ds:def id=jt-schema-parse-fy6ebs7g
func validateWithHandSchema(w io.Writer) error {
	s, err := schema.Parse([]byte(handSchema))
	if err != nil {
		return err
	}
	d, err := jsonc.Parse([]byte(`{"name": "API"}`), jsonc.WithFileName("inline.jsonc"))
	if err != nil {
		return err
	}
	fmt.Fprintln(w, schema.Validate(d, s).Err())

	// pattern is a Go regexp (RE2): lookaround is refused when parsing.
	_, err = schema.Parse([]byte(`{"pattern": "^(?!admin)"}`))
	fmt.Fprintf(w, "bad schema: %v (ErrSchema: %t)\n", err, errors.Is(err, schema.ErrSchema))
	return nil
}

// validateAsPlugin installs the schema as a plugin.Validator: doc.Validate
// runs it on demand (never on its own). Warnings cannot travel through the
// returned error, so OnWarning receives them.
//
//ds:def id=jt-schema-plugin-tmevwhee
func validateAsPlugin(w io.Writer, name string, s *schema.Schema) error {
	onWarning := schema.OnWarning(func(i schema.Issue) {
		fmt.Fprintln(w, "warning:", i.Message)
	})
	checker := schema.Plugin(s, onWarning, schema.WithFileName(configName))
	d, err := jsonc.Open(name, jsonc.WithPlugin(checker))
	if err != nil {
		return err
	}
	fmt.Fprintln(w, "validate:", d.Validate())
	return nil
}

// guardSave installs the schema plugin, which is also a plugin.SaveGuard:
// it fixes the file and saves it, then makes a bad edit, and the second Save
// is refused while the file on disk keeps the good version.
//
//ds:def id=jt-schema-save-4fhp9stf
func guardSave(w io.Writer, name string, s *schema.Schema) error {
	checker := schema.Plugin(s, schema.WithFileName(configName))
	d, err := jsonc.Open(name, jsonc.WithPlugin(checker))
	if err != nil {
		return err
	}
	if err := errors.Join(d.Set("logLevel", "warn"), d.Set("port", 8443), d.Delete("tpyo")); err != nil {
		return err
	}
	fmt.Fprintln(w, "save fixed:", d.Save())
	if err := d.Set("port", -1); err != nil {
		return err
	}
	fmt.Fprintln(w, "save broken:", withoutDir(d.Save(), filepath.Dir(name)))
	onDisk, err := jsonc.Open(name)
	if err != nil {
		return err
	}
	port, err := onDisk.GetInt("port")
	fmt.Fprintln(w, "port on disk:", port)
	return err
}

// withoutDir drops the temporary directory from an error so the printed
// output is the same on every run.
func withoutDir(err error, dir string) string {
	if err == nil {
		return "<nil>"
	}
	return strings.ReplaceAll(err.Error(), dir+string(filepath.Separator), "")
}
