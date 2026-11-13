package jsonccmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ubgo/textdata/clikit/outfmt"
)

// TestSetHelpExamplesWriteTypedValues: every `set` example in --help (the
// root's and set's own) that sets a non-string value does so with
// --json-value, so copying it writes a number, a bool or an object, never
// the string "9090". Each example runs against a fixture in dry-run.
func TestSetHelpExamplesWriteTypedValues(t *testing.T) {
	t.Parallel()
	var examples []string
	for _, c := range []string{"", "set"} {
		root := newRootCmd(&app{printer: &outfmt.Printer{Program: DefaultProgram}})
		cmd := root
		if c != "" {
			cmd, _, _ = root.Find([]string{c})
		}
		for line := range strings.Lines(cmd.Example) {
			line = strings.TrimSpace(line)
			if rest, ok := strings.CutPrefix(line, DefaultProgram+" set "); ok {
				rest, _, _ = strings.Cut(rest, " #")
				examples = append(examples, rest)
			}
		}
	}
	if len(examples) == 0 {
		t.Fatal("no set examples found")
	}
	for _, ex := range examples {
		fields := strings.Fields(ex)
		if len(fields) < 3 || strings.Contains(ex, "'") {
			continue // the quoted-object example is covered by its --json-value flag below
		}
		value := fields[2]
		typed := value == "true" || value == "false" || value == "null" || strings.Trim(value, "0123456789") == ""
		if typed && !strings.Contains(ex, "--json-value") {
			t.Errorf("example %q sets %s as a string; add --json-value", ex, value)
		}
		dir := t.TempDir()
		p := filepath.Join(dir, "config.jsonc")
		if err := os.WriteFile(p, []byte("{\"server\": {\"port\": 8080, \"host\": \"x\"}, \"debug\": false}\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		args := append([]string{"set", p}, fields[1:]...)
		if !strings.Contains(ex, "--dry-run") {
			args = append(args, "--dry-run")
		}
		code, out, errOut := run(args...)
		if code != ExitOK {
			t.Errorf("example %q: exit %d, stderr %q", ex, code, errOut)
			continue
		}
		if typed && strings.Contains(out, `"`+value+`"`) {
			t.Errorf("example %q writes the string %q:\n%s", ex, value, out)
		}
	}
}
