package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// goldenPath holds the exact output the README and the guide show.
var goldenPath = filepath.Join("testdata", "output.txt")

// TestRun runs the example and compares its whole output with the golden
// file, so the published output cannot drift from what the program prints.
func TestRun(t *testing.T) {
	t.Setenv("GH_TOKEN", "")
	t.Setenv("GITHUB_TOKEN", "") // the banner labels an ambient token
	var out bytes.Buffer
	if err := run(&out, t.TempDir()); err != nil {
		t.Fatalf("run: %v", err)
	}
	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := out.String(); got != string(want) {
		t.Errorf("output differs from %s\n--- got ---\n%s\n--- want ---\n%s", goldenPath, got, want)
	}
}
