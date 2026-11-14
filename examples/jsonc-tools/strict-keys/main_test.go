package main

import (
	"strings"
	"testing"
)

// want is the program's output, pasted into the guide's "Try it" section;
// a change here means the guide needs the same change.
const want = `
default: ok=false
  error   agents.jsonc:4:25: profiles.work.billing: must be one of "plan", "api"; got "free"
  warning agents.jsonc:5:14: profiles.home.biling: unknown key "biling"
strict keys: ok=false
  error   agents.jsonc:4:25: profiles.work.billing: must be one of "plan", "api"; got "free"
  error   agents.jsonc:5:14: profiles.home.biling: unknown key "biling"
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
