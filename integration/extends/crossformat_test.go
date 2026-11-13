package extends_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"testing/fstest"

	"github.com/ubgo/textdata/dotenv"
	"github.com/ubgo/textdata/jsonc"
	"github.com/ubgo/textdata/plugin"
	"github.com/ubgo/textdata/plugins/extends"
)

func writeAll(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for name, s := range files {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(s), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

// TestJSONCOnDisk drives jsonc's $extends through the real disk includer:
// nested references relative to each file, a confined root, a cycle.
func TestJSONCOnDisk(t *testing.T) {
	top := t.TempDir()
	root := filepath.Join(top, "repo")
	writeAll(t, top, map[string]string{
		"secret.jsonc":            `{"leak": true}`,
		"repo/base.jsonc":         "{\n  // shared\n  \"port\": 80,\n  \"db\": {\"host\": \"h\", \"pool\": 5}\n}\n",
		"repo/team/team.jsonc":    `{"$extends": "../base.jsonc", "db": {"pool": 7}, "team": "x"}`,
		"repo/app/app.jsonc":      "{\n  \"$extends\": [\"../team/team.jsonc\"],\n  \"port\": 8080\n}\n",
		"repo/app/escape.jsonc":   `{"$extends": "../../secret.jsonc"}`,
		"repo/app/loop.jsonc":     `{"$extends": "../app/./loop2.jsonc"}`,
		"repo/app/loop2.jsonc":    `{"$extends": "loop.jsonc"}`,
		"repo/app/missing.jsonc":  "{\n  \"$extends\": \"nope.jsonc\"\n}",
		"repo/app/abs.jsonc":      fmt.Sprintf(`{"$extends": %q}`, filepath.Join(root, "base.jsonc")),
		"repo/app/notobject.json": `[]`,
	})
	inc := extends.New(extends.WithRoot(root))

	appPath := filepath.Join(root, "app", "app.jsonc")
	d, err := jsonc.Open(appPath, jsonc.WithPlugin(inc))
	if err != nil {
		t.Fatal(err)
	}
	before := d.Render()
	x, err := d.Extended()
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := x.Decode(&got); err != nil {
		t.Fatal(err)
	}
	want := map[string]any{"port": 8080.0, "team": "x", "db": map[string]any{"host": "h", "pool": 7.0}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v want %v", got, want)
	}
	if string(d.Render()) != string(before) {
		t.Error("Render changed after Extended")
	}

	open := func(name string, opts ...extends.Option) error {
		d, err := jsonc.Open(filepath.Join(root, "app", name), jsonc.WithPlugin(extends.New(append([]extends.Option{extends.WithRoot(root)}, opts...)...)))
		if err != nil {
			t.Fatal(err)
		}
		_, err = d.Extended()
		return err
	}
	if err := open("escape.jsonc"); !errors.Is(err, plugin.ErrDenied) {
		t.Errorf("escape: %v, want ErrDenied", err)
	}
	if err := open("loop.jsonc"); !errors.Is(err, jsonc.ErrExtendsCycle) {
		t.Errorf("loop: %v, want ErrExtendsCycle", err)
	}
	if err := open("missing.jsonc"); !errors.Is(err, plugin.ErrNotFound) {
		t.Errorf("missing: %v, want ErrNotFound", err)
	}
	if err := open("abs.jsonc"); err != nil {
		t.Errorf("absolute inside the root: %v", err)
	}
	if err := open("abs.jsonc", extends.AllowAbsolute(false)); !errors.Is(err, plugin.ErrDenied) {
		t.Errorf("absolute refused: %v, want ErrDenied", err)
	}
}

// TestOneIncluderBothFormats: one includer instance serves jsonc's $extends
// and loads a dotenv base for a caller layering .env files, from the same
// folder with the same confinement. The includer never looks at the bytes.
func TestOneIncluderBothFormats(t *testing.T) {
	dir := t.TempDir()
	writeAll(t, dir, map[string]string{
		"base.env":      "# shared\nHOST=db\nPORT=5432\n",
		"base.jsonc":    `{"host": "db"}`,
		"app/.env":      "PORT=6543\n",
		"app/app.jsonc": `{"$extends": "../base.jsonc", "port": 6543}`,
	})
	inc := extends.New(extends.WithRoot(dir))

	d, err := jsonc.Open(filepath.Join(dir, "app", "app.jsonc"), jsonc.WithPlugin(inc))
	if err != nil {
		t.Fatal(err)
	}
	x, err := d.Extended()
	if err != nil {
		t.Fatal(err)
	}
	if host, _ := x.GetString("host"); host != "db" {
		t.Errorf("jsonc host = %q", host)
	}

	data, loc, err := inc.Include(filepath.Join(dir, "app", ".env"), "../base.env")
	if err != nil {
		t.Fatal(err)
	}
	base := dotenv.Parse(string(data))
	base.Path = loc
	if base.Render() != string(data) {
		t.Error("dotenv base does not round-trip")
	}
	app, err := dotenv.Open(filepath.Join(dir, "app", ".env"))
	if err != nil {
		t.Fatal(err)
	}
	merged := base.Map()
	for k, v := range app.Map() {
		merged[k] = v
	}
	if !reflect.DeepEqual(merged, map[string]string{"HOST": "db", "PORT": "6543"}) {
		t.Errorf("layered = %v", merged)
	}
}

// TestFSIncluderWithJSONC: an embedded or in-memory tree works the same way.
func TestFSIncluderWithJSONC(t *testing.T) {
	fsys := fstest.MapFS{
		"defaults/base.jsonc": {Data: []byte(`{"a": 1, "b": 1}`)},
		"conf/app.jsonc":      {Data: []byte(`{"$extends": "../defaults/base.jsonc", "b": 2}`)},
	}
	src, _ := fsys.ReadFile("conf/app.jsonc")
	d, err := jsonc.Parse(src, jsonc.WithFileName("conf/app.jsonc"), jsonc.WithPlugin(extends.NewFS(fsys)))
	if err != nil {
		t.Fatal(err)
	}
	x, err := d.Extended()
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := x.Decode(&got); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, map[string]any{"a": 1.0, "b": 2.0}) {
		t.Errorf("got %v", got)
	}
}

func Example() {
	dir, _ := os.MkdirTemp("", "extends-example")
	defer func() { _ = os.RemoveAll(dir) }()
	_ = os.WriteFile(filepath.Join(dir, "base.jsonc"), []byte(`{"port": 80, "host": "localhost"}`), 0o600)
	_ = os.WriteFile(filepath.Join(dir, "app.jsonc"), []byte(`{
  "$extends": "./base.jsonc", // shared defaults
  "port": 8080
}`), 0o600)

	doc, err := jsonc.Open(filepath.Join(dir, "app.jsonc"), jsonc.WithPlugin(extends.New(extends.WithRoot(dir))))
	if err != nil {
		panic(err)
	}
	cfg, err := doc.Extended()
	if err != nil {
		panic(err)
	}
	port, _ := cfg.GetInt("port")
	host, _ := cfg.GetString("host")
	fmt.Println(host, port)
	// Output: localhost 8080
}
