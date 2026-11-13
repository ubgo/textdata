package main

import (
	"bytes"
	"errors"
	"testing"

	"github.com/ubgo/textdata/plugin"
)

// wantOutput is the program's exact output; the README and the guide paste it.
const wantOutput = `installed: primary: Lookuper, Resolver
installed: fallback: Lookuper, Resolver
REGION:      "eu-west-1"
ZONE:        "eu-west-1-a"
DB_HOST:     "db.primary"
CACHE_HOST:  "cache.fallback"
DB_PASSWORD: "pw-primary"
TYPO:        ""
BROKEN:      error from plugin "primary", unavailable=true
jsonc:       "pw-primary"
`

func TestRun(t *testing.T) {
	var buf bytes.Buffer
	if err := run(&buf); err != nil {
		t.Fatal(err)
	}
	if got := buf.String(); got != wantOutput {
		t.Errorf("output:\n%s\nwant:\n%s", got, wantOutput)
	}
}

// TestStoreContract pins the contract a value plugin keeps: a Resolver says
// ErrNotFound and ErrUnsupportedScheme, a Lookuper says ok=false for a miss.
func TestStoreContract(t *testing.T) {
	s := &Store{name: "t", values: map[string]string{}, lookups: map[string]string{"GONE": "gone"}}
	if _, err := s.Resolve(memScheme, "gone"); !errors.Is(err, plugin.ErrNotFound) {
		t.Errorf("Resolve miss = %v", err)
	}
	if _, err := s.Resolve("other", "x"); !errors.Is(err, plugin.ErrUnsupportedScheme) {
		t.Errorf("Resolve other scheme = %v", err)
	}
	if _, ok, err := s.Lookup("GONE"); ok || err != nil {
		t.Errorf("Lookup miss = %v, %v", ok, err)
	}
}
