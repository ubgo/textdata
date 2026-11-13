package dotenvcmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestUsageErrorsExitTwo: a malformed invocation that a verb, not cobra,
// detects is still a usage error: exit 2 and code "usage" in the envelope,
// the same as an unknown flag.
func TestUsageErrorsExitTwo(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	env := filepath.Join(dir, ".env")
	if err := os.WriteFile(env, []byte("A=1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".env.prod"), []byte("A=2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		args []string
	}{
		{"set --after with --before", []string{"-f", env, "set", "B=1", "--after", "A", "--before", "A"}},
		{"set argument without =", []string{"-f", env, "set", "B"}},
		{"diff unknown --format", []string{"-f", env, "diff", env, filepath.Join(dir, ".env.prod"), "--format", "xml"}},
		{"matrix unknown --format", []string{"matrix", "--dir", dir, "--format", "xml"}},
		{"matrix bad --placeholder", []string{"matrix", "--dir", dir, "--placeholder", "("}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			code, _, stderr := runCLI(t, tc.args...)
			if code != ExitUsage {
				t.Errorf("exit = %d, want %d (stderr %q)", code, ExitUsage, stderr)
			}
			code, stdout, _ := runCLI(t, append([]string{"--json"}, tc.args...)...)
			if code != ExitUsage || !strings.Contains(stdout, `"code":"usage"`) {
				t.Errorf("--json: exit %d, envelope %q; want exit 2 and code usage", code, stdout)
			}
		})
	}
}

// TestUnknownFormatWritesNoReport: an unknown --format is rejected before -o
// creates or truncates the report file, and the failure reaches the terminal,
// not the file.
func TestUnknownFormatWritesNoReport(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	env := filepath.Join(dir, ".env")
	if err := os.WriteFile(env, []byte("A=1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	report := filepath.Join(dir, "report.html")
	if err := os.WriteFile(report, []byte("keep me"), 0o600); err != nil {
		t.Fatal(err)
	}
	code, _, stderr := runCLI(t, "-f", env, "diff", env, env, "--format", "xml", "-o", report)
	if code != ExitUsage || !strings.Contains(stderr, "--format must be") {
		t.Errorf("exit %d stderr %q; want exit 2 with the --format message on stderr", code, stderr)
	}
	if got, _ := os.ReadFile(report); string(got) != "keep me" {
		t.Errorf("report file = %q, want it untouched", got)
	}
}
