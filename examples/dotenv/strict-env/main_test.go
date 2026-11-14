package main

import (
	"strings"
	"testing"
)

// want is the program's output, pasted into the guide's "Try it" section;
// a change here means the guide needs the same change.
const want = `
DSN      postgres://db.internal/app
REPLICA  .env:4:9: expand REPLICA: expand: variable DB_HOTS is not set (offset 11)
TOKEN    .env:5:7: expand TOKEN: expand: no resolver for scheme "vault" in ${vault:secret/app#token} (offset 0)
STAMP    .env:6:7: expand STAMP: expand: command substitution is off: $(date +%s) (offset 0)
`

func TestRun(t *testing.T) {
	var b strings.Builder
	if err := run(&b); err != nil {
		t.Fatal(err)
	}
	if got := b.String(); got != strings.TrimPrefix(want, "\n") {
		t.Errorf("output changed:\n%s", got)
	}
}
