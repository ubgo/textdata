package main

import (
	"bytes"
	"sync/atomic"
	"testing"
)

// wantOutput is the program's exact output; the README and the guide paste it.
const wantOutput = `DB_PASSWORD:  pw-v2
OLD_PASSWORD: pw-v1
LEGACY_TOKEN: kv1-token
API_TOKEN:    kv1-token
USER:         app
jsonc:        pw-v2
locked:       denied=true
no #field:    invalid=true
requests:     4
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

// TestConnectFromEnv runs the FromEnv constructor against the fake server
// through the variables the vault CLI uses.
func TestConnectFromEnv(t *testing.T) {
	var requests atomic.Int32
	srv := newFakeVault(&requests)
	defer srv.Close()
	t.Setenv("VAULT_ADDR", srv.URL)
	t.Setenv("VAULT_TOKEN", demoToken)
	p, err := connectFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	v, ok, err := p.Lookup("DB_USER")
	if err != nil || !ok || v != "app" {
		t.Fatalf("Lookup = %q, %v, %v", v, ok, err)
	}
}
