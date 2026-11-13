package jsonc

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ubgo/textdata/expand"
	"github.com/ubgo/textdata/plugin"
	"github.com/ubgo/textdata/pos"
)

func TestExpandRequested(t *testing.T) {
	t.Parallel()
	if mustParse(t, `{}`).ExpandRequested() {
		t.Fatal("ExpandRequested without WithExpand")
	}
	if !mustParse(t, `{}`, WithExpand()).ExpandRequested() {
		t.Fatal("ExpandRequested with WithExpand")
	}
}

// TestExpandErrorsCarryPosition: expansion failures name file:line:col of
// the string value and still match the underlying error.
func TestExpandErrorsCarryPosition(t *testing.T) {
	t.Parallel()
	d := mustParse(t, "{\n  \"a\": \"ok\",\n  \"b\": \"${NEED:?set it}\"\n}", WithFileName("conf.jsonc"))
	for _, get := range []func() error{
		func() error { _, err := d.Expanded(); return err },
		func() error { _, err := d.GetExpanded("b"); return err },
	} {
		err := get()
		var pe *pos.Error
		var re *expand.RequiredError
		if !errors.As(err, &pe) || pe.Pos.Line != 3 || pe.Pos.Col != 8 || !errors.As(err, &re) || re.Key != "NEED" {
			t.Fatalf("err = %v", err)
		}
		if !strings.HasPrefix(err.Error(), `conf.jsonc:3:8: expand "b": `) {
			t.Fatalf("message = %q", err.Error())
		}
	}

	p := mustParse(t, `{"x": ["$V"]}`, WithPlugin(failLookup{}))
	_, err := p.Expanded()
	var pe *pos.Error
	var plErr *plugin.Error
	if !errors.As(err, &pe) || pe.Pos.Col != 8 || !errors.As(err, &plErr) {
		t.Fatalf("plugin err = %v", err)
	}

	tr := mustParse(t, `{"x": "v"}`, WithPlugin(failTransform{}))
	_, err = tr.GetExpanded("x")
	if !errors.As(err, &pe) || pe.Pos.Col != 7 || !errors.As(err, &plErr) || !errors.Is(err, errTransform) {
		t.Fatalf("transform err = %v", err)
	}
}

var errTransform = errors.New("no")

type failTransform struct{}

func (failTransform) Name() string                                  { return "tr" }
func (failTransform) TransformValue(string, string) (string, error) { return "", errTransform }

// memIncluder serves $extends bases from memory.
type memIncluder map[string]string

func (memIncluder) Name() string { return "mem" }

func (m memIncluder) Include(_, ref string) ([]byte, string, error) {
	s, ok := m[ref]
	if !ok {
		return nil, "", plugin.ErrNotFound
	}
	return []byte(s), ref, nil
}

// TestReadAppliesExtends: Read merges $extends bases when an Includer is
// installed, and leaves $extends an ordinary key otherwise.
func TestReadAppliesExtends(t *testing.T) {
	t.Parallel()
	file := filepath.Join(t.TempDir(), "app.jsonc")
	if err := os.WriteFile(file, []byte("{\n  \"$extends\": \"base\",\n  \"b\": 2 // own\n}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	m, err := Read(file, WithPlugin(memIncluder{"base": `{"a": 1, "b": 1}`}))
	if err != nil {
		t.Fatal(err)
	}
	if m["a"] != 1.0 || m["b"] != 2.0 {
		t.Fatalf("Read with Includer = %v", m)
	}
	if _, ok := m["$extends"]; ok {
		t.Fatalf("$extends kept: %v", m)
	}
	m, err = Read(file)
	if err != nil {
		t.Fatal(err)
	}
	if m["$extends"] != "base" || m["a"] != nil {
		t.Fatalf("Read without Includer = %v", m)
	}
}
