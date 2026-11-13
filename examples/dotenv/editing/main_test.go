package main

import (
	"bytes"
	"testing"
)

// want is the program's exact output; the guide's "Try it" section shows the
// same text.
const want = `== Set
PORT created: false
LOG_LEVEL created: true
same value is a no-op: true
# app
export PORT = 9090 # dev only
HOST=localhost
DB_HOST=db.internal
#DEBUG=true
LOG_LEVEL=info
GREETING="hello world"
PASSWORD='p$$w'
MOTD="line one
line two"
== SetAfter and SetBefore
anchor missing: dotenv: anchor key not found: "NOPE"
# app
APP_NAME=shop
export PORT = 8080 # dev only
HOST=localhost
DB_HOST=db.internal
DB_PORT=5432
#DEBUG=true
CACHE_URL=redis://cache
== Unset and Restore
Unset HOST: true
Unset HOST again: false
Unset DB_HOST, delete: true
Disabled: [{HOST localhost} {DEBUG true}]
# app
export PORT = 8080 # dev only
# HOST=localhost
#DEBUG=true
Restore HOST: true
Restore DEBUG: true
# app
export PORT = 8080 # dev only
HOST=localhost
DEBUG=true
== Inherited
HOME: "/home/app" found=true
AWS_REGION: "" found=false
GetExpanded(HOME) = ""
== generate
# Copy to .env and fill in.
# Do not commit .env.

HOST=localhost
# derived
URL=https://${HOST}/v1 # derived
GREETING="hello world"
CRLF file: "A=1\r\nB=2\r\n"
== Append vs Set
Set:    "PORT=9090\n" Count=1
Append: "PORT=8080\nPORT=9090\n" Count=2
`

func TestRun(t *testing.T) {
	var b bytes.Buffer
	if err := run(&b); err != nil {
		t.Fatal(err)
	}
	if got := b.String(); got != want {
		t.Errorf("output mismatch\ngot:\n%s\nwant:\n%s", got, want)
	}
}
