package main

import (
	"strings"
	"testing"
)

// want is the program's output, pasted into the guide's "Try it" section;
// a change here means the guide needs the same change.
const want = `
dotenv.Read: DB_PORT=5432 LOG_LEVEL=debug DATABASE_URL=postgres://localhost:5432/app
dotenv.Open: keys=[DB_HOST DB_PORT DATABASE_URL LOG_LEVEL]
  Get=postgres://${DB_HOST}:${DB_PORT}/app
  GetExpanded=postgres://localhost:5432/app
  has API_SECRET=false
jsonc.Read: server=map[host:0.0.0.0 port:8080]
jsonc.Open: server.port=8080 features[0]=search decoded={Server:{Host:0.0.0.0 Port:8080} Features:[search billing]}
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
