// Command expand reads ${VAR}, ${scheme:ref} and $(command) references in a
// JSONC config. Expansion is opt-in and never changes the document: the
// expanded values come from a copy, while Get, Render and Save keep the
// references as written. Every value source here is a fake the program owns.
package main

import (
	"errors"
	"fmt"
	"io"
	"log"
	"os"

	"github.com/ubgo/textdata/expand"
	"github.com/ubgo/textdata/jsonc"
)

// configName is the file name every step gives its document, so errors read
// app.jsonc:line:col.
const configName = "app.jsonc"

// src uses the reference forms jsonc supports in string values.
//
//ds:def id=src-62pdqreb
const src = `{
  // ${HOST} comes from the lookup; PORT falls back to 5432
  "db": {
    "url": "postgres://${HOST}:${PORT:-5432}/app",
    "password": "${vault:db/password}",
  },
  "build": "$(git rev-parse --short HEAD)",
  "price": "$$5",
}
`

// vars is the fake environment the lookup reads; jsonc never reads the real
// process environment unless you pass os.LookupEnv yourself.
//
//ds:def id=vars-h9p7r6fq
var vars = map[string]string{"HOST": "db.internal"}

// lookup is the WithLookup callback over vars.
//
//ds:def id=lookup-cyykhfrd
func lookup(name string) (string, bool) {
	v, ok := vars[name]
	return v, ok
}

func main() {
	dir, err := os.MkdirTemp("", "expand-")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(dir)
	if err := run(os.Stdout, dir); err != nil {
		log.Fatal(err)
	}
}

// run executes every step in guide order; dir is a scratch folder for the
// one-call Read.
func run(w io.Writer, dir string) error {
	if err := expandVars(w); err != nil {
		return err
	}
	if err := requireVar(w); err != nil {
		return err
	}
	if err := withPlugins(w); err != nil {
		return err
	}
	if err := commandSubstitution(w); err != nil {
		return err
	}
	return readExpanded(w, dir)
}

// expandVars expands through a lookup. Expanded returns a copy; the document
// keeps the reference, which is what Save would write. $(command) is left as
// written because command substitution is off.
//
//ds:def id=expandvars-uq8v3dfv
func expandVars(w io.Writer) error {
	doc, err := jsonc.Parse([]byte(src), jsonc.WithLookup(lookup))
	if err != nil {
		return err
	}
	x, err := doc.Expanded()
	if err != nil {
		return err
	}
	url, err := x.GetString("db.url")
	if err != nil {
		return err
	}
	raw, err := doc.GetString("db.url")
	if err != nil {
		return err
	}
	build, err := doc.GetExpanded("build")
	if err != nil {
		return err
	}
	price, err := doc.GetExpanded("price")
	if err != nil {
		return err
	}
	fmt.Fprintln(w, "expanded:", url)
	fmt.Fprintln(w, "raw:     ", raw)
	fmt.Fprintln(w, "build (commands off):", build, "| price:", price)
	return nil
}

// requireVar shows ${VAR:?message}: an unset variable is an error with the
// value's file:line:col, unwrapping to expand.RequiredError.
//
//ds:def id=requirevar-fxndfpjq
func requireVar(w io.Writer) error {
	doc, err := jsonc.Parse([]byte("{\n  \"token\": \"${API_TOKEN:?set API_TOKEN}\"\n}"),
		jsonc.WithFileName(configName), jsonc.WithLookup(lookup))
	if err != nil {
		return err
	}
	_, err = doc.GetExpanded("token")
	var req *expand.RequiredError
	if !errors.As(err, &req) {
		return fmt.Errorf("want a RequiredError, got %v", err)
	}
	fmt.Fprintln(w, "required:", err)
	fmt.Fprintln(w, "missing key:", req.Key)
	return nil
}
