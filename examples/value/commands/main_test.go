package main

import (
	"strings"
	"testing"
)

// want is the program's output, pasted into the guide's "Try it" section;
// a change here means the guide needs the same change.
const want = `
off:      token=$(echo never-runs)
on:       hello ada!
failed:   code=2
          expand: command "echo 'no such entry' >&2; exit 2" failed: shell: command "echo 'no such entry' >&2; exit 2" exited with code 2: no such entry
deadline: ErrTimeout=true
runner:   ErrTimeout=true
allowed:  fake-api-token
refused:  expand: command "rm -rf ~" failed: command not allowed
`

func TestRun(t *testing.T) {
	var b strings.Builder
	if err := run(&b); err != nil {
		t.Fatal(err)
	}
	if got := b.String(); got != strings.TrimPrefix(want, "\n") {
		t.Errorf("output changed:\n%s", got)
	}
}
