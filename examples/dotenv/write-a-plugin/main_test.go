package main

import (
	"bytes"
	"testing"
)

// want is the program's exact output; the guide's "Try it" section shows the
// same text.
const want = `== install and read
plugin vault: Lookuper, SaveGuard
plugin require-keys: SaveGuard
GetExpanded: postgres://app@db/app?password=hush
Get:         postgres://${DB_USER}@db/app?password=${DB_PASSWORD}
== guard
leak refused: true
dotenv: plugin "vault": guard save .env: plain-text secret in file: DB_PASSWORD holds DB_PASSWORD
dotenv: plugin "require-keys": guard save .env: dotenv: DB_URL is required
file unchanged: true files in dir: 1
== store down
dotenv: plugin "vault": lookup DB_PASSWORD: plugin unavailable: connection refused
unavailable: true
`

func TestRun(t *testing.T) {
	var b bytes.Buffer
	if err := run(&b, t.TempDir()); err != nil {
		t.Fatal(err)
	}
	if got := b.String(); got != want {
		t.Errorf("output mismatch\ngot:\n%s\nwant:\n%s", got, want)
	}
}
