// Command strict-expand reads a JSONC config into a typed struct with
// jsonc.WithStrictExpand: references resolve as usual, and a reference that
// would otherwise read as "" (a typo'd variable, a scheme no plugin claims, a
// $(command) with commands off) fails with the file:line:col of the string
// and a typed expand error. The values are made up.
package main

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/ubgo/textdata/expand"
	"github.com/ubgo/textdata/jsonc"
	"github.com/ubgo/textdata/pos"
)

// Config is the typed shape the file decodes into.
type Config struct {
	Default string `json:"default"`
	Token   string `json:"token"`
	Region  string `json:"region"`
}

// The files: one whose references all resolve, one with a typo.
const (
	goodFile = "good.jsonc"
	good     = `{
  "default": "claude",
  "token": "${API_TOKEN}",          // from the lookup
  "region": "${REGION:-eu-west-1}"  // a default still applies
}
`
	typoFile = "typo.jsonc"
	typo     = `{
  "default": "claude",
  "token": "${API_TOKN}"
}
`
)

// env is the variable source handed to WithLookup.
var env = map[string]string{"API_TOKEN": "fake-token"}

func main() {
	if err := run(os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// run loads both files.
func run(w io.Writer) error {
	cfg, err := load(goodFile, good)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "good: %+v\n", cfg)

	_, err = load(typoFile, typo)
	var pe *pos.Error
	var ue *expand.UnsetError
	if !errors.As(err, &pe) || !errors.As(err, &ue) {
		return fmt.Errorf("want a *pos.Error wrapping *expand.UnsetError, got %v", err)
	}
	fmt.Fprintf(w, "typo: at=%s name=%s strict=%t\n  %v\n", pe.Pos, ue.Name, errors.Is(err, expand.ErrStrict), err)
	return nil
}

// load parses src, expands it strictly and decodes it. With Read, the same
// is jsonc.Read(path, jsonc.WithExpand(), jsonc.WithStrictExpand(), ...).
//
//ds:def id=jsonc-strict-load-b8n4t6wd
func load(name, src string) (Config, error) {
	var cfg Config
	d, err := jsonc.Parse([]byte(src),
		jsonc.WithFileName(name),
		jsonc.WithStrictExpand(),
		jsonc.WithLookup(func(k string) (string, bool) { v, ok := env[k]; return v, ok }),
	)
	if err != nil {
		return cfg, err
	}
	x, err := d.Expanded()
	if err != nil {
		return cfg, err
	}
	return cfg, x.Decode(&cfg)
}
