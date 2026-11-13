package main

import (
	"bytes"
	"testing"
)

// want is the program's exact output; the guide's "Try it" section shows it.
const want = `--- merged ---
{
  // HTTP listener
  "server": {
    "host": "0.0.0.0",
    "port": 9090,
  },
  "features": [
    "auth"
  ],
  // Local tracing collector
  "tracing": "http://localhost:4318",
}
base unchanged: true
second merge is a no-op: true
broken is ErrInvalid: true
array root is ErrType: true
target unchanged: true
`

func TestRun(t *testing.T) {
	var buf bytes.Buffer
	if err := run(&buf); err != nil {
		t.Fatal(err)
	}
	if got := buf.String(); got != want {
		t.Errorf("output changed:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}
