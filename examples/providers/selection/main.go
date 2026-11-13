// Command selection walks the selection grammar every textdata CLI and push
// provider shares (clikit/selection), applied to a real .env file by dotenv's
// operations package (envkit.SelectFile) — the same call dotenvctl makes
// before `github push` or `vercel push`.
//
// It writes one .env into a temp directory and resolves it with a different
// selection.Options per step, printing what would be acted on (pairs) and
// what a guard held back (skipped). Nothing leaves the process.
package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/ubgo/textdata/clikit/selection"
	"github.com/ubgo/textdata/dotenv/cli/envkit"
)

// envFileName is the file every step selects from.
const envFileName = ".env"

// envFileMode is 0600 because env files hold credentials.
const envFileMode = 0o600

// deployPrefix marks the keys meant for the CI secret store; the steps below
// select, rename and filter on it.
const deployPrefix = "GITHUB_SECRET_"

// envContent mixes deploy secrets, app config, a placeholder, an empty value
// and a ${reference}, so each guard and each filter has something to act on.
const envContent = `# deploy secrets (pushed to CI)
GITHUB_SECRET_DB_URL=postgres://${DB_HOST}/app
GITHUB_SECRET_API_KEY=__YOU__
GITHUB_SECRET_GITHUB_TOKEN=ghp_local_only
GITHUB_SECRET_SENTRY_DSN=
# app config
DB_HOST=db.internal
DEPLOY_REGION=eu-west-1
LOG_LEVEL=debug
`

func main() {
	dir, err := os.MkdirTemp("", "selection-example-")
	if err == nil {
		err = run(os.Stdout, dir)
		// Cleanup is best-effort: the run's result matters more than a
		// leftover temp directory.
		_ = os.RemoveAll(dir)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "selection:", err)
		os.Exit(1)
	}
}

// step names one scenario: a title and the options it resolves with.
type step struct {
	title string
	opts  selection.Options
}

// run writes the .env and prints each step's resolved selection.
func run(w io.Writer, dir string) error {
	path := filepath.Join(dir, envFileName)
	if err := os.WriteFile(path, []byte(envContent), envFileMode); err != nil {
		return err
	}

	steps := []step{
		{"1. every key, expanded, guards on", everyKey()},
		{"2. only deploy secrets, prefix stripped", deploySecrets()},
		{"3. force-include one app key", forceInclude()},
		{"4. lift both guards", liftGuards()},
		{"5. no expansion", noExpand()},
		{"6. a custom placeholder pattern", customPlaceholder()},
	}
	for _, s := range steps {
		if err := show(w, path, s); err != nil {
			return err
		}
	}
	if err := namedKeyMustExist(w, path); err != nil {
		return err
	}
	return badPattern(w)
}

// show resolves one step and prints pairs and skips in resolution order.
func show(w io.Writer, path string, s step) error {
	sel, err := envkit.SelectFile(path, s.opts)
	if err != nil {
		return fmt.Errorf("%s: %w", s.title, err)
	}
	fmt.Fprintln(w, s.title)
	for _, p := range sel.Pairs {
		fmt.Fprintf(w, "  pair    %s=%s\n", p.Key, p.Value)
	}
	for _, sk := range sel.Skipped {
		fmt.Fprintf(w, "  skipped %s (%s)\n", sk.Name, sk.Reason)
	}
	return nil
}

// everyKey is the push default: every active key, ${references} resolved,
// placeholder and empty values held back. Expand is set by hand here; the
// push and prune verbs turn it on for you (--expand defaults to true there).
//
//ds:def id=everykey-u4bhavam
func everyKey() selection.Options {
	return selection.Options{Expand: true}
}

// deploySecrets keeps only the prefixed keys, drops one by its ORIGINAL
// name (ExcludeKeys matches before renaming), and strips the prefix so the
// destination sees DB_URL, not GITHUB_SECRET_DB_URL.
//
//ds:def id=deploysecrets-xdm9r9ay
func deploySecrets() selection.Options {
	return selection.Options{
		Prefix:      deployPrefix,
		StripPrefix: true,
		ExcludeKeys: []string{deployPrefix + "GITHUB_TOKEN"},
		Expand:      true,
	}
}

// forceInclude adds a key the prefix filter would drop. It is appended after
// the prefixed keys and keeps its own name, because it never had the prefix.
//
//ds:def id=forceinclude-4zqem3f9
func forceInclude() selection.Options {
	return selection.Options{
		Prefix:      deployPrefix,
		StripPrefix: true,
		IncludeKeys: []string{"DEPLOY_REGION"},
		ExcludeKeys: []string{deployPrefix + "GITHUB_TOKEN"},
		Expand:      true,
	}
}

// liftGuards turns both guards off: the placeholder and the empty value are
// selected like any other value (--include-placeholders, --include-empty).
//
//ds:def id=liftguards-bt8rqkzf
func liftGuards() selection.Options {
	return selection.Options{
		Prefix:              deployPrefix,
		StripPrefix:         true,
		IncludePlaceholders: true,
		IncludeEmpty:        true,
		Expand:              true,
	}
}

// noExpand keeps ${DB_HOST} literal, which is what a reader wants and almost
// never what a secret store wants.
//
//ds:def id=noexpand-vqkqqcs7
func noExpand() selection.Options {
	return selection.Options{Keys: []string{deployPrefix + "DB_URL"}}
}

// customPlaceholder swaps the __LIKE_THIS__ convention for another one. It
// replaces the default rather than adding to it, so __YOU__ is now selected
// as an ordinary value.
//
//ds:def id=customplaceholder-5ddbcpnr
func customPlaceholder() selection.Options {
	return selection.Options{
		Prefix:             deployPrefix,
		Expand:             true,
		PlaceholderPattern: `^ghp_local_only$`,
	}
}

// namedKeyMustExist shows the grammar's one invariant: a key the user named
// (Keys or IncludeKeys) that is not in the file is an error, never a silent
// skip. Excluding an absent key is fine.
//
//ds:def id=namedkeymustexist-db8wseyh
func namedKeyMustExist(w io.Writer, path string) error {
	_, err := envkit.SelectFile(path, selection.Options{Keys: []string{"MISSING"}})
	fmt.Fprintln(w, "7. a named key that does not exist")
	fmt.Fprintf(w, "  error: %v\n", err)

	_, err = envkit.SelectFile(path, selection.Options{ExcludeKeys: []string{"MISSING"}})
	fmt.Fprintf(w, "  excluding it instead: err=%v\n", err)
	return nil
}

// badPattern validates a user-supplied pattern up front. PlaceholderMatcher
// returns the shared compiled default for "" and an error, never a panic, for
// a pattern that does not compile.
//
//ds:def id=badpattern-w3rkhehe
func badPattern(w io.Writer) error {
	re, err := selection.PlaceholderMatcher("")
	if err != nil {
		return err
	}
	fmt.Fprintln(w, "8. checking a pattern before use")
	fmt.Fprintf(w, "  default %s matches __YOU__: %t\n", re, re.MatchString("__YOU__"))

	_, err = selection.PlaceholderMatcher("__(")
	fmt.Fprintf(w, "  bad pattern: %v\n", err)
	return nil
}
