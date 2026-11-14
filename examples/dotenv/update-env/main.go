// Command update edits a shared .env with dotenv.Update: five workers each
// set their own key and bump a shared counter at the same time, and every
// edit survives with the file's comment intact. Update holds the
// cross-process lock (atomicfile.Lock) across open, edit and save, so the
// same holds for separate processes; goroutines keep the output stable.
package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/ubgo/textdata/dotenv"
)

// Demo sizes and the starting file.
const (
	envFile  = ".env"
	workers  = 5
	lockWait = 5 * time.Second
	initial  = "# deploy counter, bumped by every worker\nDEPLOYS=0\n"
)

func main() {
	dir, err := os.MkdirTemp("", "dotenv-update-example-")
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

// run starts the workers and prints what they leave behind.
func run(w io.Writer, dir string) error {
	path := filepath.Join(dir, envFile)
	if err := os.WriteFile(path, []byte(initial), 0o600); err != nil {
		return err
	}
	var wg sync.WaitGroup
	errs := make(chan error, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs <- recordDeploy(path, "WORKER_"+strconv.Itoa(i))
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			return err
		}
	}
	f, err := dotenv.Open(path)
	if err != nil {
		return err
	}
	n, _ := f.Get("DEPLOYS")
	fmt.Fprintf(w, "DEPLOYS=%s\nkeys: %d\ncomment kept: %t\n", n, len(f.Keys()), f.Entries()[0].Kind == dotenv.KindComment)
	return nil
}

// recordDeploy is one process's read-modify-write. The deadline bounds only
// the wait for the lock; the file is saved when fn returns nil and changed
// something, never when it returns an error.
//
//ds:def id=dotenv-update-recorddeploy-c7r3n9fs
func recordDeploy(path, worker string) error {
	ctx, cancel := context.WithTimeout(context.Background(), lockWait)
	defer cancel()
	return dotenv.Update(ctx, path, func(f *dotenv.File) error {
		v, _ := f.Get("DEPLOYS")
		n, err := strconv.Atoi(v)
		if err != nil {
			return err
		}
		f.Set(worker, "done")
		f.Set("DEPLOYS", strconv.Itoa(n+1))
		return nil
	})
}
