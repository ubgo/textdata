package dotenvcmd_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/ubgo/textdata/dotenv/cli/dotenvcmd"
)

// TestProgramNameReachesExamplesAndCompletion: --help examples (built-in
// verbs and the provider namespaces) and the completion script follow
// WithProgramName; the standalone binary still says dotenvctl.
func TestProgramNameReachesExamplesAndCompletion(t *testing.T) {
	t.Parallel()
	run := func(name string, args ...string) string {
		var out, errb bytes.Buffer
		var opts []dotenvcmd.ExecuteOption
		if name != "" {
			opts = append(opts, dotenvcmd.WithProgramName(name))
		}
		if code := dotenvcmd.Execute(args, &out, &errb, opts...); code != dotenvcmd.ExitOK {
			t.Fatalf("%v: exit %d, stderr %q", args, code, errb.String())
		}
		return out.String()
	}

	for _, verb := range [][]string{{"get", "--help"}, {"matrix", "--help"}, {"github", "--help"}, {"vercel", "--help"}, {"plugins", "--help"}} {
		hosted := run("textdatactl dotenv", verb...)
		if !strings.Contains(hosted, "Examples:\n  textdatactl dotenv "+verb[0]) {
			t.Errorf("%v hosted: examples do not name textdatactl dotenv:\n%s", verb, hosted)
		}
		if strings.Contains(strings.ReplaceAll(hosted, "dotenvctl-", ""), "dotenvctl") {
			t.Errorf("%v hosted: help still names dotenvctl:\n%s", verb, hosted)
		}
		if standalone := run("", verb...); !strings.Contains(standalone, "  dotenvctl "+verb[0]) {
			t.Errorf("%v standalone: examples lost dotenvctl:\n%s", verb, standalone)
		}
	}

	if got := run("textdatactl dotenv", "completion", "bash"); !strings.Contains(got, "# bash completion V2 for textdatactl ") {
		t.Errorf("hosted completion is not a textdatactl script:\n%.200s", got)
	}
	if got := run("", "completion", "bash"); !strings.Contains(got, "# bash completion V2 for dotenvctl ") {
		t.Errorf("standalone completion is not a dotenvctl script:\n%.200s", got)
	}
}
