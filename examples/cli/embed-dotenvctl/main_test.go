package main

import (
	"strings"
	"testing"
)

// want is the program's output, pasted into the guide's "Try it" section;
// a change here means the guide needs the same change.
const want = `
http://localhost:8080
exit: 0
dotenvctl: key "PORT" not found in missing.env
exit: 1
myapp env: unknown command "nosuchverb" for "myapp env"
Run 'myapp env --help' for usage.
exit: 2
{"ok":true,"data":{"results":[{"key":"PORT","created":true}],"changed":true,"dry_run":false}}
exit: 0
myapp env: key "PORT" not found in missing.env
exit: 1
`

func TestRun(t *testing.T) {
	// missing.env is resolved against the working directory; run from an
	// empty one so a stray file cannot change the answer.
	t.Chdir(t.TempDir())
	var b strings.Builder
	if err := run(&b, t.TempDir()); err != nil {
		t.Fatal(err)
	}
	if got := b.String(); got != strings.TrimPrefix(want, "\n") {
		t.Errorf("output changed:\n%s", got)
	}
}
