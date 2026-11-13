// Command github mounts the real GitHub provider (githubplugin) under a cobra
// root, exactly as dotenvctl does, and drives it with a fake gh: every call
// the plugin makes goes through providerkit.Deps.Runner, which here records
// the argv and stdin and answers from memory. No gh binary, no network and no
// GitHub account are touched.
//
// Run it to see the confirm banner, how the target is resolved, which gh
// commands each verb runs, and where the secret value travels (stdin, never
// argv).
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
	githubplugin "github.com/ubgo/textdata/dotenv/cli/providers/github"
)

// programName prefixes failure lines, as dotenvctl's does.
const programName = "dotenvctl"

// flagJSON is the host's global output switch.
const flagJSON = "json"

// exitUsage is what a host returns when cobra itself rejects the command
// line, matching the kit's usage code.
const exitUsage = 2

// envFileMode is 0600 because env files hold credentials.
const envFileMode = 0o600

// tokenVar is the gh auth override the banner labels.
const tokenVar = "GH_TOKEN"

// envContent marks deploy secrets with a prefix; one is still a placeholder.
const envContent = `GITHUB_SECRET_DB_URL=postgres://${DB_HOST}/app
GITHUB_SECRET_GHCR_PAT=ghp_example_only
GITHUB_SECRET_API_KEY=__YOU__
DB_HOST=db.internal
LOG_LEVEL=debug
app.name=api
`

func main() {
	dir, err := os.MkdirTemp("", "github-provider-example-")
	if err == nil {
		err = run(os.Stdout, dir)
		// Cleanup is best-effort: the run's result matters more than a
		// leftover temp directory.
		_ = os.RemoveAll(dir)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "github:", err)
		os.Exit(1)
	}
}

// fakeGh stands in for the gh CLI. It answers the two lookups the confirm
// gate makes (who am I, which repo is this directory) and `secret list`, and
// records every other call. A real host passes providerkit.ExecRunner{}.
//
//ds:def id=fakegh-pkpuy26f
type fakeGh struct {
	w      io.Writer
	remote []string // names `gh secret list` returns
}

// Run implements providerkit.Runner.
//
//ds:def id=fakegh-run-ccjrvarj
func (f *fakeGh) Run(_ context.Context, stdin, name string, args ...string) ([]byte, error) {
	joined := strings.Join(args, " ")
	fmt.Fprintf(f.w, "    [%s %s] stdin=%q\n", name, joined, stdin)
	switch {
	case joined == "api user --jq .login":
		return []byte("octocat\n"), nil
	case strings.HasPrefix(joined, "repo view"):
		return []byte("acme/api\n"), nil
	case strings.HasPrefix(joined, "secret list"):
		rows := make([]string, 0, len(f.remote))
		for _, n := range f.remote {
			rows = append(rows, fmt.Sprintf(`{"name":%q}`, n))
		}
		return []byte("[" + strings.Join(rows, ",") + "]"), nil
	}
	return nil, nil
}

// mount builds the host side: a Printer, a Source that resolves the .env with
// the flags the verb parsed, the fake runner, and a non-interactive session
// (like CI), then adds githubplugin under the root as `github`.
//
//ds:def id=mount-z5xybc2g
func mount(w io.Writer, envPath string, gh providerkit.Runner) *cobra.Command {
	printer := &outfmt.Printer{Out: w, Err: w, Program: programName}
	root := &cobra.Command{Use: programName, SilenceErrors: true, SilenceUsage: true}
	root.PersistentFlags().BoolVar(&printer.JSON, flagJSON, false, "emit the {ok,data|error} envelope")
	root.SetErr(io.Discard)
	root.AddCommand(githubplugin.New().Command(providerkit.Deps{
		Printer: printer,
		File:    envPath,
		Source: func(opts providerkit.SelectOpts) (providerkit.Source, error) {
			sel, err := envkit.SelectFile(envPath, opts)
			if err != nil {
				return nil, providerkit.Fail(printer, outfmt.CodeIO, "%v", err)
			}
			return sel.Flat(), nil
		},
		Runner:      gh, // providerkit.ExecRunner{} in production
		Interactive: func() bool { return false },
		Ask:         func(string) bool { return false },
	}))
	return root
}

// execute prints the command line, runs it and returns the host exit code.
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
		// Only cobra's own parse errors (unknown flag, wrong arguments)
		// arrive without a CmdError; a host reports them as usage.
		fmt.Fprintf(w, "%s: %v\n", programName, err)
		return exitUsage
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
	gh := &fakeGh{w: w, remote: []string{"API_KEY", "DB_URL", "GHCR_PAT", "OLD_TOKEN", "VPS_DEPLOY_KEY"}}
	for _, args := range scenarios() {
		fmt.Fprintf(w, "exit %d\n\n", execute(w, mount(w, envPath, gh), args...))
	}
	return tokenOverride(w, mount(w, envPath, gh))
}

// scenarios are the command lines the guide walks through, in order.
//
//ds:def id=scenarios-vmqccpt9
func scenarios() [][]string {
	const sel = "--prefix=GITHUB_SECRET_"
	return [][]string{
		// 1. preview: target from the cwd's git remote, nothing written
		{"github", "push", sel, "--strip-prefix", "--dry-run"},
		// 2. CI shape: explicit repo and environment, --yes, one envelope
		{"--json", "github", "push", sel, "--strip-prefix", "--repo", "acme/api", "--env", "prod", "--yes"},
		// 3. environment secrets need the environment to exist first
		{"github", "env-create", "prod", "--repo", "acme/api", "--yes"},
		// 4. names only; each scope is listed on its own
		{"github", "list", "--repo", "acme/api", "--env", "prod"},
		// 5. prune preview, sparing a secret another pipeline manages
		{"github", "prune", sel, "--strip-prefix", "--repo", "acme/api", "--keep", "VPS_DEPLOY_KEY", "--dry-run"},
		// 6. --org cannot be combined with --repo: usage error before any write
		{"github", "push", "--org", "acme", "--repo", "acme/api", "--yes"},
		// 7. LOG_LEVEL is a legal secret name, app.name is not: nothing is written
		{"github", "push", "LOG_LEVEL", "app.name", "--repo", "acme/api", "--yes"},
	}
}

// tokenOverride shows the auth label. gh reads GH_TOKEN itself; the plugin
// only checks whether it is set, to print "via GH_TOKEN" in the banner so an
// exported token never acts unnoticed. The fake runner never sees the token.
//
//ds:def id=tokenoverride-2yhnvfrs
func tokenOverride(w io.Writer, root *cobra.Command) error {
	prev, had := os.LookupEnv(tokenVar)
	if err := os.Setenv(tokenVar, "example-token-not-real"); err != nil {
		return err
	}
	defer func() {
		if had {
			_ = os.Setenv(tokenVar, prev)
		} else {
			_ = os.Unsetenv(tokenVar)
		}
	}()
	fmt.Fprintf(w, "# with %s set in the environment\n", tokenVar)
	fmt.Fprintf(w, "exit %d\n", execute(w, root, "github", "push", "DB_HOST", "--repo", "acme/api", "--dry-run"))
	return nil
}
