package main

import (
	"strings"
	"testing"
)

// want is the program's output, pasted into the guide's "Try it" section;
// a change here means the guide needs the same change.
const want = `
${URL}                             => postgres://db.internal:5432/app
${EMPTY-unset} ${EMPTY:-empty}     =>  empty
${HOST+set} [${EMPTY:+nonempty}]   => set []
${MISSING:-${HOST:-x}}             => db.internal
price: $$5 for $HOST               => price: $5 for db.internal
resolved: secret-for-db/password []
split: "vault" "db/password" true
parsed: name=PORT default-if-empty=true arg=5432
run off: a $(echo $WHO) b [$(fail)]
run on:  a ECHO WORLD b
run fail: expand: command "fail" failed: exit status 1 (cmd="fail")
A => "a-b-", SELF => "", default depth 32
error: expand: required variable TOKEN is not set or is empty: set TOKEN in .env.local
key=TOKEN empty=true
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
