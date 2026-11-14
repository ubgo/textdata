package main

import (
	"strings"
	"testing"
)

// want is the program's output, pasted into the guide's "Try it" section;
// a change here means the guide needs the same change.
const want = `
exit:     code=11 stderr="entry not found"
          shell: command "echo 'entry not found' >&2; exit 11" exited with code 11: entry not found
timeout:  ErrTimeout=true
          shell: command "sleep 30" timed out
deadline: ErrTimeout=true DeadlineExceeded=true
cancel:   Canceled=true ErrTimeout=false
no shell: ErrNoShell=true
          shell: command "echo hi": no shell found: no-such-shell: exec: "no-such-shell": executable file not found in $PATH
stdin:    stdin is empty
no leak:  shell: command "printf '%s' \"$FAKE_SECRET\"; exit 1" exited with code 1 (stdout in error: false)
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
