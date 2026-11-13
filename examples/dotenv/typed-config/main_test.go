package main

import (
	"bytes"
	"testing"
)

// want is the program's exact output; the guide's "Try it" section shows the
// same text.
const want = `== .env
{Port:8443 Debug:true Hosts:[api.acme.io www.acme.io] Wait:30s Token:t-123}
== .env.bad
PORT: "eighty" is not an integer
WAIT: "soon" is not a duration
TOKEN is required
== adapter
HOSTS: api.acme.io;www.acme.io true
AUTH: "" false
ExpandedMap: dotenv: required variable MISSING is not set or is empty: set MISSING
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
