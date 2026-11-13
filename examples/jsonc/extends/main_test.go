package main

import (
	"bytes"
	"testing"
)

// want is the program's exact output; the guide's "Try it" section shows it.
const want = `references: [../defaults/base.jsonc]
merged view:
{"log": {"level": "debug", "format": "json"}, "server": {"host":"0.0.0.0","port":9090}}
merged view has $extends: false | original still has it: true
no includer is ErrNoIncluder: true
cycle: loop/a.jsonc:1:14: $extends "b.jsonc": loop/b.jsonc:1:14: $extends "a.jsonc" closes a cycle: loop/a.jsonc -> loop/b.jsonc -> loop/a.jsonc: jsonc: $extends cycle
is ErrExtendsCycle: true
Read: map[host:0.0.0.0 port:9090] map[format:json level:debug]
`

func TestRun(t *testing.T) {
	var buf bytes.Buffer
	if err := run(&buf, t.TempDir()); err != nil {
		t.Fatal(err)
	}
	if got := buf.String(); got != want {
		t.Errorf("output changed:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}
