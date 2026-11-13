package ctl

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ubgo/voltkit/versioncmd"

	"github.com/ubgo/textdata/jsonc/cli/jsonccmd"
)

func jsoncFile(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "c.jsonc")
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// TestJSONCMountMatchesStandalone: every verb behaves exactly as jsonccmd
// run on its own, with the program name "textdatactl jsonc".
func TestJSONCMountMatchesStandalone(t *testing.T) {
	t.Parallel()
	const content = "{\n  // port\n  \"port\": 8080,\n  \"host\": \"x\"\n}\n"
	other := jsoncFile(t, `{"port":9090}`)
	cases := [][]string{
		{"get", "", "port"},
		{"get", "", "port", "--json"},
		{"get", "", "nope"},
		{"get", "", "nope", "--json"},
		{"set", "", "port", "1", "--json-value", "--dry-run"},
		{"to-json", ""},
		{"diff", "", other},
		{"diff", "", other, "--json"},
		{"--bogus"},
		{"--bogus", "--json"},
	}
	for _, args := range cases {
		p := jsoncFile(t, content)
		args = append([]string(nil), args...)
		if len(args) > 1 && args[1] == "" {
			args[1] = p
		}
		code, out, errOut := run(append([]string{"jsonc"}, args...)...)

		var sOut, sErr bytes.Buffer
		sCode := jsonccmd.Execute(args, &sOut, &sErr)
		if code != sCode || out != sOut.String() || errOut != sErr.String() {
			t.Errorf("%v: mount (%d %q %q) != standalone (%d %q %q)", args, code, out, errOut, sCode, sOut.String(), sErr.String())
		}
	}
	if code, _, errOut := run("jsonc", "get", "/nonexistent/x.jsonc", "a"); code != ExitFailure || !strings.HasPrefix(errOut, "textdatactl jsonc: ") {
		t.Errorf("program name: %d %q", code, errOut)
	}
}

func TestJSONCEditsThroughMount(t *testing.T) {
	t.Parallel()
	p := jsoncFile(t, "{\n  // keep me\n  \"a\": 1,\n}\n")
	if code, _, errOut := run("jsonc", "set", p, "a", "2", "--json-value"); code != ExitOK {
		t.Fatalf("set: %d %s", code, errOut)
	}
	if b, _ := os.ReadFile(p); string(b) != "{\n  // keep me\n  \"a\": 2,\n}\n" {
		t.Errorf("file = %q", b)
	}
	if code, out, _ := run("jsonc", "--help"); code != ExitOK || !strings.Contains(out, "textdatactl jsonc [command]") {
		t.Errorf("jsonc --help: %d %q", code, out)
	}
	if code, out, _ := run("--help"); code != ExitOK || !strings.Contains(out, "jsonc") || !strings.Contains(out, "JSON/JSONC") {
		t.Errorf("root --help lists jsonc: %d %q", code, out)
	}
}

func TestVersionCommand(t *testing.T) {
	t.Parallel()
	code, out, errOut := run("version")
	if code != ExitOK || !strings.HasPrefix(out, Program+" ") || !strings.Contains(out, "source:") || errOut != "" {
		t.Errorf("version: %d %q %q", code, out, errOut)
	}
	code, out, _ = run("version", "--json")
	var env struct {
		OK   bool `json:"ok"`
		Data struct {
			Binary  string `json:"binary"`
			Version string `json:"version"`
			Source  string `json:"source"`
		} `json:"data"`
	}
	if code != ExitOK || json.Unmarshal([]byte(out), &env) != nil || !env.OK || env.Data.Binary != Program || env.Data.Version == "" || env.Data.Source == "" {
		t.Errorf("version --json: %d %q", code, out)
	}
	// --version and the version command print one version from one source.
	if code, vout, _ := run("--version"); code != ExitOK || vout != Program+" version "+env.Data.Version+"\n" {
		t.Errorf("--version %q disagrees with version --json %q", vout, env.Data.Version)
	}
	if want := versioncmd.Collect(Program).Version; env.Data.Version != want {
		t.Errorf("version = %q, want Collect's %q", env.Data.Version, want)
	}
	if code, _, errOut := run("version", "extra"); code != ExitUsage || !strings.Contains(errOut, `unknown command "extra"`) {
		t.Errorf("version extra: %d %q", code, errOut)
	}
	if code, out, _ := run("version", "--help"); code != ExitOK || !strings.Contains(out, "textdatactl version --json") {
		t.Errorf("version --help: %d %q", code, out)
	}
}
