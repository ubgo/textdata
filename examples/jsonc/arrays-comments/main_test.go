package main

import (
	"bytes"
	"testing"
)

// want is the program's exact output; the guide's "Try it" section shows it.
const want = `--- append and insert ---
{
  // Hosts tried in order
  "hosts": [
    "local",
    "a.internal", // primary
    "b.internal",
    "c.internal",
  ],
  "retries": 3,
}
--- remove ---
{
  // Hosts tried in order
  "hosts": [
    "local",
    "a.internal", // primary
    "c.internal",
  ],
  "retries": 3,
}
--- raw value ---
{
  // Hosts tried in order
  "hosts": [
    "local",
    "a.internal", // primary
    "c.internal",
    {"host": "d.internal" /* canary */},
  ],
  "retries": 3,
}
--- comments ---
{
  // Hosts tried in order, local first
  "hosts": [
    "local",
    "a.internal",
    "c.internal",
    {"host": "d.internal" /* canary */},
  ],
  // How often to retry
  // before giving up
  "retries": 3,
}
comment of retries: "How often to retry\nbefore giving up"
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
