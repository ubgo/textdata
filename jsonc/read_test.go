package jsonc

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/ubgo/textdata/pos"
)

const sample = `{
  // Sync settings
  "sync": {
    "remote": {"url": "git@x:y.git", "branch": "main"},
    "interval": 30, // seconds
    "ratio": 0.5,
    "big": 9007199254740993,
    "enabled": true,
    "off": null
  },
  "a.b": {"c": [10, 20, {"deep": "yes"}]},
  "list": [],
  "dup": 1,
  "dup": 2,
}
`

func TestGetters(t *testing.T) {
	t.Parallel()
	d := mustParse(t, sample)

	if s, err := d.GetString("sync.remote.url"); err != nil || s != "git@x:y.git" {
		t.Errorf("GetString = %q, %v", s, err)
	}
	if n, err := d.GetInt("sync.interval"); err != nil || n != 30 {
		t.Errorf("GetInt = %d, %v", n, err)
	}
	if n, err := d.GetInt("sync.big"); err != nil || n != 9007199254740993 {
		t.Errorf("GetInt keeps precision beyond float64: %d, %v", n, err)
	}
	if f, err := d.GetFloat("sync.ratio"); err != nil || f != 0.5 {
		t.Errorf("GetFloat = %v, %v", f, err)
	}
	if b, err := d.GetBool("sync.enabled"); err != nil || !b {
		t.Errorf("GetBool = %v, %v", b, err)
	}
	if v, err := d.Get(`"a.b".c[2].deep`); err != nil || v != "yes" {
		t.Errorf("quoted path = %v, %v", v, err)
	}
	if v, err := d.Get("/a.b/c/1"); err != nil || v != float64(20) {
		t.Errorf("JSON Pointer = %v, %v", v, err)
	}
	if v, err := d.Get("dup"); err != nil || v != float64(2) {
		t.Errorf("duplicate: last wins = %v, %v", v, err)
	}
	if v, err := d.Get("sync.off"); err != nil || v != nil {
		t.Errorf("null = %v, %v", v, err)
	}
	if v, err := d.Get("sync.remote"); err != nil || !reflect.DeepEqual(v, map[string]any{"url": "git@x:y.git", "branch": "main"}) {
		t.Errorf("object = %v, %v", v, err)
	}
	if root, err := d.Get(""); err != nil || len(root.(map[string]any)) != 4 {
		t.Errorf("root = %v, %v", root, err)
	}
	if k, _ := d.Kind("list"); k != KindArray {
		t.Errorf("Kind = %v", k)
	}
	if keys, err := d.Keys(""); err != nil || !reflect.DeepEqual(keys, []string{"sync", "a.b", "list", "dup"}) {
		t.Errorf("Keys = %v, %v", keys, err)
	}
	if n, _ := d.Len(`"a.b".c`); n != 3 {
		t.Errorf("Len array = %d", n)
	}
	if n, _ := d.Len(""); n != 4 {
		t.Errorf("Len object (distinct keys) = %d", n)
	}
	if !d.Has("sync.remote.branch") || d.Has("sync.nope") || d.Has("list[0]") || d.Has("a..b") {
		t.Error("Has")
	}
	if c, _ := d.Comment("sync"); c != "Sync settings" {
		t.Errorf("leading comment = %q", c)
	}
	if c, _ := d.Comment("sync.interval"); c != "seconds" {
		t.Errorf("trailing comment = %q", c)
	}
	if c, _ := d.Comment("list"); c != "" {
		t.Errorf("no comment = %q", c)
	}
}

func TestGetterErrors(t *testing.T) {
	t.Parallel()
	d := mustParse(t, sample, WithFileName("cfg.jsonc"))
	_, err := d.GetInt("sync.ratio")
	var pe *pos.Error
	if !errors.Is(err, ErrType) || !errors.As(err, &pe) || err.Error() != `cfg.jsonc:6:14: "sync.ratio" is a number, not an integer` {
		t.Errorf("GetInt(fraction) = %v", err)
	}
	if _, err := d.GetString("sync.interval"); err == nil || !strings.HasSuffix(err.Error(), `"sync.interval" is a number, not a string`) {
		t.Errorf("GetString(number) = %v", err)
	}
	if _, err := d.GetBool("nope"); !errors.Is(err, ErrNotFound) || !strings.Contains(err.Error(), `"nope"`) {
		t.Errorf("missing = %v", err)
	}
	if _, err := d.Get("sync.remote.url.x"); !errors.Is(err, ErrNotFound) {
		t.Errorf("into a string = %v", err)
	}
	if _, err := d.Get("a[0"); err == nil {
		t.Error("bad path accepted")
	}
	if _, err := d.Keys("list"); !errors.Is(err, ErrType) {
		t.Errorf("Keys(array) = %v", err)
	}
	if _, err := d.Len("dup"); !errors.Is(err, ErrType) {
		t.Errorf("Len(number) = %v", err)
	}
	if _, err := d.GetFloat("sync.url"); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetFloat missing = %v", err)
	}
	if _, err := mustParse(t, "[1e400]").GetInt("[0]"); !errors.Is(err, ErrType) {
		t.Errorf("GetInt overflow = %v", err)
	}
	if n, err := mustParse(t, "[1e3]").GetInt("[0]"); err != nil || n != 1000 {
		t.Errorf("GetInt(1e3) = %d, %v", n, err)
	}
	if _, err := mustParse(t, "[1e400]").GetFloat("[0]"); !errors.Is(err, ErrType) {
		t.Errorf("GetFloat overflow = %v", err)
	}
}

func TestJSONAndDecode(t *testing.T) {
	t.Parallel()
	d := mustParse(t, sample)
	j, err := d.JSON()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(j), "//") && !strings.Contains(string(j), "git@") {
		t.Errorf("comment left in JSON: %s", j)
	}
	if strings.Contains(string(j), "Sync settings") || strings.Contains(string(j), "seconds") {
		t.Errorf("comment left in JSON: %s", j)
	}
	var cfg struct {
		Sync struct {
			Remote   struct{ URL string }
			Interval int
		}
		Dup int
	}
	if err := d.Decode(&cfg); err != nil || cfg.Sync.Remote.URL != "git@x:y.git" || cfg.Sync.Interval != 30 || cfg.Dup != 2 {
		t.Errorf("Decode = %+v, %v", cfg, err)
	}
	if err := d.Decode(&[]int{}); err == nil {
		t.Error("Decode into the wrong type succeeded")
	}
	empty := mustParse(t, "// nothing yet\n")
	if j, err := empty.JSON(); err != nil || string(j) != "null" {
		t.Errorf("empty JSON = %q, %v", j, err)
	}
	if _, err := empty.Get("a"); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get on empty = %v", err)
	}
}

func TestOpenReadAndDocument(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	p := filepath.Join(dir, "c.jsonc")
	if err := os.WriteFile(p, []byte(sample), 0o640); err != nil {
		t.Fatal(err)
	}
	m, err := Read(p)
	if err != nil || m["dup"] != float64(2) {
		t.Errorf("Read = %v, %v", m, err)
	}
	d, err := Open(p)
	if err != nil || !d.Existed() || d.Path() != p || d.Mode() != 0o640 {
		t.Fatalf("Open = %v %v", d, err)
	}
	if d.FormatName() != "jsonc" || d.Location() != p || string(d.Bytes()) != sample {
		t.Error("Document view")
	}
	paths := d.ValuePaths()
	want := []string{"sync.remote.url", "sync.remote.branch", "sync.interval", "sync.ratio", "sync.big", "sync.enabled", "sync.off", `"a.b".c[0]`, `"a.b".c[1]`, `"a.b".c[2].deep`, "list", "dup", "dup"}
	if !reflect.DeepEqual(paths, want) {
		t.Errorf("ValuePaths =\n%q\nwant\n%q", paths, want)
	}
	if v, ok := d.ValueAt("sync.interval"); !ok || v != "30" {
		t.Errorf("ValueAt number = %q", v)
	}
	if v, ok := d.ValueAt("sync.remote.url"); !ok || v != "git@x:y.git" {
		t.Errorf("ValueAt string = %q", v)
	}
	if _, ok := d.ValueAt("zz"); ok {
		t.Error("ValueAt missing")
	}

	missing, err := Open(filepath.Join(dir, "none.jsonc"))
	if err != nil || missing.Existed() || missing.Mode() != defaultFileMode {
		t.Errorf("Open missing = %v, %v", missing, err)
	}
	if _, err := Open(dir); err == nil {
		t.Error("Open(dir) succeeded")
	}
	bad := filepath.Join(dir, "bad.jsonc")
	_ = os.WriteFile(bad, []byte("{\n  x\n}"), 0o600)
	if _, err := Read(bad); err == nil || !strings.HasPrefix(err.Error(), bad+":2:3:") {
		t.Errorf("Read(bad) = %v", err)
	}
	if _, err := ParseReader(errReader{}); err == nil {
		t.Error("ParseReader error lost")
	}
	if d2, err := ParseReader(strings.NewReader("[1]")); err != nil || !d2.Has("[0]") {
		t.Errorf("ParseReader = %v", err)
	}
}

type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errors.New("boom") }
