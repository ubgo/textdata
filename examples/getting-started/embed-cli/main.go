// Command embed-cli runs textdatactl in-process through package ctl: the same
// verbs, flags, exit codes and --json envelope as the installed binary, with
// output going to any io.Writer. Use it to drive the CLI from tests or to ship
// it inside a larger tool.
package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/ubgo/textdata/cmd/textdatactl/ctl"
)

// configSource is the JSONC file the demo commands read and edit.
const configSource = `{
  // Where the API listens.
  "server": {"host": "0.0.0.0", "port": 8080},
}
`

func main() {
	dir, err := os.MkdirTemp("", "embed-cli-example-")
	if err == nil {
		err = run(os.Stdout, dir)
		// Cleanup is best-effort: the run's result matters more than a
		// leftover temp directory.
		_ = os.RemoveAll(dir)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// run writes a sample file into dir and runs a few verbs on it.
// The caller owns dir (main makes a temp one, the test passes t.TempDir()),
// so nothing is written outside it.
func run(w io.Writer, dir string) error {
	config := filepath.Join(dir, "config.jsonc")
	if err := os.WriteFile(config, []byte(configSource), 0o600); err != nil {
		return err
	}
	listFormats(w)
	return runVerbs(w, config)
}

// listFormats prints the format namespaces textdatactl mounts.
//
//ds:def id=listformats-pccwjeek
func listFormats(w io.Writer) {
	for _, f := range ctl.Formats() {
		fmt.Fprintf(w, "format %-6s %s\n", f.Name, f.Short)
	}
}

// runVerbs calls ctl.Execute with the arguments a shell would pass after the
// program name, and checks the exit code against the named constants.
//
//ds:def id=runverbs-ymcdztp4
func runVerbs(w io.Writer, config string) error {
	for _, args := range [][]string{
		{"jsonc", "set", config, "server.port", "9090", "--json-value"},
		{"jsonc", "get", config, "server.port"},
		{"jsonc", "get", config, "server", "--json"},
	} {
		if code := ctl.Execute(args, w, w); code != ctl.ExitOK {
			return fmt.Errorf("%v: exit %d", args[:2], code)
		}
	}
	code := ctl.Execute([]string{"jsonc", "get", config, "missing"}, io.Discard, io.Discard)
	fmt.Fprintf(w, "missing path: exit %d (failure=%v)\n", code, code == ctl.ExitFailure)
	return nil
}
