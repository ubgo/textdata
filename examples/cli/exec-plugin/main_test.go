package main

import (
	"os"
	"runtime"
	"strings"
	"testing"
)

// want is the program's output, pasted into the guide's "Try it" section;
// a change here means the guide needs the same change.
const want = `
plugin github: shadowed by built-in (dotenvctl-github)
plugin hello: active (dotenvctl-hello)
hello, args: [world]
file: .env
json: 0
placeholder: ^__[A-Z0-9_]+__$
version set: yes
callback: hi from the file
exit: 0
hello, args: []
file: .env
json: 1
placeholder: ^__[A-Z0-9_]+__$
version set: yes
callback: hi from the file
exit: 0
`

// TestMain lets the test binary answer the plugin's callback: dotenvctl
// hands plugins os.Executable(), which under go test is this binary.
func TestMain(m *testing.M) {
	if code, ok := serveAsHost(); ok {
		os.Exit(code)
	}
	os.Exit(m.Run())
}

func TestRun(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the plugin is a POSIX shell script")
	}
	var b strings.Builder
	if err := run(&b, t.TempDir()); err != nil {
		t.Fatal(err)
	}
	if got := b.String(); got != strings.TrimPrefix(want, "\n") {
		t.Errorf("output changed:\n%s", got)
	}
}
