package jsonccmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const messy = "{\n    // port\n  \"port\": 8080, \"tags\": [1,2,],\n}\n"

func TestFmt(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"default", nil, "{\n  // port\n  \"port\": 8080,\n  \"tags\": [\n    1,\n    2\n  ]\n}\n"},
		{"indent 4", []string{"--indent", "4"}, "{\n    // port\n    \"port\": 8080,\n    \"tags\": [\n        1,\n        2\n    ]\n}\n"},
		{"tabs and trailing commas", []string{"--tabs", "--trailing-commas"}, "{\n\t// port\n\t\"port\": 8080,\n\t\"tags\": [\n\t\t1,\n\t\t2,\n\t],\n}\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := writeFile(t, "c.jsonc", messy)
			code, out, errOut := run(append([]string{"fmt", f}, tt.args...)...)
			if code != ExitOK || out != "" || errOut != "" {
				t.Fatalf("exit %d out %q err %q", code, out, errOut)
			}
			if got := readFile(t, f); got != tt.want {
				t.Errorf("file\n%q\nwant\n%q", got, tt.want)
			}
			if info, _ := os.Stat(f); info.Mode().Perm() != 0o640 {
				t.Errorf("mode %v, want 0640 kept", info.Mode().Perm())
			}
			// Formatted: --check passes and a second run writes nothing.
			if code, out, _ := run(append([]string{"fmt", f, "--check"}, tt.args...)...); code != ExitOK || out != "" {
				t.Errorf("check after fmt: %d %q", code, out)
			}
		})
	}
}

func TestFmtCheckAndDryRun(t *testing.T) {
	t.Parallel()
	f := writeFile(t, "c.jsonc", messy)
	old := time.Now().Add(-time.Hour).Truncate(time.Second)
	if err := os.Chtimes(f, old, old); err != nil {
		t.Fatal(err)
	}
	code, out, errOut := run("fmt", f, "--check")
	if code != ExitFailure || out != f+"\n" || errOut != "" {
		t.Errorf("--check: %d %q %q", code, out, errOut)
	}
	code, out, _ = run("fmt", f, "--check", "--json")
	want := `{"ok":true,"data":{"file":"` + f + `","changed":true,"written":false,"check":true,"dry_run":false}}` + "\n"
	if code != ExitFailure || out != want {
		t.Errorf("--check --json: %d\n%s\nwant\n%s", code, out, want)
	}
	code, out, _ = run("fmt", f, "--dry-run")
	if code != ExitOK || !strings.HasPrefix(out, "-    // port\n") || !strings.Contains(out, "+  // port\n") {
		t.Errorf("--dry-run: %d %q", code, out)
	}
	if readFile(t, f) != messy {
		t.Fatal("--check or --dry-run wrote the file")
	}
	if info, _ := os.Stat(f); !info.ModTime().Equal(old) {
		t.Error("--check or --dry-run touched the file")
	}

	// An already formatted file is not rewritten.
	clean := writeFile(t, "d.jsonc", "{\n  \"a\": 1\n}\n")
	if err := os.Chtimes(clean, old, old); err != nil {
		t.Fatal(err)
	}
	_, out, _ = run("fmt", clean, "--json")
	if !strings.Contains(out, `"changed":false,"written":false`) {
		t.Errorf("clean --json: %s", out)
	}
	if info, _ := os.Stat(clean); !info.ModTime().Equal(old) {
		t.Error("fmt rewrote a formatted file")
	}
}

func TestFmtFailures(t *testing.T) {
	t.Parallel()
	f := writeFile(t, "c.jsonc", messy)
	bad := writeFile(t, "bad.jsonc", "{\"a\": }")
	tests := []struct {
		args []string
		code int
		ec   string
	}{
		{[]string{"fmt", f, "--indent", "0"}, ExitUsage, "usage"},
		{[]string{"fmt", f, "--check", "--dry-run"}, ExitUsage, "usage"},
		{[]string{"fmt", bad}, ExitFailure, "syntax"},
		{[]string{"fmt", filepath.Join(t.TempDir(), "none.jsonc")}, ExitFailure, "io"},
		{[]string{"fmt", f, "--strict"}, ExitFailure, "syntax"},
	}
	for _, tt := range tests {
		code, out, _ := run(append(tt.args, "--json")...)
		e := decodeEnvelope(t, out)
		if code != tt.code || e.Error == nil || e.Error.Code != tt.ec {
			t.Errorf("%v: %d %s", tt.args, code, out)
		}
	}
	if readFile(t, f) != messy || readFile(t, bad) != "{\"a\": }" {
		t.Error("a failed fmt changed a file")
	}
}

const schemaDoc = `{
  "type": "object",
  "required": ["port"],
  "properties": {
    "port": {"type": "integer", "minimum": 1},
    "tags": {"type": "array"}
  },
  "additionalProperties": false
}`

func TestValidate(t *testing.T) {
	t.Parallel()
	s := writeFile(t, "s.json", schemaDoc)
	good := writeFile(t, "good.jsonc", "{\n  // ok\n  \"port\": 80,\n  \"tags\": [],\n}\n")
	extra := writeFile(t, "extra.jsonc", "{\n  \"port\": 80,\n  \"x\": 1\n}\n")
	bad := writeFile(t, "bad.jsonc", "{\n  \"port\": 0,\n  \"port\": \"80\",\n  \"tags\": [1,],\n}\n")

	// Clean file: silent, exit 0, with and without a schema.
	for _, args := range [][]string{{"validate", good}, {"validate", good, "--schema", s}} {
		if code, out, errOut := run(args...); code != ExitOK || out != "" || errOut != "" {
			t.Errorf("%v: %d %q %q", args, code, out, errOut)
		}
	}

	// A warning prints but does not fail.
	code, out, _ := run("validate", extra, "--schema", s)
	if want := extra + ":3:3: warning: x: unknown key \"x\" (additionalProperties)\n"; code != ExitOK || out != want {
		t.Errorf("warning: %d %q, want %q", code, out, want)
	}

	// Errors: lint and schema, sorted by position, `file:line:col: msg (rule)`.
	code, out, _ = run("validate", bad, "--schema", s, "--forbid-trailing-commas")
	want := []string{
		bad + `:3:3: duplicate key "port" (first at 2:3); the last one wins (duplicate-key)`,
		bad + `:3:11: port: must be integer, got string (type)`,
		bad + `:4:13: trailing comma after the last element (trailing-comma)`,
		bad + `:4:15: trailing comma after the last member (trailing-comma)`,
	}
	if code != ExitFailure || out != strings.Join(want, "\n")+"\n" {
		t.Errorf("errors: %d\n%s\nwant\n%s", code, out, strings.Join(want, "\n"))
	}

	// Without the schema or the trailing-comma rule, only lint's defaults.
	if code, out, _ := run("validate", bad); code != ExitFailure || strings.Count(out, "\n") != 1 || !strings.Contains(out, "(duplicate-key)") {
		t.Errorf("lint only: %d %q", code, out)
	}

	code, out, _ = run("validate", extra, "--schema", s, "--json")
	e := decodeEnvelope(t, out)
	var p validatePayload
	if code != ExitOK || !e.OK || json.Unmarshal(e.Data, &p) != nil || !p.Valid || len(p.Errors) != 0 || len(p.Warnings) != 1 {
		t.Fatalf("--json warning: %d %s", code, out)
	}
	if w := p.Warnings[0]; w.Source != "schema" || w.Rule != "additionalProperties" || w.Path != "x" || w.Line != 3 || w.Col != 3 || w.File != extra {
		t.Errorf("warning issue: %+v", w)
	}
	code, out, _ = run("validate", bad, "--schema", s, "--json")
	if code != ExitFailure || !strings.Contains(out, `"valid":false`) || !strings.Contains(out, `"source":"lint","rule":"duplicate-key"`) {
		t.Errorf("--json errors: %d %s", code, out)
	}
}

func TestValidateSyntaxAndFailures(t *testing.T) {
	t.Parallel()
	broken := writeFile(t, "b.jsonc", "{\n  \"a\": 1,\n  \"a\": 2\n  \"c\": 3\n}\n")
	s := writeFile(t, "s.json", schemaDoc)
	code, out, _ := run("validate", broken, "--schema", s)
	if code != ExitFailure || !strings.Contains(out, broken+":4:3: ") || !strings.Contains(out, "(syntax)") {
		t.Errorf("syntax: %d %q", code, out)
	}
	if strings.Contains(out, "(type)") || strings.Contains(out, "(required)") {
		t.Errorf("schema ran on a broken document: %q", out)
	}
	commented := writeFile(t, "c.json", "{\"a\": 1 // x\n}")
	if code, out, _ := run("validate", commented, "--strict"); code != ExitFailure || !strings.Contains(out, "(syntax)") {
		t.Errorf("strict: %d %q", code, out)
	}

	badSchema := writeFile(t, "bad.schema.json", `{"type": 5}`)
	good := writeFile(t, "g.jsonc", `{"port": 1}`)
	tests := []struct {
		args []string
		ec   string
	}{
		{[]string{"validate", good, "--schema", badSchema}, "schema"},
		{[]string{"validate", good, "--schema", filepath.Join(t.TempDir(), "none.json")}, "io"},
		{[]string{"validate", filepath.Join(t.TempDir(), "none.jsonc")}, "io"},
	}
	for _, tt := range tests {
		code, out, _ := run(append(tt.args, "--json")...)
		e := decodeEnvelope(t, out)
		if code != ExitFailure || e.Error == nil || e.Error.Code != tt.ec {
			t.Errorf("%v: %d %s", tt.args, code, out)
		}
	}
}
