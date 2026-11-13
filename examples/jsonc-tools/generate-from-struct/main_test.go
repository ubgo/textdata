package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// expectedFile is the exact output the guide shows under "Try it".
var expectedFile = filepath.Join("testdata", "expected.txt")

func TestRun(t *testing.T) {
	var got bytes.Buffer
	if err := run(&got, t.TempDir()); err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(expectedFile)
	if err != nil {
		t.Fatal(err)
	}
	if got.String() != string(want) {
		t.Errorf("output differs\n--- got\n%s--- want\n%s", got.String(), want)
	}
}

// TestPlainJSONIsJSON pins the guide's claim that Comments(false) output is
// plain JSON.
func TestPlainJSONIsJSON(t *testing.T) {
	b, err := plainJSON()
	if err != nil {
		t.Fatal(err)
	}
	if !json.Valid(b) {
		t.Errorf("not JSON:\n%s", b)
	}
}
