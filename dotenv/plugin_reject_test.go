package dotenv

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/ubgo/textdata/plugin"
)

// includerOnly stands in for the extends plugin: an Includer and nothing else.
type includerOnly struct{}

func (includerOnly) Name() string { return "extends" }
func (includerOnly) Include(from, ref string) ([]byte, string, error) {
	return nil, "", nil
}

// validatorOnly stands in for a schema checker with no SaveGuard side.
type validatorOnly struct{}

func (validatorOnly) Name() string                   { return "schema" }
func (validatorOnly) Validate(plugin.Document) error { return nil }

// includerAndLookuper holds one unsupported and one supported ability.
type includerAndLookuper struct{ includerOnly }

func (includerAndLookuper) Name() string { return "mixed" }
func (includerAndLookuper) Lookup(string) (string, bool, error) {
	return "", false, nil
}

// TestWithPlugin_RejectsUnsupported pins that a plugin whose only abilities
// dotenv never uses is refused with an error naming it and its abilities,
// instead of being installed and silently ignored.
func TestWithPlugin_RejectsUnsupported(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte("A=1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	const want = `dotenv: plugin "extends": no ability dotenv supports (Includer)`

	if _, err := Open(path, WithPlugin(includerOnly{})); err == nil || err.Error() != want {
		t.Errorf("Open error = %v, want %q", err, want)
	}
	if _, err := Read(path, WithPlugin(includerOnly{})); !errors.Is(err, ErrUnsupportedPlugin) {
		t.Errorf("Read error = %v, want ErrUnsupportedPlugin", err)
	}
	f := Parse("A=1\n", WithPlugin(validatorOnly{}))
	if _, _, err := f.GetExpanded("A"); err == nil || err.Error() != `dotenv: plugin "schema": no ability dotenv supports (Validator)` {
		t.Errorf("GetExpanded error = %v", err)
	}
	if _, err := f.ExpandedMap(); !errors.Is(err, ErrUnsupportedPlugin) {
		t.Errorf("ExpandedMap error = %v", err)
	}
	if err := f.SaveAs(filepath.Join(t.TempDir(), "x.env")); !errors.Is(err, ErrUnsupportedPlugin) {
		t.Errorf("SaveAs error = %v", err)
	}
	if n := len(f.Plugins()); n != 0 {
		t.Errorf("Plugins() = %d, want the refused plugin not installed", n)
	}

	// One supported ability is enough to install.
	m := Parse("A=1\n", WithPlugin(includerAndLookuper{}))
	if _, _, err := m.GetExpanded("A"); err != nil {
		t.Errorf("mixed plugin: GetExpanded error = %v", err)
	}
	if len(m.Plugins()) != 1 {
		t.Errorf("mixed plugin not installed: %+v", m.Plugins())
	}
}
