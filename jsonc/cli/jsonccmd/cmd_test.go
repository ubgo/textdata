package jsonccmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"testing"
	"time"
)

// fixture exercises what every edit must keep: comments above and beside
// members, a block comment, a trailing comma, blank lines.
const fixture = `{
  // server settings
  "server": {
    "host": "localhost", // dev only
    "port": 8080,
    "debug": true,
  },

  /* upstreams */
  "hosts": ["a", "b", "c"],
  "id": 9007199254740993,
  "name": "<app>"
}
`

func writeFile(t *testing.T, name, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte(content), 0o640); err != nil {
		t.Fatal(err)
	}
	return p
}

func readFile(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func run(args ...string) (code int, stdout, stderr string) {
	var out, errb bytes.Buffer
	code = Execute(args, &out, &errb)
	return code, out.String(), errb.String()
}

// envelope is the clikit/outfmt wire shape.
type envelope struct {
	OK    bool            `json:"ok"`
	Data  json.RawMessage `json:"data"`
	Error *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func decodeEnvelope(t *testing.T, out string) envelope {
	t.Helper()
	if strings.Count(out, "\n") != 1 {
		t.Fatalf("want exactly one envelope line, got %q", out)
	}
	var e envelope
	if err := json.Unmarshal([]byte(out), &e); err != nil {
		t.Fatalf("not an envelope: %q: %v", out, err)
	}
	return e
}

func TestGet(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		path string
		code int
		out  string // stdout on success, stderr on failure
	}{
		{"string prints bare", "server.host", ExitOK, "localhost\n"},
		{"number literal kept", "id", ExitOK, "9007199254740993\n"},
		{"bool", "server.debug", ExitOK, "true\n"},
		{"pointer", "/hosts/1", ExitOK, "b\n"},
		{"no html escaping", "name", ExitOK, "<app>\n"},
		{"object as indented json without comments", "server", ExitOK, "{\n  \"host\": \"localhost\",\n  \"port\": 8080,\n  \"debug\": true\n}\n"},
		{"array", "hosts", ExitOK, "[\n  \"a\",\n  \"b\",\n  \"c\"\n]\n"},
		{"missing path", "server.nope", ExitFailure, "textdatactl jsonc: "},
		{"bad path is usage", `a."b`, ExitUsage, "textdatactl jsonc: "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := writeFile(t, "c.jsonc", fixture)
			code, out, errOut := run("get", f, tt.path)
			if code != tt.code {
				t.Fatalf("exit %d, want %d (out %q err %q)", code, tt.code, out, errOut)
			}
			if code != ExitOK {
				if out != "" || !strings.HasPrefix(errOut, tt.out) {
					t.Errorf("failure: out %q err %q", out, errOut)
				}
				return
			}
			if out != tt.out || errOut != "" {
				t.Errorf("out %q err %q, want %q", out, errOut, tt.out)
			}
		})
	}
}

func TestGetJSON(t *testing.T) {
	t.Parallel()
	f := writeFile(t, "c.jsonc", fixture)
	code, out, _ := run("get", f, "server", "--json")
	if code != ExitOK {
		t.Fatalf("exit %d", code)
	}
	want := `{"ok":true,"data":{"path":"server","kind":"object","value":{"host":"localhost","port":8080,"debug":true}}}` + "\n"
	if out != want {
		t.Errorf("out\n%s\nwant\n%s", out, want)
	}
	_, out, _ = run("get", f, "id", "--json")
	if want := `{"ok":true,"data":{"path":"id","kind":"number","value":9007199254740993}}` + "\n"; out != want {
		t.Errorf("number: %s", out)
	}
	code, out, errOut := run("get", f, "nope", "--json")
	e := decodeEnvelope(t, out)
	if code != ExitFailure || e.OK || e.Error.Code != "not_found" || errOut != "" {
		t.Errorf("missing: %d %q %q", code, out, errOut)
	}
}

func TestReadFailures(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	missing := filepath.Join(dir, "none.jsonc")
	bad := writeFile(t, "bad.jsonc", "{\n  \"a\": ,\n}")
	commented := writeFile(t, "c.json", "{\"a\": 1, // x\n}")
	tests := []struct {
		args []string
		code int
		ec   string
		msg  string
	}{
		{[]string{"get", missing, "a"}, ExitFailure, "io", "no such file"},
		{[]string{"get", bad, "a"}, ExitFailure, "syntax", "bad.jsonc:2:"},
		{[]string{"to-json", bad}, ExitFailure, "syntax", "bad.jsonc:2:"},
		{[]string{"diff", missing, bad}, ExitFailure, "io", "no such file"},
		{[]string{"get", commented, "a", "--strict"}, ExitFailure, "syntax", "c.json:1:"},
		{[]string{"get", commented}, ExitUsage, "usage", "accepts 2 arg(s)"},
		{[]string{"nosuchverb"}, ExitUsage, "usage", "unknown command"},
		{[]string{"get", commented, "a", "--bogus"}, ExitUsage, "usage", "unknown flag"},
	}
	for _, tt := range tests {
		code, out, errOut := run(tt.args...)
		if code != tt.code || out != "" || !strings.Contains(errOut, tt.msg) || !strings.HasPrefix(errOut, "textdatactl jsonc: ") {
			t.Errorf("%v: %d out %q err %q", tt.args, code, out, errOut)
		}
		code, out, errOut = run(append(tt.args, "--json")...)
		e := decodeEnvelope(t, out)
		if code != tt.code || e.OK || e.Error == nil || e.Error.Code != tt.ec || !strings.Contains(e.Error.Message, tt.msg) || errOut != "" {
			t.Errorf("%v --json: %d out %q err %q", tt.args, code, out, errOut)
		}
	}
}

func TestSet(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		args []string
		want string // the file after the edit
	}{
		{"string in place", []string{"server.host", "api.local"}, strings.Replace(fixture, `"localhost", // dev`, `"api.local", // dev`, 1)},
		{"string stays a string", []string{"server.port", "9090"}, strings.Replace(fixture, `8080`, `"9090"`, 1)},
		{"json number", []string{"server.port", "9090", "--json-value"}, strings.Replace(fixture, `8080`, `9090`, 1)},
		{"json object verbatim", []string{"server.port", `{"z":1,"a":2}`, "--json-value"}, strings.Replace(fixture, `8080`, `{"z":1,"a":2}`, 1)},
		{"new key at end of object", []string{"server.tls", "true", "--json-value"}, strings.Replace(fixture, "    \"debug\": true,\n", "    \"debug\": true,\n    \"tls\": true,\n", 1)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := writeFile(t, "c.jsonc", fixture)
			code, out, errOut := run(append([]string{"set", f}, tt.args...)...)
			if code != ExitOK || out != "" || errOut != "" {
				t.Fatalf("exit %d out %q err %q", code, out, errOut)
			}
			if got := readFile(t, f); got != tt.want {
				t.Errorf("file\n%s\nwant\n%s", got, tt.want)
			}
			info, _ := os.Stat(f)
			if info.Mode().Perm() != 0o640 {
				t.Errorf("mode %v, want 0640 kept", info.Mode().Perm())
			}
		})
	}
}

func TestSetJSONEnvelopeAndCreate(t *testing.T) {
	t.Parallel()
	f := writeFile(t, "c.jsonc", fixture)
	code, out, _ := run("set", f, "server.tls", "true", "--json-value", "--json")
	want := `{"ok":true,"data":{"file":"` + f + `","path":"server.tls","created":true,"changed":true,"dry_run":false}}` + "\n"
	if code != ExitOK || out != want {
		t.Errorf("%d\n%s\nwant\n%s", code, out, want)
	}

	// A missing file is created, 0600.
	nf := filepath.Join(t.TempDir(), "new.jsonc")
	if code, _, errOut := run("set", nf, "a.b", "1", "--json-value"); code != ExitOK {
		t.Fatalf("create: %d %s", code, errOut)
	}
	if got := readFile(t, nf); got != "{\"a\": {\"b\": 1}}" && !strings.Contains(got, `"b": 1`) {
		t.Errorf("created %q", got)
	}
	if info, _ := os.Stat(nf); info.Mode().Perm() != 0o600 {
		t.Errorf("new file mode %v", info.Mode().Perm())
	}
}

func TestSetUsageAndNoOp(t *testing.T) {
	t.Parallel()
	f := writeFile(t, "c.jsonc", fixture)
	code, out, errOut := run("set", f, "server.port", "{nope", "--json-value")
	if code != ExitUsage || out != "" || !strings.Contains(errOut, "is not one JSON value") {
		t.Errorf("bad json value: %d %q %q", code, out, errOut)
	}
	if code, _, _ := run("set", f, "server.port", "", "--json-value"); code != ExitUsage {
		t.Errorf("empty json value: %d", code)
	}
	if code, _, errOut := run("set", f, "hosts[9]", "x"); code != ExitFailure || !strings.Contains(errOut, "not found") {
		t.Errorf("array element: %d %q", code, errOut)
	}
	if readFile(t, f) != fixture {
		t.Fatal("a failed set changed the file")
	}

	// Setting the current value writes nothing: the mtime stays.
	old := time.Now().Add(-time.Hour).Truncate(time.Second)
	if err := os.Chtimes(f, old, old); err != nil {
		t.Fatal(err)
	}
	code, out, _ = run("set", f, "server.port", "8080", "--json-value")
	if code != ExitOK || out != "no changes\n" {
		t.Errorf("no-op: %d %q", code, out)
	}
	if info, _ := os.Stat(f); !info.ModTime().Equal(old) {
		t.Errorf("no-op touched the file: %v", info.ModTime())
	}
}

func TestDryRun(t *testing.T) {
	t.Parallel()
	f := writeFile(t, "c.jsonc", fixture)
	code, out, _ := run("set", f, "server.port", "9090", "--json-value", "--dry-run")
	if want := "-    \"port\": 8080,\n+    \"port\": 9090,\n"; code != ExitOK || out != want {
		t.Errorf("dry run %d\n%q\nwant\n%q", code, out, want)
	}
	_, out, _ = run("unset", f, "server.debug", "--dry-run", "--json")
	e := decodeEnvelope(t, out)
	var p editPayload
	if err := json.Unmarshal(e.Data, &p); err != nil || !p.DryRun || !p.Changed || len(p.Diff) != 2 || p.Diff[1] != `+    // "debug": true,` {
		t.Errorf("unset dry run %s", out)
	}
	if readFile(t, f) != fixture {
		t.Fatal("dry run wrote the file")
	}
}

func TestUnsetRestore(t *testing.T) {
	t.Parallel()
	f := writeFile(t, "c.jsonc", fixture)
	if code, _, errOut := run("unset", f, "server.debug"); code != ExitOK {
		t.Fatalf("unset: %d %s", code, errOut)
	}
	want := strings.Replace(fixture, `    "debug": true,`, `    // "debug": true,`, 1)
	if got := readFile(t, f); got != want {
		t.Fatalf("after unset\n%s", got)
	}
	if code, _, _ := run("get", f, "server.debug"); code != ExitFailure {
		t.Error("an unset member still reads")
	}
	if code, _, errOut := run("restore", f, "server.debug"); code != ExitOK {
		t.Fatalf("restore: %d %s", code, errOut)
	}
	if got := readFile(t, f); got != fixture {
		t.Fatalf("restore is not byte-exact:\n%s", got)
	}

	tests := []struct {
		args []string
		ec   string
	}{
		{[]string{"restore", f, "server.debug"}, "exists"},
		{[]string{"restore", f, "server.never"}, "not_found"},
		{[]string{"unset", f, "hosts[0]"}, "type"},
		{[]string{"unset", f, "server.debug", "--strict"}, "syntax"}, // the fixture has comments
	}
	for _, tt := range tests {
		code, out, _ := run(append(tt.args, "--json")...)
		e := decodeEnvelope(t, out)
		if code != ExitFailure || e.Error == nil || e.Error.Code != tt.ec {
			t.Errorf("%v: %d %s", tt.args, code, out)
		}
	}
	strictFile := writeFile(t, "s.json", "{\n  \"a\": 1\n}\n")
	_, out, _ := run("unset", strictFile, "a", "--strict", "--json")
	if e := decodeEnvelope(t, out); e.Error == nil || e.Error.Code != "strict" {
		t.Errorf("strict unset: %s", out)
	}
	layout := writeFile(t, "l.jsonc", `{"a": 1, "b": 2}`)
	_, out, _ = run("unset", layout, "a", "--json")
	if e := decodeEnvelope(t, out); e.Error == nil || e.Error.Code != "layout" {
		t.Errorf("layout unset: %s", out)
	}
	if readFile(t, f) != fixture || readFile(t, strictFile) != "{\n  \"a\": 1\n}\n" {
		t.Error("a refused edit changed a file")
	}
}

func TestDelete(t *testing.T) {
	t.Parallel()
	f := writeFile(t, "c.jsonc", fixture)
	if code, _, errOut := run("delete", f, "server.host"); code != ExitOK {
		t.Fatalf("delete: %d %s", code, errOut)
	}
	if got, want := readFile(t, f), strings.Replace(fixture, "    \"host\": \"localhost\", // dev only\n", "", 1); got != want {
		t.Errorf("after delete\n%s", got)
	}
	if code, _, _ := run("delete", f, "hosts[1]"); code != ExitOK {
		t.Fatal("delete element")
	}
	if _, out, _ := run("get", f, "hosts", "--json"); !strings.Contains(out, `"value":["a","c"]`) {
		t.Errorf("hosts after delete: %s", out)
	}
	_, out, _ := run("delete", f, "", "--json")
	if e := decodeEnvelope(t, out); e.Error == nil || e.Error.Code != "type" {
		t.Errorf("delete root: %s", out)
	}
	if code, _, _ := run("delete", f, "nope"); code != ExitFailure {
		t.Error("delete of a missing path succeeded")
	}
}

func TestDiff(t *testing.T) {
	t.Parallel()
	a := writeFile(t, "a.jsonc", fixture)
	b := writeFile(t, "b.jsonc", `{"server":{"host":"localhost","port":9090,"debug":true,"tls":true},"hosts":["a","b"],"id":9007199254740993,"name":null}`)
	same := writeFile(t, "same.json", `{"name":"<app>","id":9007199254740993,"hosts":["a","b","c"],"server":{"debug":true,"port":8080.0,"host":"localhost"}}`)

	code, out, errOut := run("diff", a, b)
	want := "- hosts[2]: \"c\"\n~ name: \"<app>\" -> null\n~ server.port: 8080 -> 9090\n+ server.tls: true\n"
	if code != ExitFailure || out != want || errOut != "" {
		t.Errorf("diff %d\n%s\nwant\n%s\nerr %q", code, out, want, errOut)
	}

	code, out, _ = run("diff", a, same)
	if code != ExitOK || out != "" {
		t.Errorf("same: %d %q", code, out)
	}
	_, out, _ = run("diff", a, same, "--json")
	if out != `{"ok":true,"data":{"equal":true,"changes":[]}}`+"\n" {
		t.Errorf("same --json: %s", out)
	}

	code, out, _ = run("diff", a, b, "--json")
	e := decodeEnvelope(t, out)
	var p diffPayload
	if code != ExitFailure || !e.OK || json.Unmarshal(e.Data, &p) != nil || p.Equal || len(p.Changes) != 4 {
		t.Fatalf("diff --json: %d %s", code, out)
	}
	removed, changed := p.Changes[0], p.Changes[1]
	if removed.New != nil || removed.NewPos != nil || string(removed.Old) != `"c"` || *removed.OldPos != (wirePos{File: a, Line: 10, Col: 23}) {
		t.Errorf("removed: %+v", removed)
	}
	if string(changed.New) != "null" || changed.Pointer != "/name" || changed.NewPos == nil || changed.NewPos.File != b {
		t.Errorf("changed to null: %+v", changed)
	}
}

func TestToJSON(t *testing.T) {
	t.Parallel()
	f := writeFile(t, "c.jsonc", "\ufeff{\n  // c\n  \"a\": [1, 2,], /* x */\n}")
	code, out, _ := run("to-json", f)
	if want := "{\n  \"a\": [1, 2]\n}\n"; code != ExitOK || out != want {
		t.Errorf("to-json %d %q, want %q", code, out, want)
	}
	if !json.Valid([]byte(out)) {
		t.Error("not JSON")
	}
	_, out, _ = run("to-json", f, "--json")
	if out != `{"ok":true,"data":{"value":{"a":[1,2]}}}`+"\n" {
		t.Errorf("--json: %s", out)
	}
	empty := writeFile(t, "e.jsonc", "")
	if _, out, _ := run("to-json", empty); out != "null\n" {
		t.Errorf("empty: %q", out)
	}
}

func TestHelpAndProgramName(t *testing.T) {
	t.Parallel()
	code, out, _ := run("--help")
	for _, w := range []string{"textdatactl jsonc [command]", "get", "set", "unset", "restore", "delete", "diff", "to-json", "fmt", "validate", "Examples:"} {
		if code != ExitOK || !strings.Contains(out, w) {
			t.Errorf("--help missing %q", w)
		}
	}
	if strings.Contains(out, "completion") {
		t.Error("--help lists a completion command")
	}
	for _, verb := range []string{"get", "set", "unset", "restore", "delete", "diff", "to-json", "fmt", "validate"} {
		if code, out, _ := run(verb, "--help"); code != ExitOK || !strings.Contains(out, "Examples:") || !strings.Contains(out, "textdatactl jsonc "+verb) {
			t.Errorf("%s --help: %d %q", verb, code, out)
		}
	}

	var o, e bytes.Buffer
	code = Execute([]string{"get", "/nonexistent/x.jsonc", "a"}, &o, &e, WithProgramName("myapp cfg"))
	if code != ExitFailure || !strings.HasPrefix(e.String(), "myapp cfg: ") {
		t.Errorf("program name: %d %q", code, e.String())
	}
	o.Reset()
	e.Reset()
	if Execute([]string{"nope"}, &o, &e, WithProgramName("myapp cfg")) != ExitUsage || !strings.Contains(e.String(), "Run 'myapp cfg --help' for usage.") {
		t.Errorf("usage hint: %q", e.String())
	}
	o.Reset()
	if Execute([]string{"--version"}, &o, &e, WithProgramName("myapp cfg")) != ExitOK || !strings.HasPrefix(o.String(), "myapp cfg version ") {
		t.Errorf("version: %q", o.String())
	}
}

// TestCommandGraft drives the tree the way a host that grafts it does.
func TestCommandGraft(t *testing.T) {
	t.Parallel()
	f := writeFile(t, "c.jsonc", fixture)
	c := Command()
	if c.Name() != "jsonc" {
		t.Errorf("name %q", c.Name())
	}
	var o, e bytes.Buffer
	c.SetOut(&o)
	c.SetErr(&e)
	c.SetArgs([]string{"get", f, "server.port"})
	if err := c.Execute(); err != nil || o.String() != "8080\n" {
		t.Errorf("graft get: %v %q", err, o.String())
	}

	c = Command()
	o.Reset()
	c.SetOut(&o)
	c.SetErr(&e)
	c.SetArgs([]string{"get", f, "nope", "--json"})
	err := c.Execute()
	var xe *ExitError
	if !errors.As(err, &xe) || xe.Code != ExitFailure || !xe.Quiet || !strings.Contains(o.String(), `"not_found"`) {
		t.Errorf("graft failure: %v %q", err, o.String())
	}
}

func TestJSONRequested(t *testing.T) {
	t.Parallel()
	tests := []struct {
		args []string
		want bool
	}{
		{nil, false},
		{[]string{"--json"}, true},
		{[]string{"--json=false"}, false},
		{[]string{"--json", "--json=maybe"}, false},
		{[]string{"--", "--json"}, false},
	}
	for _, tt := range tests {
		if got := jsonRequested(tt.args); got != tt.want {
			t.Errorf("%v = %v", tt.args, got)
		}
	}
}

func TestVersionString(t *testing.T) {
	t.Parallel()
	tests := []struct {
		info *debug.BuildInfo
		ok   bool
		want string
	}{
		{nil, false, devVersion},
		{&debug.BuildInfo{Main: debug.Module{Path: cliModulePath, Version: "v0.1.1"}}, true, "v0.1.1"},
		{&debug.BuildInfo{Main: debug.Module{Path: cliModulePath, Version: develStamp}}, true, devVersion},
		{&debug.BuildInfo{Main: debug.Module{Path: "host"}, Deps: []*debug.Module{{Path: cliModulePath, Version: "v0.2.0"}}}, true, "v0.2.0"},
		{&debug.BuildInfo{Main: debug.Module{Path: "host"}, Deps: []*debug.Module{{Path: cliModulePath, Version: "v0.2.0", Replace: &debug.Module{Path: "../x"}}}}, true, devVersion},
		{&debug.BuildInfo{Main: debug.Module{Path: "host"}}, true, devVersion},
	}
	for _, tt := range tests {
		info, ok := tt.info, tt.ok
		if got := versionString(func() (*debug.BuildInfo, bool) { return info, ok }); got != tt.want {
			t.Errorf("versionString = %q, want %q", got, tt.want)
		}
	}
}
