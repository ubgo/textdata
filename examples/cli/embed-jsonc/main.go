// Command embed-jsonc runs the jsonc verbs inside another Go program three
// ways: jsonccmd.Execute in-process, jsonccmd.Command grafted into a host
// cobra tree, and the whole textdatactl umbrella through ctl.Execute.
package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/ubgo/textdata/cmd/textdatactl/ctl"
	"github.com/ubgo/textdata/jsonc/cli/jsonccmd"
)

// hostProgram is the host binary's name. Command names its subtree "jsonc",
// so users of the host type "myapp jsonc get FILE PATH".
const hostProgram = "myapp"

// configName is the file the demo creates in its temp directory.
const configName = "config.jsonc"

// missingFile is a relative name that does not exist, so the failure
// message carries no temp path and the output stays stable.
const missingFile = "missing.jsonc"

// missingEnv is the same idea for the dotenv namespace of the umbrella.
const missingEnv = "missing.env"

// sampleConfig carries a comment, so the demo shows it surviving an edit.
const sampleConfig = `{
  // where the API listens
  "server": {"host": "localhost", "port": 8080}
}
`

func main() {
	dir, err := os.MkdirTemp("", "embed-jsonc-")
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

// run executes every step of the guide against a config file in dir.
// The caller owns dir (main makes a temp one, the test passes t.TempDir()),
// so nothing is written outside it.
func run(w io.Writer, dir string) error {
	file := filepath.Join(dir, configName)
	if err := os.WriteFile(file, []byte(sampleConfig), 0o600); err != nil {
		return err
	}

	fmt.Fprintln(w, "exit:", runJSONC(w, "set", file, "server.port", "9090", "--json-value"))
	// The comment and the one-line layout survive; only the number changed.
	edited, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	fmt.Fprint(w, string(edited))
	fmt.Fprintln(w, "exit:", runJSONC(w, "get", file, "server.port"))
	fmt.Fprintln(w, "exit:", runJSONC(w, "get", missingFile, "server.port"))
	fmt.Fprintln(w, "exit:", runGrafted(w, "jsonc", "get", file, "server", "--json"))
	fmt.Fprintln(w, "exit:", runGrafted(w, "jsonc", "get", missingFile, "server"))
	fmt.Fprintln(w, "exit:", runGrafted(w, "jsonc", "nosuchverb"))
	fmt.Fprintln(w, "formats:", formatNames())
	fmt.Fprintln(w, "exit:", ctl.Execute([]string{"jsonc", "get", file, "server.host"}, w, w))
	fmt.Fprintln(w, "exit:", ctl.Execute([]string{"dotenv", "-f", missingEnv, "get", "PORT"}, w, w))
	return nil
}

// runJSONC runs one jsonc command line in-process. args are what a user
// types after "textdatactl jsonc"; both streams go to w so the demo shows
// failures too. Execute returns 0 ok, 1 failed (or diff found differences),
// 2 usage, and never calls os.Exit.
//
//ds:def id=runjsonc-bqrk46h2
func runJSONC(w io.Writer, args ...string) int {
	return jsonccmd.Execute(args, w, w, jsonccmd.WithProgramName(hostProgram+" jsonc"))
}

// runGrafted adds jsonccmd.Command to a host root and runs one command line.
// A verb failure comes back as *jsonccmd.ExitError, already printed (Quiet);
// a cobra usage error comes back unprinted, so the host reports it.
//
//ds:def id=rungrafted-sefus95z
func runGrafted(w io.Writer, args ...string) int {
	root := &cobra.Command{Use: hostProgram, SilenceUsage: true, SilenceErrors: true}
	root.AddCommand(jsonccmd.Command(jsonccmd.WithProgramName(hostProgram + " jsonc")))
	root.SetArgs(args)
	root.SetOut(w)
	root.SetErr(w)
	err := root.Execute()
	if err == nil {
		return jsonccmd.ExitOK
	}
	var xe *jsonccmd.ExitError
	if errors.As(err, &xe) {
		if !xe.Quiet && xe.Err != nil {
			fmt.Fprintf(w, "%s: %v\n", hostProgram, xe.Err)
		}
		return xe.Code
	}
	fmt.Fprintf(w, "%s: %v\n", hostProgram, err)
	return jsonccmd.ExitUsage
}

// formatNames lists the namespaces the textdatactl umbrella mounts; a host
// that builds its own umbrella can range over ctl.Formats and call each
// Format.Execute with its own program name.
//
//ds:def id=formatnames-tbfqmvgp
func formatNames() []string {
	var names []string
	for _, f := range ctl.Formats() {
		names = append(names, f.Name)
	}
	return names
}
