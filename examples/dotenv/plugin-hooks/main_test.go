package main

import (
	"bytes"
	"testing"
)

// want is the program's exact output; the guide's "Try it" section shows the
// same text.
const want = `== function options
ZONE: eu-west-1-a PASS: hunter2 Get(PASS): encrypted:c2VjcmV0 err: <nil>
save: dotenv: plugin "save-guard": guard save never-written.env: read-only
== Resolver
PASS=pa55
OTHER=[]
LIT=${mem:db/password}
dotenv: plugin "mem-store": resolve mem:nope: not found: mem:nope
== observers
disabled: [DEBUG] never referenced: [PORT URL]
== ordering
A: t2(t1(second)) B: t2(t1(last-runner))
== firewall
reads as: SECRET render unchanged: true saved unchanged: true
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
