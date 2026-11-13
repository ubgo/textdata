package main

import (
	"strings"
	"testing"
)

// want is the program's output, pasted into the guide's "Try it" section;
// a change here means the guide needs the same change.
const want = `
.env.stag    env=stag    contract=false keys=3 placeholders=1
.env.prod    env=prod    contract=false keys=5 placeholders=0
.env.example env=example contract=true  keys=3 placeholders=0
select TOKEN=t-1
select REGION=eu
select DB_URL=postgres://db.prod/app
named key missing: envkit: key "NOPE" not found
preview: [+SENTRY_ENV=staging] written: false
restore: [-# SENTRY_ENV=staging +SENTRY_ENV=staging]
unset absent key is ErrKeyNotFound: true
diff + DEPLOY_REGION
diff + DEPLOY_TOKEN
diff - SENTRY_ENV
diff ~ API_KEY: __FILL_ME__ -> k-123
diff ~ DB_URL: postgres://db.stag/app -> postgres://db.prod/app
diff ~ HOST: db.stag -> db.prod
columns: [stag prod]
drift API_KEY       stag=placeholder prod=present
drift DEPLOY_REGION stag=missing prod=present
drift DEPLOY_TOKEN  stag=missing prod=present
drift SENTRY_DSN    stag=disabled prod=missing
drift SENTRY_ENV    stag=present prod=missing
contract gaps: true [SENTRY_DSN]
child env: [API_KEY=k-123 DB_URL=postgres://db.prod/app DEPLOY_REGION=eu DEPLOY_TOKEN=t-1 HOST=db.prod TERM=xterm]
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
