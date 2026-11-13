package main

import (
	"bytes"
	"testing"
)

// want is the program's exact output; the guide's "Try it" section shows the
// same text.
const want = `== Get vs GetExpanded
Get=postgres://${HOST}:$PORT/app
GetExpanded=postgres://db:5432/app ok=true
MISSING: []
== operators
${SET:-def}    -> "value"
${SET-def}     -> "value"
${SET:+alt}    -> "alt"
${SET+alt}     -> "alt"
${EMPTY:-def}  -> "def"
${EMPTY-def}   -> ""
${EMPTY:+alt}  -> ""
${EMPTY+alt}   -> "alt"
${UNSET:-def}  -> "def"
${UNSET-def}   -> "def"
${UNSET:+alt}  -> ""
${UNSET+alt}   -> ""
== dollars
NAME="world"
COST="cost 5$"
TEMPLATE="${NAME}"
PRICE="$5 or 100$"
UNCLOSED="${NAME"
BACKSLASH="\\world"
== nesting, self-reference, cycles
A="1"
B="10"
C="100"
D="100"
LIST="one,two"
CYCLE_A="ab"
CYCLE_B="ba"
== reachable names
BARE=".url"
BRACED="jdbc"
DASHED="KEY"
LITERAL="${spring.url}"
== ExpandedMap and RequiredError
AUTH: Bearer t-123
nil map: true key=TOKEN message="set TOKEN" empty=true
dotenv: required variable TOKEN is not set or is empty: set TOKEN
== command substitution
off: REV = $(${GIT} rev-parse --short HEAD)
GIT="git"
REV="a1b2c3d"
WHO error: expand: command "whoami" failed: command "whoami" not allowed
LIT="$(whoami)"
runner then off: REV = $(${GIT} rev-parse --short HEAD)
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
