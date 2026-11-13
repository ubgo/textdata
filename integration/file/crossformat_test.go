package fileplugin_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/ubgo/textdata/dotenv"
	"github.com/ubgo/textdata/jsonc"
	"github.com/ubgo/textdata/plugin"
	fileplugin "github.com/ubgo/textdata/plugins/file"
)

// TestSameInstanceBothFormats installs ONE plugin instance in a dotenv file
// and a jsonc file opened from the same folder: both read the referenced file,
// both write the reference back unchanged, and confinement holds in both.
func TestSameInstanceBothFormats(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "cfg")
	if err := os.MkdirAll(cfg, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfg, "key.pem"), []byte("pem-body\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "outside"), []byte("nope\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	envPath := filepath.Join(cfg, ".env")
	envSrc := "KEY=${file:./key.pem}\nBAD=${file:../outside}\n"
	jsPath := filepath.Join(cfg, "app.jsonc")
	jsSrc := "{\n  // tls\n  \"tls\": {\"key\": \"${file:./key.pem}\"},\n  \"bad\": \"${file:../outside}\",\n}\n"
	if err := os.WriteFile(envPath, []byte(envSrc), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(jsPath, []byte(jsSrc), 0o600); err != nil {
		t.Fatal(err)
	}

	p := fileplugin.New(cfg) // confined to cfg by default

	env, err := dotenv.Open(envPath, dotenv.WithPlugin(p))
	if err != nil {
		t.Fatal(err)
	}
	if v, _, err := env.GetExpanded("KEY"); err != nil || v != "pem-body" {
		t.Fatalf("dotenv = %q, %v", v, err)
	}
	if _, _, err := env.GetExpanded("BAD"); !errors.Is(err, plugin.ErrDenied) {
		t.Errorf("dotenv escape = %v", err)
	}
	if env.Render() != envSrc {
		t.Error("dotenv render changed")
	}

	doc, err := jsonc.Open(jsPath, jsonc.WithPlugin(p))
	if err != nil {
		t.Fatal(err)
	}
	if v, err := doc.GetExpanded("tls.key"); err != nil || v != "pem-body" {
		t.Fatalf("jsonc = %v, %v", v, err)
	}
	if _, err := doc.GetExpanded("bad"); !errors.Is(err, plugin.ErrDenied) {
		t.Errorf("jsonc escape = %v", err)
	}
	if string(doc.Render()) != jsSrc {
		t.Error("jsonc render changed")
	}
}

// Example reads a key file next to a JSONC document.
func Example() {
	dir, _ := os.MkdirTemp("", "fileplugin-example")
	defer os.RemoveAll(dir)
	_ = os.WriteFile(filepath.Join(dir, "key.pem"), []byte("pem-body\n"), 0o600)

	p := fileplugin.New(dir) // refs are confined to dir by default
	doc, _ := jsonc.Parse([]byte(`{"key": "${file:./key.pem}"}`), jsonc.WithPlugin(p))
	v, err := doc.GetExpanded("key")
	fmt.Println(v, err)
	fmt.Println(string(doc.Render()))
	// Output:
	// pem-body <nil>
	// {"key": "${file:./key.pem}"}
}
