package main

import (
	"strings"
	"testing"
)

// want is the program's output, pasted into the guide's "Try it" section;
// a change here means the guide needs the same change.
const want = `
before: "# service\r\nhost=db.internal\r\n\r\nport=5432\r\n"
after:  "# service\r\nhost=db.internal\r\n\r\nport=6543\r\n"
replaced bytes [36,40)
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
