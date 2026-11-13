package main

import (
	"strings"
	"testing"
)

// want is the program's output, pasted into the guide's "Try it" section;
// a change here means the guide needs the same change.
const want = `
store:           [Lookuper Resolver]
no-private-keys: [SaveGuard]
upper:           []
info: store: Lookuper, Resolver
info: no-private-keys: SaveGuard
info: upper: (none)
save guards: 1, first resolver: store, second resolver: unknown
error: demo: plugin "store": lookup DOWN: dial store: plugin unavailable
unavailable=true plugin=true
dotenv: DSN=postgres://app@db/app?password=hush TOKEN=t-123
jsonc:  db.password=hush token=t-123
dotenv save: dotenv: plugin "no-private-keys": guard save $DIR/.env: dotenv .env: TLS_KEY holds a private key: access denied (denied=true)
.env on disk: "TLS_KEY=\n"
jsonc save:  jsonc: plugin "no-private-keys": guard save $DIR/config.jsonc: jsonc config.jsonc: tls.key holds a private key: access denied (denied=true)
plugins seen by jsonc: [no-private-keys: SaveGuard]
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
