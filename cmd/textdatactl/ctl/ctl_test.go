package ctl

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ubgo/textdata/dotenv/cli/dotenvcmd"
)

func run(args ...string) (code int, stdout, stderr string) {
	var out, errb bytes.Buffer
	code = Execute(args, &out, &errb)
	return code, out.String(), errb.String()
}

func envFile(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// TestDotenvMountMatchesStandalone: every verb behaves exactly as dotenvctl,
// apart from the program name in its own messages.
func TestDotenvMountMatchesStandalone(t *testing.T) {
	t.Parallel()
	const content = "# app\nDOMAIN=acme.dev\nAPI_URL=https://api.${DOMAIN}\n"
	cases := [][]string{
		{"get", "API_URL"},
		{"get", "API_URL", "--expand"},
		{"list", "--json"},
		{"keys"},
		{"get", "NOPE"},
		{"get", "NOPE", "--json"},
		{"set", "X=1", "--dry-run"},
		{"--bogus"},
	}
	for _, args := range cases {
		p := envFile(t, content)
		full := append([]string{"dotenv", "-f", p}, args...)
		code, out, errOut := run(full...)

		var sOut, sErr bytes.Buffer
		sCode := dotenvcmd.Execute(append([]string{"-f", p}, args...), &sOut, &sErr)

		if code != sCode {
			t.Errorf("%v: exit %d, standalone %d", args, code, sCode)
		}
		if out != sOut.String() {
			t.Errorf("%v: stdout %q, standalone %q", args, out, sOut.String())
		}
		if want := strings.ReplaceAll(sErr.String(), "dotenvctl", "textdatactl dotenv"); errOut != want {
			t.Errorf("%v: stderr %q, want %q", args, errOut, want)
		}
	}
}

func TestDotenvEditsThroughMount(t *testing.T) {
	t.Parallel()
	p := envFile(t, "# keep me\nA=1\n\nB=2\n")
	if code, _, errOut := run("dotenv", "-f", p, "set", "A=9"); code != ExitOK {
		t.Fatalf("set exit %d: %s", code, errOut)
	}
	b, _ := os.ReadFile(p)
	if string(b) != "# keep me\nA=9\n\nB=2\n" {
		t.Errorf("file = %q", b)
	}
	code, out, _ := run("dotenv", "-f", p, "get", "A", "--json")
	var env struct {
		OK   bool            `json:"ok"`
		Data json.RawMessage `json:"data"`
	}
	if code != ExitOK || json.Unmarshal([]byte(out), &env) != nil || !env.OK {
		t.Errorf("get --json = %d %q", code, out)
	}
}

func TestRootShell(t *testing.T) {
	t.Parallel()
	code, out, _ := run("--help")
	if code != ExitOK || !strings.Contains(out, "dotenv") || !strings.Contains(out, "Comment-preserving .env") {
		t.Errorf("--help = %d %q", code, out)
	}
	if code, out, _ := run(); code != ExitOK || !strings.Contains(out, "Available Commands") {
		t.Errorf("no args = %d %q", code, out)
	}
	code, _, errOut := run("yaml", "get")
	if code != ExitUsage || !strings.Contains(errOut, `unknown format "yaml"`) || !strings.Contains(errOut, "Run 'textdatactl --help'") {
		t.Errorf("unknown format = %d %q", code, errOut)
	}
	if code, _, errOut := run("--nope"); code != ExitUsage || !strings.Contains(errOut, "unknown flag") {
		t.Errorf("unknown flag = %d %q", code, errOut)
	}
	if code, out, _ := run("--version"); code != ExitOK || !strings.Contains(out, "textdatactl version") {
		t.Errorf("--version = %d %q", code, out)
	}
}

// TestFormatCommandDelegates drives the cobra tree directly, the path an
// embedding host takes, and checks the format command delegates the same way.
func TestFormatCommandDelegates(t *testing.T) {
	t.Parallel()
	var got []string
	formats := []Format{{Name: "fake", Short: "f", Execute: func(args []string, stdout, _ io.Writer, program string) int {
		got = append(append(got, program), args...)
		if len(args) > 0 && args[0] == "fail" {
			return ExitFailure
		}
		return ExitOK
	}}}
	root := newRoot(formats)
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"fake", "x", "--flag"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if strings.Join(got, " ") != "textdatactl fake x --flag" {
		t.Errorf("delegated %q", got)
	}
	root.SetArgs([]string{"fake", "fail"})
	if err := root.Execute(); err == nil || !strings.Contains(err.Error(), "exited with code 1") {
		t.Errorf("failure = %v", err)
	}
}
