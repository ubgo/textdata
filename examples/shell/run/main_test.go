package main

import (
	"strings"
	"testing"
)

// want is the program's output, pasted into the guide's "Try it" section;
// a change here means the guide needs the same change.
const want = `
run:     "hello from the shell"
default: ["sh" "-c"]
runner:  "hi from work"
bash:    "bash it is"
printf 'secret\r\n'          => "secret"
printf '  padded  \n\n'      => "  padded  "
printf 'line 1\nline 2\n'    => "line 1\nline 2"
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
