// Command embed-dotenvctl runs the whole dotenvctl verb tree inside another Go
// program: dotenvcmd.Execute in-process, the exit code as a return value, the
// host's own name in the lines the CLI prints about itself, and the verbs
// mounted as a subcommand of a host cobra tree.
package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/ubgo/textdata/dotenv/cli/dotenvcmd"
)

// hostProgram is the host binary's name, and hostEnvVerb the subcommand it
// mounts the dotenv verbs under; users type "myapp env get KEY".
const (
	hostProgram = "myapp"
	hostEnvVerb = "env"
)

// envFileName is the file the demo creates in its temp directory.
const envFileName = ".env"

// missingFile is a relative name that does not exist: dotenvctl reads a
// missing file as empty, so a get on it fails with a message that does not
// carry a temp path, which keeps the output stable.
const missingFile = "missing.env"

// sampleEnv holds one plain and one derived value, so --expand has work to do.
const sampleEnv = "HOST=localhost\nURL=http://${HOST}:8080\n"

func main() {
	dir, err := os.MkdirTemp("", "embed-dotenvctl-")
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

// run executes every step of the guide against a .env file in dir.
// The caller owns dir (main makes a temp one, the test passes t.TempDir()),
// so nothing is written outside it.
func run(w io.Writer, dir string) error {
	envFile := filepath.Join(dir, envFileName)
	// 0600 because env files hold credentials.
	if err := os.WriteFile(envFile, []byte(sampleEnv), 0o600); err != nil {
		return err
	}

	fmt.Fprintln(w, "exit:", runVerb(w, "-f", envFile, "get", "URL", "--expand"))
	fmt.Fprintln(w, "exit:", runVerb(w, "-f", missingFile, "get", "PORT"))
	fmt.Fprintln(w, "exit:", runAsHost(w, "nosuchverb"))
	fmt.Fprintln(w, "exit:", runHostTree(w, hostEnvVerb, "-f", envFile, "set", "PORT=8080", "--json"))
	fmt.Fprintln(w, "exit:", runHostTree(w, hostEnvVerb, "-f", missingFile, "get", "PORT"))
	return nil
}

// runVerb runs one dotenvctl command line in-process. args are what a user
// types after "dotenvctl"; both streams go to w so the demo shows failures
// too. Execute returns the exit code (0 ok, 1 failed, 2 usage) and never
// calls os.Exit, so the host decides what a failure means.
//
//ds:def id=runverb-hyve6qvu
func runVerb(w io.Writer, args ...string) int {
	return dotenvcmd.Execute(args, w, w)
}

// runAsHost is runVerb for a host that mounts the verbs under its own words.
// WithProgramName puts "myapp env" in the failure prefix, the --help hint,
// the help Usage line and --version, so no message names a dotenvctl binary
// the user does not have.
//
//ds:def id=runashost-f5w6cdqm
func runAsHost(w io.Writer, args ...string) int {
	return dotenvcmd.Execute(args, w, w, dotenvcmd.WithProgramName(hostProgram+" "+hostEnvVerb))
}

// envCommand mounts the verbs as "myapp env" in a host cobra tree. dotenvcmd
// has no cobra Command to graft, so the host delegates: flag parsing is off
// (dotenvctl parses its own flags) and RunE hands the raw args to Execute.
// The exit code travels back in an exitCodeError.
//
//ds:def id=envcommand-whd27a9r
func envCommand() *cobra.Command {
	return &cobra.Command{
		Use:                hostEnvVerb + " <verb> [args]",
		Short:              "Edit .env files (the dotenvctl verbs)",
		DisableFlagParsing: true,
		RunE: func(c *cobra.Command, args []string) error {
			code := dotenvcmd.Execute(args, c.OutOrStdout(), c.ErrOrStderr(),
				dotenvcmd.WithProgramName(hostProgram+" "+hostEnvVerb))
			if code != dotenvcmd.ExitOK {
				return exitCodeError(code)
			}
			return nil
		},
	}
}

// exitCodeError carries a delegated exit code out of the host tree. The
// failure was already printed by the dotenv verbs, so the host prints nothing.
type exitCodeError int

// Error implements error.
func (e exitCodeError) Error() string { return fmt.Sprintf("exit %d", int(e)) }

// runHostTree builds the host root with the env subcommand and runs one
// command line through it, returning the exit code main would pass to
// os.Exit.
//
//ds:def id=runhosttree-3jzzrrdy
func runHostTree(w io.Writer, args ...string) int {
	root := &cobra.Command{Use: hostProgram, SilenceUsage: true, SilenceErrors: true}
	root.AddCommand(envCommand())
	root.SetArgs(args)
	root.SetOut(w)
	root.SetErr(w)
	if err := root.Execute(); err != nil {
		if code, ok := err.(exitCodeError); ok {
			return int(code)
		}
		fmt.Fprintf(w, "%s: %v\n", hostProgram, err)
		return dotenvcmd.ExitUsage
	}
	return dotenvcmd.ExitOK
}
