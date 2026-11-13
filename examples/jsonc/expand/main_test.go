package main

import (
	"bytes"
	"testing"
)

// want is the program's exact output; the guide's "Try it" section shows it.
const want = `expanded: postgres://db.internal:5432/app
raw:      postgres://${HOST}:${PORT:-5432}/app
build (commands off): $(git rev-parse --short HEAD) | price: $5
required: app.jsonc:2:12: expand "token": expand: required variable API_TOKEN is not set or is empty: set API_TOKEN
missing key: API_TOKEN
password: "hush" | build: a1b2c3d
plugin: lookup: Lookuper
plugin: fake-vault: Resolver
plugin: trim-space: ValueTransformer
plugin: fake-git: CommandRunner
sh -c: hello
Read: postgres://db.internal:5432/app | ExpandRequested: true
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
