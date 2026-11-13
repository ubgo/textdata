package main

import (
	"bytes"
	"testing"
)

// want is the program's exact output; the guide's "Try it" section shows it.
const want = `--- after edits ---
{
  // Where to sync
  "sync": {
    "remote": "origin", // git remote name
    "branch": "main",
    // "interval": 60,
  },

  /* feature switches */
  "features": {
    "alpha": true,
  },
  "log": {
    "level": "info"
  },
}
restored interval: 60 | second Restore is ErrExists: true
saved app.jsonc, mode -rw-r-----
first run: existed=false, created new.jsonc mode -rw-------
{
  "port": 8080
}
link still a symlink: true | Path after SaveAs: link.jsonc
no path: jsonc: no path set; use Open, or SetPath before Save
after SetPath, saved from-bytes.jsonc
refused: jsonc: plugin "no-plaintext-secret": guard save guarded.jsonc: plaintext secret; write a ${scheme:ref} reference instead
is errPlaintextSecret: true | file written: false
`

func TestRun(t *testing.T) {
	var buf bytes.Buffer
	if err := run(&buf, t.TempDir()); err != nil {
		t.Fatal(err)
	}
	if got := buf.String(); got != want {
		t.Errorf("output changed:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}
