package main

import (
	"bytes"
	"testing"
)

// wantOutput is the program's exact output; the README and the guide paste it.
const wantOutput = `dotenv TLS_KEY: pem-body
jsonc tls.key:  pem-body
../shared.txt:  denied=true
over 4 bytes:   too large=true
untrimmed:      "pem-body\n"
WithRoot(..):   shared-value
confined abs:   denied=true
Unconfined abs: shared-value
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
