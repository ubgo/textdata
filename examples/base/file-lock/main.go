// Command file-lock guards a read-modify-write with atomicfile.Lock: eight
// workers bump one counter file and no increment is lost, then a Lock with a
// deadline gives up while the lock is held. The workers are goroutines so the
// output is deterministic; the lock is the OS's own (flock, LockFileEx), held
// per Lock call, so separate processes exclude each other the same way.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ubgo/textdata/atomicfile"
)

// Demo sizes and the file the workers share.
const (
	counterFile = "runs.count"
	workers     = 8
	bumpsEach   = 25
	waitLimit   = 50 * time.Millisecond
)

func main() {
	dir, err := os.MkdirTemp("", "file-lock-example-")
	if err == nil {
		err = run(os.Stdout, dir)
		// Best-effort cleanup: the run's result matters more.
		_ = os.RemoveAll(dir)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// run bumps the counter from every worker, prints the total, then shows the
// timeout.
func run(w io.Writer, dir string) error {
	path := filepath.Join(dir, counterFile)
	var wg sync.WaitGroup
	errs := make(chan error, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < bumpsEach; j++ {
				if err := bump(context.Background(), path); err != nil {
					errs <- err
					return
				}
			}
		}()
	}
	wg.Wait()
	close(errs)
	if err := <-errs; err != nil {
		return err
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "counter: %s (want %d)\n", b, workers*bumpsEach)
	return timeout(w, path)
}

// bump is the read-modify-write the lock protects: without the lock two
// workers can read the same number and one increment is lost, even though
// each Write is atomic.
//
//ds:def id=filelock-bump-h3m8q2vz
func bump(ctx context.Context, path string) error {
	lk, err := atomicfile.Lock(ctx, path)
	if err != nil {
		return err
	}
	defer lk.Close()

	b, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	n, _ := strconv.Atoi(strings.TrimSpace(string(b))) // a missing file is 0
	return atomicfile.Write(path, []byte(strconv.Itoa(n+1)), atomicfile.Options{})
}

// timeout: while the lock is held, Lock with a deadline returns an error
// wrapping atomicfile.ErrLockTimeout that names the lock file.
//
//ds:def id=filelock-timeout-p6t2w9kc
func timeout(w io.Writer, path string) error {
	held, err := atomicfile.Lock(context.Background(), path)
	if err != nil {
		return err
	}
	defer held.Close()

	ctx, cancel := context.WithTimeout(context.Background(), waitLimit)
	defer cancel()
	_, err = atomicfile.Lock(ctx, path)
	fmt.Fprintf(w, "lock file: %s\n", filepath.Base(held.Path()))
	fmt.Fprintf(w, "second Lock: timeout=%t\n", errors.Is(err, atomicfile.ErrLockTimeout))
	return nil
}
