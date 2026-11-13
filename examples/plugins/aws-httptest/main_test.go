package main

import (
	"bytes"
	"testing"
)

// wantOutput is the program's exact output; the README and the guide paste it.
const wantOutput = `DB_PASSWORD: pw-current
DB_PORT:     5432
PREVIOUS:    pw-previous
API_KEY:     plain-api-key
DB_URL:      postgres://db.internal/app
FEATURE:     on
jsonc:       postgres://db.internal/app
missing:     not found=true
requests:    secretsmanager=4 ssm=2 (credentials asked 6 times)
shared file: ok=true
no creds:    true
`

func TestRun(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir()) // keep the program's temp folder inside the test's
	var buf bytes.Buffer
	if err := run(&buf, t.TempDir()); err != nil {
		t.Fatal(err)
	}
	if got := buf.String(); got != wantOutput {
		t.Errorf("output:\n%s\nwant:\n%s", got, wantOutput)
	}
}
