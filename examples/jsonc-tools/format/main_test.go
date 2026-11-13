package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// expectedFile is the exact output the guide shows under "Try it".
var expectedFile = filepath.Join("testdata", "expected.txt")

func TestRun(t *testing.T) {
	var got bytes.Buffer
	if err := run(&got); err != nil {
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
