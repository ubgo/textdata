package main

import (
	"bytes"
	"testing"
)

// wantOutput is the program's exact output; the README and the guide paste it.
const wantOutput = `extends:     [team.jsonc]
merged:      port=8080 db.host=db.internal db.pool=10
app intact:  true
escape:      denied=true
loop:        cycle=true
no includer: true
disk read:   port=8080 db=map[host:db.internal pool:10]
disk escape: denied=true
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
