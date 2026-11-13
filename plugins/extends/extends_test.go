package extends

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/ubgo/textdata/plugin"
)

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestDiskRelativeToIncludingFile(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "base.jsonc"), `{"a":1}`)
	write(t, filepath.Join(dir, "app", "config.jsonc"), `{}`)

	data, loc, err := New().Include(filepath.Join(dir, "app", "config.jsonc"), "../base.jsonc")
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != `{"a":1}` {
		t.Errorf("data = %q", data)
	}
	if loc != filepath.Join(dir, "base.jsonc") {
		t.Errorf("loc = %q, want cleaned %q", loc, filepath.Join(dir, "base.jsonc"))
	}
}

func TestDiskEmptyFromIsWorkingDirectory(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "base.env"), "A=1\n")
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(wd) })
	data, loc, err := New().Include("", "base.env")
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "A=1\n" || loc != "base.env" {
		t.Errorf("got %q at %q", data, loc)
	}
}

// One includer serves every format: it hands back bytes and a location and
// knows nothing about what they contain.
func TestDiskCrossFormat(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "base.env"), "A=1\n")
	write(t, filepath.Join(dir, "base.jsonc"), "{\"a\": 1 // one\n}")
	inc := New(WithRoot(dir))
	from := filepath.Join(dir, "x")
	for ref, want := range map[string]string{"base.env": "A=1\n", "base.jsonc": "{\"a\": 1 // one\n}"} {
		data, _, err := inc.Include(from, ref)
		if err != nil || string(data) != want {
			t.Errorf("%s: %q, %v", ref, data, err)
		}
	}
}

func TestDiskMissing(t *testing.T) {
	_, _, err := New().Include(filepath.Join(t.TempDir(), "a.jsonc"), "nope.jsonc")
	if !errors.Is(err, plugin.ErrNotFound) || !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("err = %v, want ErrNotFound and fs.ErrNotExist", err)
	}
}

func TestDiskEmptyAndNULRef(t *testing.T) {
	for _, ref := range []string{"", "a\x00b"} {
		if _, _, err := New().Include("x", ref); err == nil {
			t.Errorf("%q: no error", ref)
		}
	}
}

func TestDiskAbsolute(t *testing.T) {
	dir := t.TempDir()
	abs := filepath.Join(dir, "base.jsonc")
	write(t, abs, "{}")
	if _, loc, err := New().Include("", abs); err != nil || loc != abs {
		t.Fatalf("allowed by default: %q %v", loc, err)
	}
	_, _, err := New(AllowAbsolute(false)).Include("", abs)
	if !errors.Is(err, plugin.ErrDenied) {
		t.Fatalf("err = %v, want ErrDenied", err)
	}
}

func TestDiskRootConfinement(t *testing.T) {
	top := t.TempDir()
	root := filepath.Join(top, "root")
	write(t, filepath.Join(root, "conf", "a.jsonc"), "{}")
	write(t, filepath.Join(root, "base.jsonc"), "{}")
	write(t, filepath.Join(top, "secret.jsonc"), `{"s":1}`)
	inc := New(WithRoot(root))
	from := filepath.Join(root, "conf", "a.jsonc")

	if _, _, err := inc.Include(from, "../base.jsonc"); err != nil {
		t.Errorf("inside root: %v", err)
	}
	if _, _, err := inc.Include(from, "../../secret.jsonc"); !errors.Is(err, plugin.ErrDenied) {
		t.Errorf("dotdot escape: %v, want ErrDenied", err)
	}
	if _, _, err := inc.Include(from, filepath.Join(top, "secret.jsonc")); !errors.Is(err, plugin.ErrDenied) {
		t.Errorf("absolute escape: %v, want ErrDenied", err)
	}
	if _, _, err := inc.Include(from, "../../missing.jsonc"); !errors.Is(err, plugin.ErrDenied) {
		t.Errorf("missing escape: %v, want ErrDenied", err)
	}
	if _, _, err := inc.Include(from, "missing.jsonc"); !errors.Is(err, plugin.ErrNotFound) {
		t.Errorf("missing inside: %v, want ErrNotFound", err)
	}
}

func TestDiskRootConfinementSymlinks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on windows")
	}
	top := t.TempDir()
	root := filepath.Join(top, "root")
	write(t, filepath.Join(root, "real.jsonc"), `{"ok":1}`)
	write(t, filepath.Join(top, "secret.jsonc"), `{"s":1}`)
	if err := os.Symlink(filepath.Join(top, "secret.jsonc"), filepath.Join(root, "evil.jsonc")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(top, filepath.Join(root, "up")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "real.jsonc"), filepath.Join(root, "good.jsonc")); err != nil {
		t.Fatal(err)
	}
	inc := New(WithRoot(root))
	from := filepath.Join(root, "a.jsonc")
	for _, ref := range []string{"evil.jsonc", "up/secret.jsonc"} {
		if _, _, err := inc.Include(from, ref); !errors.Is(err, plugin.ErrDenied) {
			t.Errorf("%s: %v, want ErrDenied", ref, err)
		}
	}
	if data, _, err := inc.Include(from, "good.jsonc"); err != nil || string(data) != `{"ok":1}` {
		t.Errorf("link inside root: %q %v", data, err)
	}

	// A root reached through a symlink still accepts its own files.
	link := filepath.Join(top, "rootlink")
	if err := os.Symlink(root, link); err != nil {
		t.Fatal(err)
	}
	if _, _, err := New(WithRoot(link)).Include(filepath.Join(link, "a.jsonc"), "real.jsonc"); err != nil {
		t.Errorf("root via symlink: %v", err)
	}
	if _, _, err := New(WithRoot(link)).Include(filepath.Join(link, "a.jsonc"), "nope.jsonc"); !errors.Is(err, plugin.ErrNotFound) {
		t.Errorf("missing via symlinked root: %v, want ErrNotFound", err)
	}
}

func TestDiskSizeLimit(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "big.jsonc"), strings.Repeat("x", 11))
	write(t, filepath.Join(dir, "ok.jsonc"), strings.Repeat("x", 10))
	inc := New(WithMaxSize(10))
	if _, _, err := inc.Include(filepath.Join(dir, "a"), "big.jsonc"); !errors.Is(err, ErrTooLarge) {
		t.Errorf("err = %v, want ErrTooLarge", err)
	}
	if _, _, err := inc.Include(filepath.Join(dir, "a"), "ok.jsonc"); err != nil {
		t.Errorf("at the limit: %v", err)
	}
	if New(WithMaxSize(-1)).cfg.maxSize != DefaultMaxSize {
		t.Error("WithMaxSize(-1) should restore the default")
	}
}

func TestDiskNotRegular(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, _, err := New().Include(filepath.Join(dir, "a"), "sub"); !errors.Is(err, ErrNotRegular) {
		t.Errorf("err = %v, want ErrNotRegular", err)
	}
}

func TestNames(t *testing.T) {
	if New().Name() != Name || NewFS(fstest.MapFS{}).Name() != FSName {
		t.Error("names")
	}
	var r plugin.Registry
	r.Add(New())
	if got := r.Info()[0].String(); got != "extends: Includer" {
		t.Errorf("Info = %q", got)
	}
}

func TestFS(t *testing.T) {
	fsys := fstest.MapFS{
		"base.jsonc":        {Data: []byte(`{"a":1}`)},
		"conf/app.jsonc":    {Data: []byte(`{}`)},
		"conf/shared.jsonc": {Data: []byte(`{"s":1}`)},
		"dir/x":             {Data: []byte("x")},
		"big":               {Data: []byte("0123456789A")},
	}
	inc := NewFS(fsys, WithMaxSize(10))

	data, loc, err := inc.Include("conf/app.jsonc", "../base.jsonc")
	if err != nil || string(data) != `{"a":1}` || loc != "base.jsonc" {
		t.Errorf("relative up: %q %q %v", data, loc, err)
	}
	if _, loc, err := inc.Include("conf/app.jsonc", "shared.jsonc"); err != nil || loc != "conf/shared.jsonc" {
		t.Errorf("sibling: %q %v", loc, err)
	}
	if _, loc, err := inc.Include("", "base.jsonc"); err != nil || loc != "base.jsonc" {
		t.Errorf("from empty: %q %v", loc, err)
	}
	for _, ref := range []string{"/base.jsonc", "../../base.jsonc"} {
		if _, _, err := inc.Include("conf/app.jsonc", ref); !errors.Is(err, plugin.ErrDenied) {
			t.Errorf("%s: %v, want ErrDenied", ref, err)
		}
	}
	if _, _, err := inc.Include("", "missing"); !errors.Is(err, plugin.ErrNotFound) {
		t.Errorf("missing: %v", err)
	}
	if _, _, err := inc.Include("", "dir"); !errors.Is(err, ErrNotRegular) {
		t.Errorf("dir: %v", err)
	}
	if _, _, err := inc.Include("", "big"); !errors.Is(err, ErrTooLarge) {
		t.Errorf("big: %v", err)
	}
	if _, _, err := inc.Include("", ""); err == nil {
		t.Error("empty ref accepted")
	}
}

// FuzzInclude: any reference, from any location, never panics, and a
// confined includer never returns a location outside its root.
func FuzzInclude(f *testing.F) {
	for _, s := range []string{"base.jsonc", "../x", "/etc/passwd", "", "a/../../b", "\x00", "./././a", "sub/../base.jsonc"} {
		f.Add("conf/app.jsonc", s)
	}
	top := f.TempDir()
	root := filepath.Join(top, "root")
	if err := os.MkdirAll(filepath.Join(root, "conf"), 0o755); err != nil {
		f.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "base.jsonc"), []byte("{}"), 0o600); err != nil {
		f.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(top, "outside.jsonc"), []byte("{}"), 0o600); err != nil {
		f.Fatal(err)
	}
	disk := New(WithRoot(root))
	fsys := NewFS(fstest.MapFS{"base.jsonc": {Data: []byte("{}")}})
	f.Fuzz(func(t *testing.T, from, ref string) {
		if _, loc, err := disk.Include(filepath.Join(root, from), ref); err == nil {
			abs, _ := filepath.Abs(loc)
			if !inside(root, abs) {
				t.Fatalf("disk returned %q outside root", loc)
			}
		}
		if _, loc, err := fsys.Include(from, ref); err == nil && !fs.ValidPath(loc) {
			t.Fatalf("fs returned invalid %q", loc)
		}
	})
}

func TestDiskFIFODoesNotBlock(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("no named pipes")
	}
	dir := t.TempDir()
	fifo := filepath.Join(dir, "pipe")
	if err := mkfifo(fifo); err != nil {
		t.Skip("mkfifo:", err)
	}
	for _, inc := range []*Disk{New(), New(WithRoot(dir))} {
		if _, _, err := inc.Include(filepath.Join(dir, "a"), "pipe"); !errors.Is(err, ErrNotRegular) {
			t.Errorf("err = %v, want ErrNotRegular", err)
		}
	}
}
