// Command vercel mounts the real Vercel provider (vercelplugin) under a cobra
// root, exactly as dotenvctl does, and drives it with a fake vercel CLI: every
// call the plugin makes goes through providerkit.Deps.Runner, which here
// records the argv and stdin and answers from memory. No vercel binary, no
// network and no Vercel account are touched.
//
// Run it to see the banner, the per-target calls, --sensitive, the empty-list
// hint, prune's keep-set and a push that fails part-way.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/ubgo/textdata/clikit/outfmt"
	"github.com/ubgo/textdata/clikit/providerkit"
	"github.com/ubgo/textdata/dotenv/cli/envkit"
	vercelplugin "github.com/ubgo/textdata/dotenv/cli/providers/vercel"
)

// programName prefixes failure lines, as dotenvctl's does.
const programName = "dotenvctl"

// flagJSON is the host's global output switch.
const flagJSON = "json"

// envFileMode is 0600 because env files hold credentials.
const envFileMode = 0o600

// exitUsage is what a host returns when cobra itself rejects the command
// line (unknown flag, wrong arguments), matching the kit's usage code.
const exitUsage = 2

// envContent is the project's .env: a reference, a placeholder, an empty
// value and one Vercel-only key marked by prefix.
const envContent = `DB_URL=postgres://${DB_HOST}/app
DB_HOST=db.internal
API_KEY=__YOU__
VERCEL_ANALYTICS_ID=abc123
EMPTY=
`

// productionTable mimics `vercel env ls production`, banner lines included:
// the plugin parses the first column of the data rows.
const productionTable = `Vercel CLI 33.0.0
> Environment Variables found for acme/web [120ms]

 name       value       environments  created
 API_KEY    Encrypted   Production    3d ago
 DB_URL     Encrypted   Production    3d ago
 OLD_FLAG   Encrypted   Production    9d ago
`

func main() {
	dir, err := os.MkdirTemp("", "vercel-provider-example-")
	if err == nil {
		err = run(os.Stdout, dir)
		// Cleanup is best-effort: the run's result matters more than a
		// leftover temp directory.
		_ = os.RemoveAll(dir)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "vercel:", err)
		os.Exit(1)
	}
}

// fakeVercel stands in for the vercel CLI. It answers `whoami` and
// `env ls production`, returns an empty table for other targets, fails
// `env add` for one key when failOn is set, and records every call. A real
// host passes providerkit.ExecRunner{}.
//
//ds:def id=fakevercel-tdjduh94
type fakeVercel struct {
	w      io.Writer
	failOn string
}

// Run implements providerkit.Runner.
//
//ds:def id=fakevercel-run-jkptq7d2
func (f *fakeVercel) Run(_ context.Context, stdin, name string, args ...string) ([]byte, error) {
	joined := strings.Join(args, " ")
	fmt.Fprintf(f.w, "    [%s %s] stdin=%q\n", name, joined, stdin)
	switch {
	case joined == "whoami":
		return []byte("acme-bot\n"), nil
	case joined == "env ls production":
		return []byte(productionTable), nil
	case strings.HasPrefix(joined, "env ls"):
		return []byte("Vercel CLI 33.0.0\n> No Environment Variables found\n"), nil
	case f.failOn != "" && strings.HasPrefix(joined, "env add "+f.failOn+" "):
		return nil, errors.New("exit status 1: Error: simulated refusal")
	}
	return nil, nil
}

// mount builds the host side and adds vercelplugin under the root as
// `vercel`. Source errors go through providerkit.Fail so they print once
// and carry an exit code, as dotenvctl's do.
//
//ds:def id=mount-2wyy43pv
func mount(w io.Writer, envPath string, cli providerkit.Runner) *cobra.Command {
	printer := &outfmt.Printer{Out: w, Err: w, Program: programName}
	root := &cobra.Command{Use: programName, SilenceErrors: true, SilenceUsage: true}
	root.PersistentFlags().BoolVar(&printer.JSON, flagJSON, false, "emit the {ok,data|error} envelope")
	root.SetErr(io.Discard)
	root.AddCommand(vercelplugin.New().Command(providerkit.Deps{
		Printer: printer,
		File:    envPath,
		Source: func(opts providerkit.SelectOpts) (providerkit.Source, error) {
			sel, err := envkit.SelectFile(envPath, opts)
			if err != nil {
				return nil, providerkit.Fail(printer, outfmt.CodeIO, "%v", err)
			}
			return sel.Flat(), nil
		},
		Runner:      cli, // providerkit.ExecRunner{} in production
		Interactive: func() bool { return false },
		Ask:         func(string) bool { return false },
	}))
	return root
}

// execute prints the command line, runs it and returns the host exit code:
// the kit's *CmdError code, or usage for cobra's own parse errors.
func execute(w io.Writer, root *cobra.Command, args ...string) int {
	fmt.Fprintln(w, "$", programName, strings.Join(args, " "))
	root.SetArgs(args)
	err := root.Execute()
	var ce *providerkit.CmdError
	switch {
	case err == nil:
		return 0
	case errors.As(err, &ce):
		return ce.Code
	default:
		fmt.Fprintf(w, "%s: %v\n", programName, err)
		return exitUsage
	}
}

// scenarios are the command lines the guide walks through, in order.
//
//ds:def id=scenarios-safb27wv
func scenarios() [][]string {
	return [][]string{
		// 1. preview: default target is development
		{"vercel", "push", "--dry-run"},
		// 2. two targets, stored as sensitive: one `env add` per key per target
		{"vercel", "push", "DB_URL", "--target", "production,preview", "--sensitive", "--yes"},
		// 3. only the Vercel-only keys, prefix stripped
		{"vercel", "push", "--prefix", "VERCEL_", "--strip-prefix", "--target", "preview", "--dry-run"},
		// 4. names only, parsed from the table
		{"vercel", "list", "--target", "production"},
		// 5. an empty target says which target it read
		{"vercel", "list", "--target", "preview"},
		// 6. prune preview: the local placeholder spares the real remote value
		{"vercel", "prune", "--target", "production", "--dry-run"},
		// 7. --sensitive belongs to push only
		{"vercel", "list", "--sensitive"},
	}
}

// run writes the .env and walks the plugin through each scenario.
func run(w io.Writer, dir string) error {
	envPath := filepath.Join(dir, ".env")
	if err := os.WriteFile(envPath, []byte(envContent), envFileMode); err != nil {
		return err
	}

	// A fresh root per command line: cobra keeps parsed flag values on the
	// command, so reusing one tree would carry --dry-run into the next run.
	for _, args := range scenarios() {
		fmt.Fprintf(w, "exit %d\n\n", execute(w, mount(w, envPath, &fakeVercel{w: w}), args...))
	}
	return partialPush(w, envPath)
}

// partialPush makes the backend refuse DB_HOST. DB_URL was already written
// and is reported; VERCEL_ANALYTICS_ID was never attempted. Every push is an
// upsert (env add --force), so re-running after the fix completes the rest.
//
//ds:def id=partialpush-dugzgphc
func partialPush(w io.Writer, envPath string) error {
	cli := &fakeVercel{w: w, failOn: "DB_HOST"}
	args := []string{"--json", "vercel", "push", "--target", "production", "--yes"}
	fmt.Fprintf(w, "exit %d\n", execute(w, mount(w, envPath, cli), args...))
	return nil
}
