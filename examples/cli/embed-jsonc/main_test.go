package main

import (
	"strings"
	"testing"
)

// want is the program's output, pasted into the guide's "Try it" section;
// a change here means the guide needs the same change.
const want = `
exit: 0
{
  // where the API listens
  "server": {"host": "localhost", "port": 9090}
}
9090
exit: 0
myapp jsonc: missing.jsonc: no such file
exit: 1
{"ok":true,"data":{"path":"server","kind":"object","value":{"host":"localhost","port":9090}}}
exit: 0
myapp jsonc: missing.jsonc: no such file
exit: 1
myapp: unknown command "nosuchverb" for "myapp jsonc"
exit: 2
formats: [dotenv jsonc]
localhost
exit: 0
textdatactl dotenv: key "PORT" not found in missing.env
exit: 1
`

func TestRun(t *testing.T) {
	// The missing.* names resolve against the working directory; run from an
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
