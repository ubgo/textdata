package main

import (
	"strings"
	"testing"
)

// want is the program's output, pasted into the guide's "Try it" section;
// a change here means the guide needs the same change.
const want = `
${gopass:work/api}                 => fake-gopass-token
${file:api.key}                    => fake-file-key
${vault:secret/data/app#password}  => fake-vault-password
${aws:ssm//app/prod/db_url}        => postgres://db.internal/app
Bearer ${gopass:work/api}          => Bearer fake-gopass-token
[${nobody:thing}]                  => []
missing: plugin=gopass not found=true
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
