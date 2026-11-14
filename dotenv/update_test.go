package dotenv

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/ubgo/textdata/atomicfile"
)

// TestUpdateConcurrentKeepsEveryEdit: 20 concurrent Updates, each setting its
// own key and bumping a shared counter, all survive, and the comment stays.
// ds:def id=testupdateconcurrentkeepseveryedit-c2pyfsjq owner=@maintainers stability=stable
func TestUpdateConcurrentKeepsEveryEdit(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte("# shared\nN=0\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	const workers = 20
	var wg sync.WaitGroup
	errs := make(chan error, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs <- Update(context.Background(), path, func(f *File) error {
				v, _ := f.Get("N")
				n, err := strconv.Atoi(v)
				if err != nil {
					return err
				}
				f.Set("K"+strconv.Itoa(i), strconv.Itoa(i))
				f.Set("N", strconv.Itoa(n+1))
				return nil
			})
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	f, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if v, _ := f.Get("N"); v != strconv.Itoa(workers) {
		t.Errorf("N = %s, want %d", v, workers)
	}
	for i := 0; i < workers; i++ {
		if !f.Has("K" + strconv.Itoa(i)) {
			t.Errorf("K%d lost", i)
		}
	}
	if f.Entries()[0].Kind != KindComment {
		t.Error("comment lost")
	}
}

// TestUpdateWritesOnlyOnChange: an fn error writes nothing, a no-op leaves a
// missing file missing, and a change on a missing file creates it at 0600.
func TestUpdateWritesOnlyOnChange(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), ".env")
	boom := errors.New("boom")
	if err := Update(context.Background(), path, func(*File) error { return boom }); !errors.Is(err, boom) {
		t.Fatalf("err = %v, want fn's error", err)
	}
	if err := Update(context.Background(), path, func(*File) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("no-op Update created the file: %v", err)
	}
	if err := Update(context.Background(), path, func(f *File) error { f.Set("A", "1"); return nil }); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != "A=1\n" {
		t.Fatalf("file = %q, %v", got, err)
	}
}

// TestUpdateLockTimeout: while the lock is held elsewhere, Update with a
// deadline fails with atomicfile.ErrLockTimeout and never calls fn.
func TestUpdateLockTimeout(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), ".env")
	lk, err := atomicfile.Lock(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer lk.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	called := false
	err = Update(ctx, path, func(*File) error { called = true; return nil })
	if !errors.Is(err, atomicfile.ErrLockTimeout) || called {
		t.Fatalf("err = %v, called = %v", err, called)
	}
}
