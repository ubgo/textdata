package main

import (
	"strings"
	"testing"
)

// want is the program's output, pasted into the guide's "Try it" section;
// a change here means the guide needs the same change.
const want = `
good: {Default:claude Token:fake-token Region:eu-west-1}
typo: at=3:12 name=API_TOKN strict=true
  typo.jsonc:3:12: expand "token": expand: variable API_TOKN is not set (offset 0)
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
