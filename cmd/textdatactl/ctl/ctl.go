// Package ctl is textdatactl's command tree: one namespace per format, each
// backed by that format's own command package.
//
// Mounting is by delegation, not by grafting cobra trees: `textdatactl dotenv
// <args>` hands <args> to dotenvcmd.Execute with the program name
// "textdatactl dotenv". The format's CLI then owns everything — flag parsing,
// exit codes (0/1/2), the --json envelope, exec plugins — so the verbs behave
// byte-for-byte as they do in the format's standalone binary (dotenvctl), and a
// format CLI is added here without textdatactl learning its flags. jsonc
// mounts the same way, through jsonc/cli/jsonccmd ("textdatactl jsonc").
//
// The verbs textdatactl owns itself are `version` (build provenance, see
// version.go) and `completion`, whose script completes every format: the
// script's `__complete <format> …` requests are delegated like any other
// format invocation.
package ctl

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"
	"github.com/ubgo/voltkit/versioncmd"

	"github.com/ubgo/textdata/dotenv/cli/dotenvcmd"
	"github.com/ubgo/textdata/jsonc/cli/jsonccmd"
)

// Program is this binary's name.
const Program = "textdatactl"

// Exit codes, the same contract every textdata CLI keeps.
const (
	ExitOK      = 0
	ExitFailure = 1
	ExitUsage   = 2
)

// Format is one mounted format CLI.
type Format struct {
	// Name is the namespace (the first argument): "dotenv".
	Name string
	// Short is the one-line help text.
	Short string
	// Execute runs the format's CLI with the namespace stripped from args and
	// the program name it should print for itself; it returns the exit code.
	Execute func(args []string, stdout, stderr io.Writer, program string) int
}

// Formats lists the mounted format CLIs in help order.
func Formats() []Format {
	return []Format{
		{
			Name:  "dotenv",
			Short: "Comment-preserving .env control (get, set, diff, matrix, run, github, vercel)",
			Execute: func(args []string, stdout, stderr io.Writer, program string) int {
				// Exec plugins call back "$DOTENVCTL_BIN" (this binary), so
				// they need the namespace to reach the verbs.
				return dotenvcmd.Execute(args, stdout, stderr,
					dotenvcmd.WithProgramName(program), dotenvcmd.WithCallbackArgs("dotenv"))
			},
		},
		{
			Name:  "jsonc",
			Short: "Comment-preserving JSON/JSONC control (get, set, unset, restore, delete, diff, to-json, fmt, validate)",
			Execute: func(args []string, stdout, stderr io.Writer, program string) int {
				return jsonccmd.Execute(args, stdout, stderr, jsonccmd.WithProgramName(program))
			},
		},
	}
}

// Execute runs textdatactl with args (without the program name) and returns
// the exit code. It never calls os.Exit.
func Execute(args []string, stdout, stderr io.Writer) int {
	formats := Formats()
	if len(args) > 0 {
		for _, f := range formats {
			if args[0] == f.Name {
				return f.Execute(args[1:], stdout, stderr, Program+" "+f.Name)
			}
		}
	}
	// The completion script (`textdatactl completion <shell>`) asks
	// `textdatactl __complete <words…>`. Words inside a format namespace are
	// that format's to complete, so the request is handed to the format CLI
	// with the namespace stripped, like any other invocation.
	if len(args) > 1 && (args[0] == cobra.ShellCompRequestCmd || args[0] == cobra.ShellCompNoDescRequestCmd) {
		for _, f := range formats {
			if args[1] == f.Name {
				rest := append([]string{args[0]}, args[2:]...)
				return f.Execute(rest, stdout, stderr, Program+" "+f.Name)
			}
		}
	}

	root := newRoot(formats)
	root.SetArgs(args)
	root.SetOut(stdout)
	root.SetErr(stderr)
	if err := root.Execute(); err != nil {
		// Every error the shell itself can raise is a usage error (unknown
		// format or flag): formats run outside cobra, above, and `version`
		// cannot fail short of a broken stdout.
		_, _ = fmt.Fprintf(stderr, "%s: %v\nRun '%s --help' for usage.\n", Program, err, Program)
		return ExitUsage
	}
	return ExitOK
}

// newRoot builds the help-and-version shell. Format namespaces appear as
// commands so --help lists them; running one goes through Execute's
// delegation, so their RunE is only reached via cobra for `help <format>`.
func newRoot(formats []Format) *cobra.Command {
	root := &cobra.Command{
		Use:   Program,
		Short: "Comment- and byte-preserving editors for human-edited config files",
		Long:  "textdatactl reads and edits config files without disturbing a byte you did not\ntouch: comments, blank lines, ordering and quoting survive every edit.\nEach format is a namespace: textdatactl <format> <verb> ...",
		// The same string `textdatactl version` prints, from the same
		// source (voltkit versioncmd: ldflags stamp, else module version).
		Version:       versioncmd.Collect(Program).Version,
		SilenceUsage:  true,
		SilenceErrors: true,
		Args: func(_ *cobra.Command, args []string) error {
			if len(args) > 0 {
				return fmt.Errorf("unknown format %q", args[0])
			}
			return nil
		},
		RunE: func(c *cobra.Command, _ []string) error { return c.Help() },
	}
	root.AddCommand(newVersionCmd())
	for _, f := range formats {
		f := f
		root.AddCommand(&cobra.Command{
			Use:                f.Name + " <verb> [args]",
			Short:              f.Short,
			DisableFlagParsing: true,
			// Execute delegates before cobra runs, so this is reached only
			// when the tree is driven directly (tests, embedding); it does
			// the same delegation. A RunE also makes cobra list the format
			// as a command rather than a help topic.
			RunE: func(c *cobra.Command, args []string) error {
				if code := f.Execute(args, c.OutOrStdout(), c.ErrOrStderr(), Program+" "+f.Name); code != ExitOK {
					return fmt.Errorf("%s exited with code %d", f.Name, code)
				}
				return nil
			},
		})
	}
	return root
}
