package jsonc

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/ubgo/textdata/plugin"
)

const dynamic = `{
  // keys are never expanded
  "${HOST}": "kept",
  "url": "postgres://${HOST:-localhost}:${PORT}/app",
  "pass": "${gopass:personal/db}",
  "cost": "$$5",
  "nested": {"list": ["${HOST}", 1, true]},
  "cmd": "$(echo hi)",
  "need": "${NOPE:-fallback}"
}
`

type vault struct {
	down bool
	seen []string
}

func (v *vault) Name() string      { return "vault" }
func (v *vault) Schemes() []string { return []string{"gopass"} }
func (v *vault) Resolve(scheme, ref string) (string, error) {
	if v.down {
		return "", plugin.ErrUnavailable
	}
	return "secret-for-" + ref, nil
}
func (v *vault) TransformValue(p, value string) (string, error) {
	v.seen = append(v.seen, p)
	return value, nil
}

func env(m map[string]string) func(string) (string, bool) {
	return func(k string) (string, bool) { v, ok := m[k]; return v, ok }
}

func TestExpanded(t *testing.T) {
	t.Parallel()
	v := &vault{}
	d := mustParse(t, dynamic, WithLookup(env(map[string]string{"HOST": "db", "PORT": "5432"})), WithPlugin(v))
	x, err := d.Expanded()
	if err != nil {
		t.Fatal(err)
	}
	for p, want := range map[string]any{
		`"${HOST}"`:      "kept",
		"url":            "postgres://db:5432/app",
		"pass":           "secret-for-personal/db",
		"cost":           "$5",
		"nested.list[0]": "db",
		"cmd":            "$(echo hi)", // substitution is off
		"need":           "fallback",
	} {
		if got, err := x.Get(p); err != nil || got != want {
			t.Errorf("expanded %s = %v, %v; want %v", p, got, err, want)
		}
	}
	if string(d.Render()) != dynamic {
		t.Error("Expanded modified the original document")
	}
	if !strings.Contains(string(x.Render()), "// keys are never expanded") {
		t.Error("Expanded dropped comments")
	}
	if got, _ := d.Get("url"); got != "postgres://${HOST:-localhost}:${PORT}/app" {
		t.Errorf("Get returns raw values: %v", got)
	}
	if got, err := d.GetExpanded("url"); err != nil || got != "postgres://db:5432/app" {
		t.Errorf("GetExpanded = %v, %v", got, err)
	}
	if got, err := d.GetExpanded("nested"); err != nil || got.(map[string]any)["list"].([]any)[0] != "db" {
		t.Errorf("GetExpanded(container) = %v, %v", got, err)
	}
	if strings.Join(v.seen, ",") == "" || !strings.Contains(strings.Join(v.seen, ","), "nested.list[0]") {
		t.Errorf("transformer paths = %v", v.seen)
	}

	v.down = true
	if _, err := d.Expanded(); !errors.Is(err, plugin.ErrUnavailable) || !strings.Contains(err.Error(), `jsonc: plugin "vault": resolve gopass:personal/db`) {
		t.Errorf("down resolver = %v", err)
	}
}

func TestExpandOptions(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("needs sh")
	}
	d := mustParse(t, `{"a": "$(printf hi)", "b": "${X:?must be set}"}`, WithCommandSubstitution(true))
	if got, err := d.GetExpanded("a"); err != nil || got != "hi" {
		t.Errorf("command = %v, %v", got, err)
	}
	if _, err := d.GetExpanded("b"); err == nil || !strings.Contains(err.Error(), `expand "b": expand: required variable X is not set or is empty: must be set`) {
		t.Errorf("required = %v", err)
	}
	if _, err := d.Expanded(); err == nil {
		t.Error("Expanded hid a required error")
	}

	var runs []string
	runner := runnerPlugin{fn: func(c string) (string, error) { runs = append(runs, c); return "R", nil }}
	d = mustParse(t, `{"a": "$(x)"}`, WithPlugin(runner))
	if got, _ := d.GetExpanded("a"); got != "R" || len(runs) != 1 {
		t.Errorf("CommandRunner plugin = %v %v", got, runs)
	}
	bad := mustParse(t, `{"a": "$X"}`, WithLookup(nil), WithPlugin(failLookup{}))
	if _, err := bad.GetExpanded("a"); err == nil || !strings.Contains(err.Error(), `plugin "fail": lookup X`) {
		t.Errorf("lookup error = %v", err)
	}
	loop := mustParse(t, `{"a": "$A"}`, WithLookup(env(map[string]string{"A": "<$A>"})))
	if got, _ := loop.GetExpanded("a"); got != "<>" {
		t.Errorf("self-reference = %v", got)
	}
	broken, _ := Parse([]byte("{"))
	if _, err := broken.Expanded(); !errors.Is(err, ErrInvalid) {
		t.Errorf("Expanded on an invalid doc = %v", err)
	}
}

type runnerPlugin struct{ fn func(string) (string, error) }

func (runnerPlugin) Name() string                          { return "runner" }
func (r runnerPlugin) RunCommand(c string) (string, error) { return r.fn(c) }

type failLookup struct{}

func (failLookup) Name() string                                  { return "fail" }
func (failLookup) Lookup(string) (string, bool, error)           { return "", false, errors.New("down") }
func (failLookup) TransformValue(string, string) (string, error) { return "", errors.New("no") }

func TestReadWithExpand(t *testing.T) {
	t.Parallel()
	p := filepath.Join(t.TempDir(), "c.jsonc")
	if err := os.WriteFile(p, []byte(`{"u": "${U}"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	m, err := Read(p, WithExpand(), WithLookup(env(map[string]string{"U": "x"})))
	if err != nil || m["u"] != "x" {
		t.Errorf("Read expanded = %v, %v", m, err)
	}
	m, _ = Read(p, WithLookup(env(map[string]string{"U": "x"})))
	if m["u"] != "${U}" {
		t.Errorf("Read without WithExpand = %v", m)
	}
	if _, err := Read(p, WithExpand(), WithPlugin(failLookup{})); err == nil {
		t.Error("Read hid an expansion error")
	}
}
