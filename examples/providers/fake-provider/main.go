// Command fake-provider builds a complete push provider on clikit/providerkit
// over an in-memory secret store, mounts it under a cobra root the way
// dotenvctl mounts github and vercel, and drives it through push, list and
// prune — with the confirm gate, the --json envelope and a mid-way failure.
//
// The store is a map, so no network, account or backend CLI is involved.
// Values come from a real .env file through envkit.SelectFile, exactly as the
// dotenv CLI feeds its own providers.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/ubgo/textdata/clikit/outfmt"
	"github.com/ubgo/textdata/clikit/providerkit"
	"github.com/ubgo/textdata/dotenv/cli/envkit"
)

// programName prefixes failure lines ("memctl: ...").
const programName = "memctl"

// providerName is the namespace the plugin mounts as ("memctl mem push").
const providerName = "mem"

// flagJSON is the host's global output switch; the kit's verbs read it
// through the shared Printer.
const flagJSON = "json"

// exitUsage is what a host returns when cobra itself rejects the command
// line, matching the kit's usage code.
const exitUsage = 2

// envFileMode is 0600 because env files hold credentials.
const envFileMode = 0o600

// envContent is the local source of truth: two real values, a placeholder
// the push guard skips, and nothing for the remote-only names.
const envContent = "DB_URL=postgres://db/app\nREDIS_URL=redis://cache:6379\nAPI_KEY=__YOU__\n"

// errBackendDown is the failure the store injects to show a mid-way stop.
var errBackendDown = errors.New("backend unavailable")

func main() {
	dir, err := os.MkdirTemp("", "fake-provider-example-")
	if err == nil {
		err = run(os.Stdout, dir)
		// Cleanup is best-effort: the run's result matters more than a
		// leftover temp directory.
		_ = os.RemoveAll(dir)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "fake-provider:", err)
		os.Exit(1)
	}
}

// memStore is the backend adapter: it implements exactly the capabilities the
// backend has (write, list names, delete) and nothing else. failOn names a
// key whose Set fails, to show the partial-failure record.
//
//ds:def id=memstore-mt7cwnnm
type memStore struct {
	values map[string]string
	failOn string
}

// Set implements providerkit.SecretWriter (an upsert).
//
//ds:def id=memstore-set-dgg9twz8
func (s *memStore) Set(_ context.Context, name, value string) error {
	if name == s.failOn {
		return errBackendDown
	}
	s.values[name] = value
	return nil
}

// Names implements providerkit.SecretLister: names only, never values.
//
//ds:def id=memstore-names-fp2jt9ym
func (s *memStore) Names(context.Context) ([]string, error) {
	names := make([]string, 0, len(s.values))
	for n := range s.values {
		names = append(names, n)
	}
	slices.Sort(names)
	return names, nil
}

// Delete implements providerkit.SecretDeleter.
//
//ds:def id=memstore-delete-b4uvm46z
func (s *memStore) Delete(_ context.Context, name string) error {
	delete(s.values, name)
	return nil
}

// EmptyListHint implements providerkit.EmptyListHinter: when the store is
// empty, `list` prints this instead of the neutral default, so the plugin can
// name the scope flag the user probably forgot.
//
//ds:def id=memstore-emptylisthint-nnk3xbfk
func (s *memStore) EmptyListHint() string {
	return "no secrets in the memory store (did you push yet?)"
}

// memPlugin is the provider. Command is the whole mandatory contract: return
// a cobra namespace built from Deps. The kit generates the three standard
// verbs; the plugin adds only its gate and its --json meta.
//
//ds:def id=memplugin-k74n5sdy
type memPlugin struct {
	store *memStore
}

// Command implements providerkit.Plugin.
//
//ds:def id=memplugin-command-edwkzky2
func (p *memPlugin) Command(deps providerkit.Deps) *cobra.Command {
	root := &cobra.Command{Use: providerName, Short: "Sync values to an in-memory store"}
	cfg := providerkit.VerbConfig{Gate: p.gate, Meta: p.meta}
	root.AddCommand(
		providerkit.NewPushCmd(deps, p.store, cfg),
		providerkit.NewListCmd(deps, p.store),
		providerkit.NewPruneCmd(deps, p.store, p.store, cfg),
	)
	return root
}

// gate runs after selection and before any write, dry runs included: print
// where values will land, then hand the decision to Confirm (dry run and
// --yes pass, a human is asked, anything else is refused with exit 2).
//
//ds:def id=memplugin-gate-6ybxkq7p
func (p *memPlugin) gate(_ context.Context, deps providerkit.Deps, info providerkit.GateInfo) error {
	deps.Printer.Humanf("target : memory (%d stored)", len(p.store.values))
	deps.Printer.Humanf("keys   : %d", len(info.Names))
	return providerkit.Confirm(deps, info)
}

// meta adds resolution context to the --json payload; it runs under --json
// only, so a costly lookup is never paid in human mode.
//
//ds:def id=memplugin-meta-h2jyhftr
func (p *memPlugin) meta(context.Context) (map[string]string, error) {
	return map[string]string{"target": "memory"}, nil
}

// newDeps wires what a host gives every plugin. Source re-resolves the .env
// file with the flags the verb parsed; Interactive is false, like CI, so
// mutating verbs need --yes. Runner is unused by an in-memory store; a store
// that shells out to a backend CLI calls it instead of os/exec.
//
//ds:def id=newdeps-gcrdbt82
func newDeps(printer *outfmt.Printer, envPath string) providerkit.Deps {
	return providerkit.Deps{
		Printer: printer,
		File:    envPath,
		Source: func(opts providerkit.SelectOpts) (providerkit.Source, error) {
			sel, err := envkit.SelectFile(envPath, opts)
			if err != nil {
				return nil, providerkit.Fail(printer, outfmt.CodeIO, "%v", err)
			}
			return sel.Flat(), nil
		},
		Runner:      providerkit.ExecRunner{},
		Interactive: func() bool { return false },
		Ask:         func(string) bool { return false },
	}
}

// execute mounts the plugin under a fresh root with a global --json flag,
// runs one command line and returns the exit code a host returns: 0, or the
// code carried by the kit's *CmdError (1 failed, 2 usage). The kit already
// printed the failure, so cobra's own error printing is silenced.
//
//ds:def id=execute-kwuqrdz4
func execute(w io.Writer, envPath string, store *memStore, args ...string) int {
	printer := &outfmt.Printer{Out: w, Err: w, Program: programName}
	root := &cobra.Command{Use: programName, SilenceErrors: true, SilenceUsage: true}
	root.PersistentFlags().BoolVar(&printer.JSON, flagJSON, false, "emit the {ok,data|error} envelope")
	root.SetErr(io.Discard)
	root.AddCommand((&memPlugin{store: store}).Command(newDeps(printer, envPath)))
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
		// Only cobra's own parse errors (unknown flag, wrong arguments)
		// arrive without a CmdError; a host reports them as usage.
		fmt.Fprintf(w, "%s: %v\n", programName, err)
		return exitUsage
	}
}

// run writes the .env and walks the provider through its verbs.
func run(w io.Writer, dir string) error {
	envPath := filepath.Join(dir, ".env")
	if err := os.WriteFile(envPath, []byte(envContent), envFileMode); err != nil {
		return err
	}

	// The remote already holds a stale name and a real API_KEY someone set.
	store := &memStore{values: map[string]string{"OLD_FLAG": "1", "API_KEY": "real-key"}}
	for _, args := range [][]string{
		{providerName, "push"},
		{providerName, "push", "--dry-run"},
		{providerName, "push", "--yes"},
		{providerName, "list"},
		{providerName, "prune", "--dry-run"},
		{providerName, "prune", "--keep", "OLD_FLAG", "--dry-run"},
		{providerName, "prune", "--yes"},
		{"--json", providerName, "push", "--prefix", "DB_", "--yes"},
	} {
		fmt.Fprintf(w, "exit %d\n\n", execute(w, envPath, store, args...))
	}
	empty := &memStore{values: map[string]string{}}
	fmt.Fprintf(w, "exit %d\n\n", execute(w, envPath, empty, providerName, "list"))
	return partialFailure(w, envPath)
}

// partialFailure makes the backend fail on the second key. What was already
// written is still reported, and under --json the record and the error
// travel in ONE ok:false envelope, so a script never parses two objects.
//
//ds:def id=partialfailure-fttx3dk7
func partialFailure(w io.Writer, envPath string) error {
	for _, args := range [][]string{
		{providerName, "push", "--yes"},
		{"--json", providerName, "push", "--yes"},
	} {
		store := &memStore{values: map[string]string{}, failOn: "REDIS_URL"}
		fmt.Fprintf(w, "exit %d\n\n", execute(w, envPath, store, args...))
	}
	return nil
}
