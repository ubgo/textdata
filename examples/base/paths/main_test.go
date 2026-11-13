package main

import (
	"strings"
	"testing"
)

// want is the program's output, pasted into the guide's "Try it" section;
// a change here means the guide needs the same change.
const want = `
dotted:  servers."eu.west"[0].port  pointer form /servers/eu.west/0/port
pointer: servers."eu.west".0.port  segment 2 index=true key=true
ParseAny("/a~1b/c") = 2 segments, String "a/b.c"
ParseAny("a.b") = 2 segments, String "a.b"
ParseAny("") = 0 segments, String ""
bad path: path: invalid syntax: "a[x]" at byte 1: index "x" is not a number (syntax=true)
built: plugins[2]."with.dot"  parent plugins[2]  last key "with.dot" ok=true  base still plugins
offset 14 -> 2:12 (line 2 col 12), lines=4, zero Pos prints -
empty list err is nil: true
list: config.jsonc:1:1: first problem (and 1 more error)
first: config.jsonc:1:1: first problem
jsonc: config.jsonc:3:3: missing comma before '"'
  at line 3 col 3: missing comma before '"'
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
