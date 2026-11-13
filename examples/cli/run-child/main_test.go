package main

import (
	"runtime"
	"strings"
	"testing"
)

// want is the program's output, pasted into the guide's "Try it" section;
// a change here means the guide needs the same change.
const want = `
child: from the file, postgres://db.internal/app
exit: 0
parent still has: from the shell
exit: 3
dotenvctl: envkit: expand: dotenv: required variable MISSING is not set or is empty: set MISSING first
exit: 1
`

func TestRun(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the child commands use /bin/sh")
	}
	var b strings.Builder
	if err := run(&b, t.TempDir()); err != nil {
		t.Fatal(err)
	}
	if got := b.String(); got != strings.TrimPrefix(want, "\n") {
		t.Errorf("output changed:\n%s", got)
	}
}
