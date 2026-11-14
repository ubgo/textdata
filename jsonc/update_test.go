package jsonc

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

// TestUpdateConcurrentKeepsEveryEdit: 20 concurrent Updates, each adding its
// own key and bumping a shared counter, all survive. The lock is held per
// Update call, so goroutines exclude each other exactly as processes do
// (the cross-process case is pinned in package atomicfile).
// ds:def id=testupdateconcurrentkeepseveryedit-eugnqggz owner=@maintainers stability=stable
func TestUpdateConcurrentKeepsEveryEdit(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "config.jsonc")
	if err := os.WriteFile(path, []byte("{\n  // shared\n  \"n\": 0\n}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	const workers = 20
	var wg sync.WaitGroup
	errs := make(chan error, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs <- Update(context.Background(), path, func(d *Doc) error {
				n, err := d.GetInt("n")
				if err != nil {
					return err
				}
				if err := d.Set("k"+strconv.Itoa(i), i); err != nil {
					return err
				}
				return d.Set("n", n+1)
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
	d, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if n, _ := d.GetInt("n"); n != workers {
		t.Errorf("n = %d, want %d", n, workers)
	}
	for i := 0; i < workers; i++ {
		if !d.Has("k" + strconv.Itoa(i)) {
			t.Errorf("k%d lost", i)
		}
	}
	if c, _ := d.Comment("n"); c == "" {
		t.Error("comment above n lost")
	}
}

// TestUpdateWritesOnlyOnChange: an fn error writes nothing, a no-op leaves a
// missing file missing, and a change on a missing file creates it.
func TestUpdateWritesOnlyOnChange(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "new.jsonc")
	boom := errors.New("boom")
	if err := Update(context.Background(), path, func(d *Doc) error { return boom }); !errors.Is(err, boom) {
		t.Fatalf("err = %v, want fn's error", err)
	}
	if err := Update(context.Background(), path, func(*Doc) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("no-op Update created the file: %v", err)
	}
	if err := Update(context.Background(), path, func(d *Doc) error { return d.Set("a", 1) }); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(path); err != nil || len(got) == 0 {
		t.Fatalf("file = %q, %v", got, err)
	}
}

// TestUpdateLockTimeout: while the lock is held elsewhere, Update with a
// deadline fails with atomicfile.ErrLockTimeout and never calls fn.
func TestUpdateLockTimeout(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "c.jsonc")
	lk, err := atomicfile.Lock(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer lk.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	called := false
	err = Update(ctx, path, func(*Doc) error { called = true; return nil })
	if !errors.Is(err, atomicfile.ErrLockTimeout) || called {
		t.Fatalf("err = %v, called = %v", err, called)
	}
}

// TestUpdateRefusesBrokenFile: a file with syntax errors fails before fn.
func TestUpdateRefusesBrokenFile(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "bad.jsonc")
	if err := os.WriteFile(path, []byte(`{"a": }`), 0o600); err != nil {
		t.Fatal(err)
	}
	called := false
	if err := Update(context.Background(), path, func(*Doc) error { called = true; return nil }); err == nil || called {
		t.Fatalf("err = %v, called = %v", err, called)
	}
}
