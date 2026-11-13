package atomicfile

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func read(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func perm(t *testing.T, p string) fs.FileMode {
	t.Helper()
	info, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	return info.Mode().Perm()
}

func noTemp(t *testing.T, dir, suffix string) {
	t.Helper()
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), suffix) {
			t.Errorf("temp file left behind: %s", e.Name())
		}
	}
}

func TestWriteNewAndReplace(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	p := filepath.Join(dir, "a.env")
	if err := Write(p, []byte("one"), Options{}); err != nil {
		t.Fatal(err)
	}
	if read(t, p) != "one" {
		t.Fatal("content")
	}
	if runtime.GOOS != "windows" && perm(t, p) != DefaultMode {
		t.Errorf("new file mode = %v, want %v", perm(t, p), DefaultMode)
	}
	if err := Write(p, []byte("two"), Options{Mode: 0o640}); err != nil {
		t.Fatal(err)
	}
	if read(t, p) != "two" {
		t.Fatal("replace content")
	}
	if runtime.GOOS != "windows" && perm(t, p) != 0o640 {
		t.Errorf("mode = %v, want 0640", perm(t, p))
	}
	noTemp(t, dir, DefaultTempSuffix)
}

func TestWriteKeepMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission bits are not POSIX on windows")
	}
	t.Parallel()
	dir := t.TempDir()
	p := filepath.Join(dir, "locked")
	if err := os.WriteFile(p, []byte("x"), 0o400); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(p, 0o400); err != nil {
		t.Fatal(err)
	}
	if err := Write(p, []byte("y"), Options{Mode: 0o666, KeepMode: true}); err != nil {
		t.Fatal(err)
	}
	if perm(t, p) != 0o400 {
		t.Errorf("KeepMode widened the file to %v", perm(t, p))
	}
	// KeepMode on a missing file falls back to Mode.
	q := filepath.Join(dir, "new")
	if err := Write(q, []byte("z"), Options{Mode: 0o644, KeepMode: true}); err != nil {
		t.Fatal(err)
	}
	if perm(t, q) != 0o644 {
		t.Errorf("new file mode = %v, want 0644", perm(t, q))
	}
}

func TestWriteThroughSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on windows")
	}
	t.Parallel()
	dir := t.TempDir()
	real := filepath.Join(dir, "real", "config.jsonc")
	if err := os.MkdirAll(filepath.Dir(real), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(real, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link.jsonc")
	if err := os.Symlink("real/config.jsonc", link); err != nil {
		t.Fatal(err)
	}
	link2 := filepath.Join(dir, "link2.jsonc")
	if err := os.Symlink(link, link2); err != nil {
		t.Fatal(err)
	}
	if err := Write(link2, []byte("new"), Options{}); err != nil {
		t.Fatal(err)
	}
	if read(t, real) != "new" {
		t.Error("real file not updated")
	}
	for _, l := range []string{link, link2} {
		info, err := os.Lstat(l)
		if err != nil || info.Mode()&fs.ModeSymlink == 0 {
			t.Errorf("%s is no longer a symlink", l)
		}
	}
	noTemp(t, filepath.Dir(real), DefaultTempSuffix)

	// NoFollow replaces the link itself.
	if err := Write(link, []byte("own"), Options{NoFollow: true}); err != nil {
		t.Fatal(err)
	}
	if info, _ := os.Lstat(link); info.Mode()&fs.ModeSymlink != 0 {
		t.Error("NoFollow kept the symlink")
	}
	if read(t, real) != "new" {
		t.Error("NoFollow wrote through the link")
	}
}

func TestResolve(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on windows")
	}
	t.Parallel()
	dir := t.TempDir()
	dangling := filepath.Join(dir, "d")
	if err := os.Symlink("missing-target", dangling); err != nil {
		t.Fatal(err)
	}
	got, err := Resolve(dangling)
	if err != nil || got != filepath.Join(dir, "missing-target") {
		t.Errorf("Resolve(dangling) = %q, %v", got, err)
	}
	if err := Write(dangling, []byte("made"), Options{}); err != nil {
		t.Fatal(err)
	}
	if read(t, filepath.Join(dir, "missing-target")) != "made" {
		t.Error("dangling link target not created")
	}

	a, b := filepath.Join(dir, "a"), filepath.Join(dir, "b")
	_ = os.Symlink(b, a)
	_ = os.Symlink(a, b)
	if _, err := Resolve(a); err == nil || !strings.Contains(err.Error(), "too many levels") {
		t.Errorf("Resolve(loop) = %v", err)
	}
	if err := Write(a, []byte("x"), Options{}); err == nil {
		t.Error("Write through a loop succeeded")
	}
	abs := filepath.Join(dir, "abs")
	_ = os.Symlink(filepath.Join(dir, "missing-target"), abs)
	if got, _ := Resolve(abs); got != filepath.Join(dir, "missing-target") {
		t.Errorf("absolute link resolved to %q", got)
	}
}

func TestWriteFailures(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	p := filepath.Join(dir, "f")
	if err := os.WriteFile(p, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	boom := errors.New("boom")

	err := Write(p, []byte("x"), Options{TempSuffix: ".t", Chmod: func(string, fs.FileMode) error { return boom }})
	if !errors.Is(err, boom) || !strings.Contains(err.Error(), "set mode") {
		t.Errorf("chmod failure = %v", err)
	}
	err = Write(p, []byte("x"), Options{TempSuffix: ".t", Rename: func(string, string) error { return boom }})
	if !errors.Is(err, boom) || !strings.Contains(err.Error(), "rename") {
		t.Errorf("rename failure = %v", err)
	}
	if read(t, p) != "keep" {
		t.Error("a failed write changed the target")
	}
	noTemp(t, dir, ".t")

	// The temp file cannot be created in a missing directory.
	if err := Write(filepath.Join(dir, "no", "such", "f"), []byte("x"), Options{}); err == nil || !strings.Contains(err.Error(), "write temp file") {
		t.Errorf("missing dir = %v", err)
	}
}

func TestWriteStaleTempIsReplaced(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	p := filepath.Join(dir, "f")
	if err := os.WriteFile(p+DefaultTempSuffix, []byte("stale stale stale"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Write(p, []byte("fresh"), Options{}); err != nil {
		t.Fatal(err)
	}
	if read(t, p) != "fresh" {
		t.Errorf("content = %q", read(t, p))
	}
}
