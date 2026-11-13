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

type guard struct{ calls *int }

func (guard) Name() string { return "no-secrets" }
func (g guard) GuardSave(doc plugin.Document) error {
	*g.calls++
	if v, _ := doc.ValueAt("token"); v != "" {
		return errors.New("token must stay empty in a committed file")
	}
	return nil
}

type checker struct{}

func (checker) Name() string { return "schema" }
func (checker) Validate(doc plugin.Document) error {
	if _, ok := doc.ValueAt("port"); !ok {
		return errors.New("port is required")
	}
	return nil
}

func TestSave(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	p := filepath.Join(dir, "c.jsonc")
	src := "{\n  // keep\n  \"port\": 1,\n  \"token\": \"\"\n}\n"
	if err := os.WriteFile(p, []byte(src), 0o640); err != nil {
		t.Fatal(err)
	}
	calls := 0
	d, err := Open(p, WithPlugin(guard{&calls}, checker{}))
	if err != nil {
		t.Fatal(err)
	}
	if err := d.Validate(); err != nil {
		t.Errorf("Validate = %v", err)
	}
	if err := d.Set("port", 2); err != nil {
		t.Fatal(err)
	}
	if err := d.Save(); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(p)
	if string(b) != "{\n  // keep\n  \"port\": 2,\n  \"token\": \"\"\n}\n" {
		t.Errorf("saved %q", b)
	}
	if st, _ := os.Stat(p); runtime.GOOS != "windows" && st.Mode().Perm() != 0o640 {
		t.Errorf("mode = %v", st.Mode().Perm())
	}

	_ = d.Set("token", "abc")
	if err := d.Save(); err == nil || !strings.Contains(err.Error(), `jsonc: plugin "no-secrets": guard save`) {
		t.Errorf("guard did not veto: %v", err)
	}
	if b2, _ := os.ReadFile(p); string(b2) != string(b) {
		t.Error("vetoed save changed the file")
	}
	if calls != 2 {
		t.Errorf("guard calls = %d", calls)
	}

	_ = d.Delete("port")
	if err := d.Validate(); err == nil || !strings.Contains(err.Error(), `plugin "schema": validate: port is required`) {
		t.Errorf("Validate = %v", err)
	}
	if got := d.Plugins(); len(got) != 2 || got[0].String() != "no-secrets: SaveGuard" || got[1].String() != "schema: Validator" {
		t.Errorf("Plugins = %v", got)
	}

	// SaveAs: new file gets the doc's mode; Path unchanged.
	q := filepath.Join(dir, "copy.jsonc")
	plain, _ := Open(p)
	if err := plain.SaveAs(q); err != nil || plain.Path() != p {
		t.Fatalf("SaveAs = %v", err)
	}
	if b3, _ := os.ReadFile(q); string(b3) != string(b) {
		t.Error("SaveAs content")
	}
	if err := plain.SaveAs(""); err == nil {
		t.Error("SaveAs(\"\") accepted")
	}
	noPath, _ := Parse([]byte("{}"))
	if err := noPath.Save(); err == nil {
		t.Error("Save without a path accepted")
	}
	noPath.SetPath(filepath.Join(dir, "new.jsonc"))
	if err := noPath.Save(); err != nil {
		t.Fatal(err)
	}
	if st, _ := os.Stat(noPath.Path()); runtime.GOOS != "windows" && st.Mode().Perm() != defaultFileMode {
		t.Errorf("new file mode = %v", st.Mode().Perm())
	}
	if err := noPath.SaveAs(filepath.Join(dir, "missing", "x.jsonc")); err == nil {
		t.Error("SaveAs into a missing dir succeeded")
	}

	// A broken document is saved byte for byte, never "fixed".
	broken, _ := Parse([]byte("{ oops"))
	broken.SetPath(filepath.Join(dir, "broken.jsonc"))
	if err := broken.Save(); err != nil {
		t.Fatal(err)
	}
	if b4, _ := os.ReadFile(broken.Path()); string(b4) != "{ oops" {
		t.Errorf("broken save = %q", b4)
	}
	if err := broken.Validate(); !errors.Is(err, ErrInvalid) {
		t.Errorf("Validate broken = %v", err)
	}
}

func TestClone(t *testing.T) {
	t.Parallel()
	d := mustParse(t, `{"a": 1}`)
	c := d.Clone()
	_ = c.Set("a", 2)
	if v, _ := d.GetInt("a"); v != 1 {
		t.Error("Clone shares state")
	}
	r := d.Render()
	r[0] = 'X'
	if d.Render()[0] != '{' {
		t.Error("Render returned the internal slice")
	}
}

// TestSaveFirstRun: Open on a missing file, Set, Save creates it.
func TestSaveFirstRun(t *testing.T) {
	t.Parallel()
	p := filepath.Join(t.TempDir(), "fresh.jsonc")
	d, err := Open(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := d.Set("sync.remote", "origin"); err != nil {
		t.Fatal(err)
	}
	if err := d.Save(); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(p)
	if string(b) != "{\n  \"sync\": {\n    \"remote\": \"origin\"\n  }\n}\n" {
		t.Errorf("fresh file = %q", b)
	}
}
