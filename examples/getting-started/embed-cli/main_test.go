package main

import (
	"strings"
	"testing"
)

// want is the program's output, pasted into the guide's "Try it" section;
// a change here means the guide needs the same change.
const want = `
format dotenv Comment-preserving .env control (get, set, diff, matrix, run, github, vercel)
format jsonc  Comment-preserving JSON/JSONC control (get, set, unset, restore, delete, diff, to-json, fmt, validate)
9090
{"ok":true,"data":{"path":"server","kind":"object","value":{"host":"0.0.0.0","port":9090}}}
missing path: exit 1 (failure=true)
`

func TestRun(t *testing.T) {
	var b strings.Builder
	if err := run(&b, t.TempDir()); err != nil {
		t.Fatal(err)
	}
	if got := b.String(); got != strings.TrimPrefix(want, "\n") {
		t.Errorf("output changed:\n%s", got)
	}
}
