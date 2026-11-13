package main

import (
	"strings"
	"testing"
)

// want is the program's output, pasted into the guide's "Try it" section;
// a change here means the guide needs the same change.
const want = `
new file:     new.env -rw------- "TOKEN=abc\n"
kept mode:    shared.env -rw-r--r-- "A=2\n"
resolve: real.env, link still a link: true
via link:     real.env -rw------- "VIA=link\n"
write error: atomicfile: rename temp file into place: disk full (is disk full: true)
temp file left behind: false
after failure: safe.env -rw------- "OLD=1\n"
`

func TestRun(t *testing.T) {
	var b strings.Builder
	if err := run(&b, t.TempDir()); err != nil {
		t.Fatal(err)
	}
	if got := b.String(); got != strings.TrimPrefix(want, "\n") {
		t.Errorf("output changed:\n%s", got)
	}
}
