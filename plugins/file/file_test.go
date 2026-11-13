package fileplugin

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"

	"github.com/ubgo/textdata/plugin"
)

const outsideSecret = "OUTSIDE-ROOT-CONTENT"

// layout builds:
//
//	tmp/outside.txt            (outside the root)
//	tmp/root/top.txt
//	tmp/root/app/key.pem       (base directory = tmp/root/app)
//	tmp/root/app/link-in   ->  ../top.txt        (stays inside)
//	tmp/root/app/link-out  ->  ../../outside.txt (escapes)
//	tmp/root/app/dir-out   ->  ../..             (escapes, as a directory)
func layout(t testing.TB) (tmp, root, base string) {
	t.Helper()
	tmp = t.TempDir()
	root = filepath.Join(tmp, "root")
	base = filepath.Join(root, "app")
	must(t, os.MkdirAll(base, 0o755))
	write(t, filepath.Join(tmp, "outside.txt"), outsideSecret+"\n")
	write(t, filepath.Join(root, "top.txt"), "top\n")
	write(t, filepath.Join(base, "key.pem"), "-----KEY-----\nline2\n")
	must(t, os.Symlink("../top.txt", filepath.Join(base, "link-in")))
	must(t, os.Symlink("../../outside.txt", filepath.Join(base, "link-out")))
	must(t, os.Symlink("../..", filepath.Join(base, "dir-out")))
	return tmp, root, base
}

func must(t testing.TB, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func write(t testing.TB, path, s string) { t.Helper(); must(t, os.WriteFile(path, []byte(s), 0o600)) }

func TestRelativeToBase(t *testing.T) {
	t.Parallel()
	tmp, _, base := layout(t)
	p := New(base)
	v, err := p.Resolve("file", "./key.pem")
	if err != nil || v != "-----KEY-----\nline2" {
		t.Fatalf("Resolve = %q, %v", v, err)
	}
	if v, _ := p.Resolve("FILE", "key.pem"); v != "-----KEY-----\nline2" {
		t.Error("upper-case scheme")
	}
	// Default: confined to the base directory.
	for _, ref := range []string{"../top.txt", "../../outside.txt", filepath.Join(tmp, "outside.txt"), filepath.Join(base, "key.pem"), "link-in", "link-out", "dir-out/outside.txt"} {
		if v, err := p.Read(ref); !errors.Is(err, plugin.ErrDenied) || v != "" {
			t.Errorf("default Read(%q) = %q, %v; want ErrDenied", ref, v, err)
		}
	}
	// Unconfined: absolute paths, climbing out and symlinks out are allowed.
	u := New(base, Unconfined())
	for ref, want := range map[string]string{
		filepath.Join(tmp, "outside.txt"): outsideSecret,
		"../../outside.txt":               outsideSecret,
		"link-out":                        outsideSecret,
		"dir-out/outside.txt":             outsideSecret,
		"../top.txt":                      "top",
	} {
		if v, err := u.Read(ref); err != nil || v != want {
			t.Errorf("unconfined Read(%q) = %q, %v", ref, v, err)
		}
	}
	if _, err := p.Resolve("gopass", "x"); !errors.Is(err, plugin.ErrUnsupportedScheme) {
		t.Errorf("other scheme = %v", err)
	}
	if p.Name() != "file" || len(p.Schemes()) != 1 || p.Schemes()[0] != "file" {
		t.Error("Name/Schemes")
	}
}

// TestBaseFixedAtConstruction: a later chdir does not move relative refs.
func TestBaseFixedAtConstruction(t *testing.T) {
	_, _, base := layout(t)
	t.Chdir(base)
	p := New("")
	t.Chdir(t.TempDir())
	if v, err := p.Read("key.pem"); err != nil || !strings.HasPrefix(v, "-----KEY") {
		t.Errorf("Read after chdir = %q, %v", v, err)
	}
}

// TestOptionOrder: the last of WithRoot and Unconfined wins; an empty WithRoot
// keeps the default root.
func TestOptionOrder(t *testing.T) {
	t.Parallel()
	_, root, base := layout(t)
	if _, err := New(base, Unconfined(), WithRoot(root)).Read("link-out"); !errors.Is(err, plugin.ErrDenied) {
		t.Errorf("Unconfined then WithRoot = %v", err)
	}
	if v, err := New(base, WithRoot(root), Unconfined()).Read("link-out"); err != nil || v != outsideSecret {
		t.Errorf("WithRoot then Unconfined = %q, %v", v, err)
	}
	if _, err := New(base, WithRoot("")).Read("../top.txt"); !errors.Is(err, plugin.ErrDenied) {
		t.Errorf("empty WithRoot = %v", err)
	}
	if _, err := New(filepath.Join(base, "missing")).Read("x"); err == nil {
		t.Error("missing base directory must fail when confined")
	}
}

func TestConfinement(t *testing.T) {
	t.Parallel()
	tmp, root, base := layout(t)
	p := New(base, WithRoot(root))
	allowed := map[string]string{
		"key.pem":          "-----KEY-----\nline2",
		"../top.txt":       "top",
		"link-in":          "top",
		"../app/key.pem":   "-----KEY-----\nline2",
		"./sub/../key.pem": "-----KEY-----\nline2",
	}
	for ref, want := range allowed {
		if v, err := p.Read(ref); err != nil || v != want {
			t.Errorf("Read(%q) = %q, %v", ref, v, err)
		}
	}
	denied := []string{
		"../../outside.txt",
		"../../root/../outside.txt",
		filepath.Join(tmp, "outside.txt"),
		filepath.Join(base, "key.pem"), // absolute refused even inside the root
		"/etc/passwd",
		"link-out",
		"dir-out/outside.txt",
	}
	for _, ref := range denied {
		v, err := p.Read(ref)
		if !errors.Is(err, plugin.ErrDenied) {
			t.Errorf("Read(%q) = %q, %v; want ErrDenied", ref, v, err)
		}
		if strings.Contains(v, outsideSecret) || (err != nil && strings.Contains(err.Error(), outsideSecret)) {
			t.Errorf("Read(%q) leaked outside content", ref)
		}
	}
	// A relative root resolves against the base directory.
	if v, err := New(base, WithRoot("..")).Read("../top.txt"); err != nil || v != "top" {
		t.Errorf("relative root = %q, %v", v, err)
	}
	if _, err := New(base, WithRoot(".")).Read("../top.txt"); !errors.Is(err, plugin.ErrDenied) {
		t.Errorf("root = base must refuse ../ : %v", err)
	}
}

// TestSymlinkedRoot: a root reached through a symlink still contains its own
// files (the macOS /tmp case).
func TestSymlinkedRoot(t *testing.T) {
	t.Parallel()
	tmp, root, _ := layout(t)
	alias := filepath.Join(tmp, "alias")
	must(t, os.Symlink(root, alias))
	p := New(filepath.Join(alias, "app"), WithRoot(alias))
	if v, err := p.Read("key.pem"); err != nil || !strings.HasPrefix(v, "-----KEY") {
		t.Errorf("Read via symlinked root = %q, %v", v, err)
	}
	if _, err := p.Read("link-out"); !errors.Is(err, plugin.ErrDenied) {
		t.Errorf("link-out via symlinked root = %v", err)
	}
}

func TestErrors(t *testing.T) {
	t.Parallel()
	_, root, base := layout(t)
	for _, p := range []*Plugin{New(base, Unconfined()), New(base), New(base, WithRoot(root))} {
		if _, err := p.Read("missing.txt"); !errors.Is(err, plugin.ErrNotFound) {
			t.Errorf("missing = %v", err)
		}
		if _, err := p.Read(""); err == nil {
			t.Error("empty ref accepted")
		}
		if _, err := p.Read("a\x00b"); err == nil {
			t.Error("NUL ref accepted")
		}
		if _, err := p.Read("."); !errors.Is(err, ErrNotRegular) {
			t.Errorf("directory = %v", err)
		}
	}
	if _, err := New(base, WithRoot(filepath.Join(root, "nope"))).Read("key.pem"); err == nil || !errors.Is(err, os.ErrNotExist) {
		t.Errorf("missing root = %v", err)
	}
}

func TestPermission(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("permission bits not enforced")
	}
	_, root, base := layout(t)
	locked := filepath.Join(base, "locked")
	write(t, locked, "x")
	must(t, os.Chmod(locked, 0o000))
	for _, p := range []*Plugin{New(base, Unconfined()), New(base), New(base, WithRoot(root))} {
		if _, err := p.Read("locked"); !errors.Is(err, plugin.ErrDenied) {
			t.Errorf("unreadable = %v", err)
		}
	}
}

func TestFIFORefused(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("no FIFOs")
	}
	_, root, base := layout(t)
	must(t, syscall.Mkfifo(filepath.Join(base, "pipe"), 0o600))
	for _, p := range []*Plugin{New(base, Unconfined()), New(base), New(base, WithRoot(root))} {
		if _, err := p.Read("pipe"); !errors.Is(err, ErrNotRegular) {
			t.Errorf("fifo = %v", err)
		}
	}
}

func TestMaxSize(t *testing.T) {
	t.Parallel()
	_, root, base := layout(t)
	write(t, filepath.Join(base, "ten"), "0123456789")
	for _, p := range []*Plugin{New(base, WithMaxSize(10), Unconfined()), New(base, WithMaxSize(10), WithRoot(root))} {
		if v, err := p.Read("ten"); err != nil || v != "0123456789" {
			t.Errorf("at limit = %q, %v", v, err)
		}
	}
	p := New(base, WithMaxSize(9))
	if v, err := p.Read("ten"); !errors.Is(err, ErrTooLarge) || v != "" {
		t.Errorf("over limit = %q, %v", v, err)
	}
	if New(base, WithMaxSize(0)).maxSize != DefaultMaxSize || New(base, WithMaxSize(-1)).maxSize != DefaultMaxSize {
		t.Error("non-positive size must keep the default")
	}
	big := filepath.Join(base, "big")
	must(t, os.WriteFile(big, make([]byte, DefaultMaxSize+1), 0o600))
	if _, err := New(base).Read("big"); !errors.Is(err, ErrTooLarge) {
		t.Errorf("default limit = %v", err)
	}
}

func TestTrimNewline(t *testing.T) {
	t.Parallel()
	_, _, base := layout(t)
	cases := map[string][2]string{ // contents -> {trimmed, raw}
		"a\n":   {"a", "a\n"},
		"a\r\n": {"a", "a\r\n"},
		"a\n\n": {"a\n", "a\n\n"},
		"a":     {"a", "a"},
		"":      {"", ""},
		"\n":    {"", "\n"},
	}
	i := 0
	for contents, want := range cases {
		name := filepath.Join(base, "t"+string(rune('a'+i)))
		i++
		write(t, name, contents)
		if v, _ := New(base).Read(filepath.Base(name)); v != want[0] {
			t.Errorf("trim %q = %q, want %q", contents, v, want[0])
		}
		if v, _ := New(base, WithTrimNewline(false)).Read(filepath.Base(name)); v != want[1] {
			t.Errorf("raw %q = %q, want %q", contents, v, want[1])
		}
	}
}

// FuzzConfinedRef: for any ref, a confined plugin never panics, never returns
// the content of a file outside the root, and either succeeds with a file that
// really lies inside the root or fails.
func FuzzConfinedRef(f *testing.F) {
	for _, s := range []string{"key.pem", "../top.txt", "../../outside.txt", "link-out", "dir-out/outside.txt", "/etc/passwd", "", ".", "..", "a\x00", "link-in", `..\..\outside.txt`, "./././key.pem", "dir-out/root/top.txt"} {
		f.Add(s)
	}
	tmp, root, base := layout(f)
	p := New(base, WithRoot(root), WithMaxSize(1<<10))
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		f.Fatal(err)
	}
	_ = tmp
	def := New(base, WithMaxSize(1<<10))
	realBase, err := filepath.EvalSymlinks(base)
	if err != nil {
		f.Fatal(err)
	}
	f.Fuzz(func(t *testing.T, ref string) {
		// Default plugin: confined to the base directory.
		if v, err := def.Read(ref); err == nil {
			real, rerr := filepath.EvalSymlinks(filepath.Join(base, ref))
			if _, ok := within(realBase, real); rerr != nil || !ok || filepath.IsAbs(ref) {
				t.Fatalf("default: ref %q succeeded outside the base: %s", ref, real)
			}
		} else if strings.Contains(v, outsideSecret) {
			t.Fatalf("default: ref %q read outside", ref)
		}
		v, err := p.Read(ref)
		if strings.Contains(v, outsideSecret) {
			t.Fatalf("ref %q read outside the root", ref)
		}
		if err == nil {
			real, rerr := filepath.EvalSymlinks(filepath.Join(base, ref))
			if rerr != nil {
				t.Fatalf("ref %q succeeded but does not resolve: %v", ref, rerr)
			}
			if _, ok := within(realRoot, real); !ok || filepath.IsAbs(ref) {
				t.Fatalf("ref %q succeeded outside the root: %s", ref, real)
			}
		}
	})
}
