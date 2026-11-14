package main

import (
	"strings"
	"testing"
)

// want is the program's output, pasted into the guide's "Try it" section;
// a change here means the guide needs the same change.
const want = `
with env:    postgres://ada@db.internal/app
without env: postgres://@/app
${DEMO_PORT:-5432}                   => 5432
${DEMO_REGION:-${DEMO_HOST}}         => db.internal
${DEMO_USER:+user is set}            => user is set
cost: $$5                            => cost: $5
${DEMO_USER:?DEMO_USER must be set}  => ada
layered:     postgres://ada@localhost/app
required:    key=DEMO_API_TOKEN
             expand: required variable DEMO_API_TOKEN is not set or is empty: set DEMO_API_TOKEN first
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
