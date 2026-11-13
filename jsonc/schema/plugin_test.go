package schema_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ubgo/textdata/jsonc"
	"github.com/ubgo/textdata/jsonc/schema"
	"github.com/ubgo/textdata/plugin"
)

// TestPluginGuardsSave: installing schema.Plugin is enough for Save to
// refuse a document with schema errors (the file on disk keeps the good
// version), while warnings alone never refuse a save.
func TestPluginGuardsSave(t *testing.T) {
	t.Parallel()
	s := mustSchema(t, `{"properties":{"port":{"type":"integer","maximum":65535}},"additionalProperties":false}`)
	var p plugin.Plugin = schema.Plugin(s)
	if _, ok := p.(plugin.SaveGuard); !ok {
		t.Fatal("schema.Plugin is not a plugin.SaveGuard")
	}

	name := filepath.Join(t.TempDir(), "c.jsonc")
	if err := os.WriteFile(name, []byte(`{"port": 80, "extra": 1}`), 0o600); err != nil {
		t.Fatal(err)
	}
	d, err := jsonc.Open(name, jsonc.WithPlugin(p))
	if err != nil {
		t.Fatal(err)
	}
	// "extra" is only a warning: the save goes through.
	if err := errors.Join(d.Set("port", 8080), d.Save()); err != nil {
		t.Fatalf("save with only warnings: %v", err)
	}
	if err := d.Set("port", 70000); err != nil {
		t.Fatal(err)
	}
	err = d.Save()
	if err == nil || !strings.Contains(err.Error(), "port: must be <= 65535") || !strings.Contains(err.Error(), `"schema"`) {
		t.Fatalf("save of an invalid document = %v", err)
	}
	b, err := os.ReadFile(name)
	if err != nil || !strings.Contains(string(b), `"port": 8080`) {
		t.Fatalf("file on disk = %s, %v", b, err)
	}
}
