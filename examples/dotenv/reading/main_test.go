package main

import (
	"bytes"
	"testing"
)

// want is the program's exact output; the guide's "Try it" section shows the
// same text.
const want = `== Read
API_URL=https://api.example.com
DOMAIN=example.com
missing file is an error: true
== Open
https://api.${DOMAIN} true existed: true
missing file existed: false keys: 0
created: "PORT=8080\n"
== Parse and ParseReader
NAME: my app
round trip: true
keys: [PORT HOST]
== accessors
Get(DB_HOST) = "db.internal" true, Has = true, Count = 2
Get(DB_USER) = "" false, Has = false, Count = 0
Get(HOME) = "" false, Has = false, Count = 0
Keys: [DB_HOST DB_PORT]
Pairs: DB_HOST=localhost
Pairs: DB_PORT=5432
Pairs: DB_HOST=db.internal
Map: 2 keys, DB_HOST = db.internal
duplicate: DB_HOST is set 2 times
== Existed, Mode, Path
existing: true -rw-r----- .env.shared
missing:  false -rw------- .env.missing
parsed:   false -rw------- ""
`

func TestRun(t *testing.T) {
	var b bytes.Buffer
	if err := run(&b, t.TempDir()); err != nil {
		t.Fatal(err)
	}
	if got := b.String(); got != want {
		t.Errorf("output mismatch\ngot:\n%s\nwant:\n%s", got, want)
	}
}
