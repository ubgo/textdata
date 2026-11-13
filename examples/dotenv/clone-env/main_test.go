package main

import (
	"bytes"
	"testing"
)

// want is the program's exact output; the guide's "Try it" section shows the
// same text.
const want = `== .env.staging (API_URL -> https://api.staging.acme.io)
# shared
DOMAIN=staging.acme.io
LOG_LEVEL=debug

# derived: keep these as references
API_URL=https://api.${DOMAIN}
CDN_URL=https://cdn.${DOMAIN}
== .env.prod (API_URL -> https://api.acme.io)
# shared
DOMAIN=acme.io
LOG_LEVEL=warn

# derived: keep these as references
API_URL=https://api.${DOMAIN}
CDN_URL=https://cdn.${DOMAIN}
== .env.qa (API_URL -> https://api.qa.acme.io)
# shared
DOMAIN=qa.acme.io
LOG_LEVEL=debug

# derived: keep these as references
API_URL=https://api.${DOMAIN}
CDN_URL=https://cdn.${DOMAIN}
== .env.baked (API_URL -> https://api.staging.acme.io)
API_URL=https://api.staging.acme.io
CDN_URL=https://cdn.staging.acme.io
DOMAIN=acme.io
LOG_LEVEL=debug
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
