package main

import (
	"bytes"
	"testing"
)

// wantOutput is the program's exact output; the README and the guide paste it.
const wantOutput = `dotenv raw:      ${gopass:personal/app/db}
dotenv expanded: hush-from-gopass (user app)
missing entry:   not found=true
jsonc expanded:  hush-from-gopass
jsonc unchanged: true
gopass calls:    3
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
