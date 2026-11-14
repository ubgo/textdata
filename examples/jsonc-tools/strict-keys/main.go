// Command strict-keys validates a config shaped like a multi-agent tool's
// (maps of structs, a string ID type with MarshalText used as a map key, an
// enum field, pointers, omitempty) against the schema built from its struct.
// By default a typo'd key is a warning; with schema.StrictKeys it is an error
// at the key's line:col, so a typed loader never silently drops a field.
package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/ubgo/textdata/jsonc"
	"github.com/ubgo/textdata/jsonc/schema"
)

// AgentID is a string-kind ID type with text marshalling, used as a value and
// as a map key; the schema reads it as a string.
type AgentID string

// MarshalText implements encoding.TextMarshaler.
func (a AgentID) MarshalText() ([]byte, error) { return []byte(a), nil }

// UnmarshalText implements encoding.TextUnmarshaler.
func (a *AgentID) UnmarshalText(b []byte) error { *a = AgentID(strings.ToLower(string(b))); return nil }

// Profile, Binding and Config are the typed shape; the tags drive the schema.
//
//ds:def id=jt-strictkeys-config-w3f7h2pe
type (
	Profile struct {
		Billing string   `json:"billing" enum:"plan,api"`
		Args    []string `json:"args,omitempty"`
		Timeout *int     `json:"timeout,omitempty"`
	}
	Binding struct {
		Agent   AgentID `json:"agent"`
		Profile string  `json:"profile"`
	}
	Config struct {
		Default  AgentID             `json:"default"`
		Profiles map[string]Profile  `json:"profiles"`
		Bindings map[AgentID]Binding `json:"bindings,omitempty"`
	}
)

// config has a bad enum value and a typo'd key ("biling") inside a map value.
const config = `{
  "default": "claude",
  "profiles": {
    "work": {"billing": "free", "args": ["--fast"]},
    "home": {"biling": "plan"}
  },
  "bindings": {"claude": {"agent": "claude", "profile": "work"}}
}
`

func main() {
	if err := run(os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// run validates the file twice: default, then with StrictKeys.
//
//ds:def id=jt-strictkeys-run-q9d4m6vt
func run(w io.Writer) error {
	s, err := schema.FromStruct(Config{})
	if err != nil {
		return err
	}
	doc, err := jsonc.Parse([]byte(config), jsonc.WithFileName("agents.jsonc"))
	if err != nil {
		return err
	}
	for _, mode := range []struct {
		name string
		opts []schema.Option
	}{
		{"default", nil},
		{"strict keys", []schema.Option{schema.StrictKeys()}},
	} {
		res := schema.Validate(doc, s, mode.opts...)
		fmt.Fprintf(w, "%s: ok=%t\n", mode.name, res.OK())
		for _, e := range res.Errors {
			fmt.Fprintf(w, "  error   %v\n", e)
		}
		for _, e := range res.Warnings {
			fmt.Fprintf(w, "  warning %v\n", e)
		}
	}
	return nil
}
