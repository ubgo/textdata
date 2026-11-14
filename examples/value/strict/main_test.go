package main

import (
	"strings"
	"testing"
)

// want is the program's output, pasted into the guide's "Try it" section;
// a change here means the guide needs the same change.
const want = `
Bearer ${API_TOKEN}      => Bearer fake-token
[${EMPTY}]               => []
${REGION:-eu-west-1}     => eu-west-1
Bearer ${API_TOKN}           strict=true  unset API_TOKN
  expand: variable API_TOKN is not set (offset 7)
${gopas:work/api}            strict=true  unknown scheme gopas
  expand: no resolver for scheme "gopas" in ${gopas:work/api} (offset 0)
$(gopass show -o work/api)   strict=true  command off
  expand: command substitution is off: $(gopass show -o work/api) (offset 0)
${API_SECRET:?set it}        strict=false required API_SECRET
  expand: required variable API_SECRET is not set or is empty: set it
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
