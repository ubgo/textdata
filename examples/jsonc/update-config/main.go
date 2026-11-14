// Command update edits a shared JSONC config with jsonc.Update: six workers
// each record their own entry and bump a shared counter at the same time, and
// every edit survives with the file's comments intact. Update holds the
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

	"github.com/ubgo/textdata/jsonc"
)

// Demo sizes and the starting file.
const (
	configFile = "config.jsonc"
	workers    = 6
	lockWait   = 5 * time.Second
	initial    = `{
  // how many runs were recorded
  "runs": 0,
  "agents": {}
}
`
)

func main() {
	dir, err := os.MkdirTemp("", "jsonc-update-example-")
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

// run starts the workers and prints the file they leave behind.
func run(w io.Writer, dir string) error {
	path := filepath.Join(dir, configFile)
	if err := os.WriteFile(path, []byte(initial), 0o600); err != nil {
		return err
	}
	var wg sync.WaitGroup
	errs := make(chan error, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs <- recordRun(path, "agent"+strconv.Itoa(i))
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			return err
		}
	}
	d, err := jsonc.Open(path)
	if err != nil {
		return err
	}
	runs, _ := d.GetInt("runs")
	keys, _ := d.Keys("agents")
	c, _ := d.Comment("runs")
	fmt.Fprintf(w, "runs: %d\nagents: %d\ncomment kept: %t\n", runs, len(keys), c != "")
	return nil
}

// recordRun is one process's read-modify-write. The deadline bounds only the
// wait for the lock; fn's edits are saved when it returns nil, and nothing is
// written when it returns an error or changes nothing.
//
//ds:def id=jsonc-update-recordrun-v5k2m8xq
func recordRun(path, agent string) error {
	ctx, cancel := context.WithTimeout(context.Background(), lockWait)
	defer cancel()
	return jsonc.Update(ctx, path, func(d *jsonc.Doc) error {
		n, err := d.GetInt("runs")
		if err != nil {
			return err
		}
		if err := d.Set("agents."+agent, map[string]any{"enabled": true}); err != nil {
			return err
		}
		return d.Set("runs", n+1)
	})
}
