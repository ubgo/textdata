package aws

import (
	"strings"
	"testing"
)

// FuzzParseRef: ParseRef never panics, an accepted reference renders back to
// a canonical form that parses to the same Ref, and an accepted id never
// holds a delimiter or whitespace.
func FuzzParseRef(f *testing.F) {
	for _, s := range []string{
		"sm/prod/db#password", "sm/prod/db?stage=AWSPREVIOUS#k", "sm/x?version=abc", "ssm//app/prod/db_url",
		"ssm/plain:3", "sm/arn:aws:secretsmanager:us-east-1:1:secret:x", "a#b#c", "",
		"sm/", "ssm/", "sm/x?", "sm/x?stage=", "sm/x?foo=1", "s3/x", "sm/a b", "sm/x#", "ssm/x?stage=y",
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		r, err := ParseRef(s)
		if err != nil {
			if r != (Ref{}) {
				t.Fatalf("error with non-zero Ref: %+v", r)
			}
			return
		}
		if r.ID == "" || strings.ContainsAny(r.ID, "?# \t\n") {
			t.Fatalf("ParseRef(%q) accepted id %q", s, r.ID)
		}
		if r.Service != SecretsManager && r.Service != ParameterStore {
			t.Fatalf("ParseRef(%q) accepted service %q", s, r.Service)
		}
		if r.VersionStage != "" && r.VersionID != "" {
			t.Fatalf("ParseRef(%q) set both stage and version", s)
		}
		again, err := ParseRef(r.String())
		if err != nil {
			t.Fatalf("ParseRef(%q).String() = %q does not parse: %v", s, r.String(), err)
		}
		if again != r {
			t.Fatalf("round trip %q: %+v != %+v", s, again, r)
		}
	})
}
