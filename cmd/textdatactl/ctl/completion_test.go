package ctl

import (
	"strings"
	"testing"
)

// TestCompletionReachesFormatVerbs: textdatactl serves a completion script
// for itself, and the script's __complete calls reach each format's verbs.
func TestCompletionReachesFormatVerbs(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{{"completion", "bash"}, {"dotenv", "completion", "bash"}} {
		code, out, errOut := run(args...)
		if code != ExitOK || !strings.Contains(out, "# bash completion V2 for textdatactl ") {
			t.Errorf("%v: exit %d, stderr %q, want a textdatactl bash script", args, code, errOut)
		}
	}
	cases := []struct {
		args []string
		want string
	}{
		{[]string{"__complete", ""}, "dotenv"},
		{[]string{"__complete", "dotenv", "ge"}, "get"},
		{[]string{"__complete", "jsonc", "to"}, "to-json"},
		{[]string{"__completeNoDesc", "dotenv", "mat"}, "matrix"},
	}
	for _, tc := range cases {
		code, out, errOut := run(tc.args...)
		if code != ExitOK || !strings.Contains(out, tc.want) {
			t.Errorf("%v: exit %d, out %q, stderr %q; want %q offered", tc.args, code, out, errOut, tc.want)
		}
	}
}
