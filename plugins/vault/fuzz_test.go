package vault

import (
	"strings"
	"testing"
)

// FuzzParseRef: ParseRef never panics, an accepted reference renders back to
// a canonical form that parses to the same Ref, and an accepted path never
// holds an empty, "." or ".." segment.
func FuzzParseRef(f *testing.F) {
	for _, s := range []string{
		"secret/data/app#password", "/kv/app", "secret/data/app?version=3#f", "a#b#c", "a#?x",
		"", "#", "a#", "a?version=0", "a?version=01", "a?x=1", "a//b", "a/../b", "./a", "a/",
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
		for _, seg := range strings.Split(r.Path, "/") {
			if seg == "" || seg == "." || seg == ".." {
				t.Fatalf("ParseRef(%q) accepted path %q", s, r.Path)
			}
		}
		if strings.ContainsAny(r.Path, "?#") {
			t.Fatalf("ParseRef(%q): path %q holds a delimiter", s, r.Path)
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
