package main

import (
	"strings"
	"testing"
)

// want is the program's output, pasted into the guide's "Try it" section;
// a change here means the guide needs the same change.
const want = `
--- .env
# Database settings for local development
# DB_HOST=localhost
DB_PORT=6543
DB_NAME=app

export LOG_LEVEL=info # quoted, with an inline comment
--- config.jsonc
{
  // Where the API listens.
  "server": {
    "host": "0.0.0.0",
    "port": 9090,
    "tls": true,
  },
  /* Features switched on in this environment. */
  "features": ["search", "billing", "audit"],
}
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
