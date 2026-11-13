package main

import (
	"bytes"
	"testing"
)

// want is the program's exact output; the guide's "Try it" section shows the
// same text.
const want = `== Save
"# app\nPORT=9090\n", files in dir: 1
== no path
dotenv: no path set; use Open, or set Path before Save
saved with mode -rw-------
== modes
.env.locked  -r--------
.env.copy    -r--------
.env.shared  -rw-r--r--
== line endings
CRLF:       "A=1\r\nB=3\r\nC=4\r\n"
mixed:      "A=x\r\nB=2\r\n"
no newline: "A=1\nB=2"
scratch:    "A=1\n"
== SaveAs
.env.prod: "DOMAIN=acme.io\n"
.env.qa: "DOMAIN=qa.acme.io\n"
Path still: .env.staging
empty path: dotenv: SaveAs needs a path
== symlink
still a link: true, target now "A=2\n"
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
