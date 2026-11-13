// Command envkit-ops drives the operations API behind dotenvctl
// (github.com/ubgo/textdata/dotenv/cli/envkit) directly from Go: discover a
// .env family, pick a selection of keys, edit with a dry-run preview, compare
// two files, build the drift matrix against a contract, and build a child
// process environment. Every call works on files in a temp directory.
package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/ubgo/textdata/dotenv"
	"github.com/ubgo/textdata/dotenv/cli/envkit"
)

// envFileMode is 0600 because env files hold credentials; envkit.Set writes
// new files with the same mode.
const envFileMode = 0o600

// File names of the demo's .env family. The contract (.env.example) lists
// every key a deploy must define; it is never a matrix column.
const (
	stagFile     = ".env.stag"
	prodFile     = ".env.prod"
	contractFile = ".env.example"
)

// family is the demo directory's content, name → body.
var family = map[string]string{
	contractFile: "DB_URL=\nAPI_KEY=\nSENTRY_DSN=\n",
	stagFile:     "HOST=db.stag\nDB_URL=postgres://${HOST}/app\nAPI_KEY=__FILL_ME__\n# SENTRY_DSN=x\n",
	prodFile:     "HOST=db.prod\nDB_URL=postgres://${HOST}/app\nAPI_KEY=k-123\nDEPLOY_TOKEN=t-1\nDEPLOY_REGION=eu\n",
}

func main() {
	dir, err := os.MkdirTemp("", "envkit-ops-")
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

// run executes every step of the guide in dir.
// The caller owns dir (main makes a temp one, the test passes t.TempDir()),
// so nothing is written outside it.
func run(w io.Writer, dir string) error {
	for name, body := range family {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), envFileMode); err != nil {
			return err
		}
	}
	steps := []func(io.Writer, string) error{
		discoverFamily, selectDeployKeys, editWithPreview, diffFiles, driftMatrix, childEnvironment,
	}
	for _, step := range steps {
		if err := step(w, dir); err != nil {
			return err
		}
	}
	return nil
}

// discoverFamily lists the .env files in dir in distance-from-prod order with
// a per-file summary. Contracts are tagged, not hidden.
//
//ds:def id=discoverfamily-mb5kcafy
func discoverFamily(w io.Writer, dir string) error {
	inv, err := envkit.Inventories(envkit.InventoryOptions{Dir: dir})
	if err != nil {
		return err
	}
	for _, i := range inv {
		fmt.Fprintf(w, "%-12s env=%-7s contract=%-5t keys=%d placeholders=%d\n",
			i.File, i.Env, i.Contract, i.Keys, i.Placeholders)
	}
	return nil
}

// selectDeployKeys picks the keys a deploy step would push: one prefix,
// renamed without it, with ${references} resolved. A key you name that does
// not exist is an error, never a silent skip.
//
//ds:def id=selectdeploykeys-6ztzpzuk
func selectDeployKeys(w io.Writer, dir string) error {
	sel, err := envkit.SelectFile(filepath.Join(dir, prodFile), envkit.SelectOptions{
		Prefix:      "DEPLOY_",
		StripPrefix: true,
		IncludeKeys: []string{"DB_URL"},
		Expand:      true,
	})
	if err != nil {
		return err
	}
	for _, p := range sel.Pairs {
		fmt.Fprintf(w, "select %s=%s\n", p.Key, p.Value)
	}
	_, err = envkit.SelectFile(filepath.Join(dir, prodFile), envkit.SelectOptions{Keys: []string{"NOPE"}})
	fmt.Fprintln(w, "named key missing:", err)
	return nil
}

// editWithPreview previews a Set with DryRun, then writes it next to an
// anchor, then comments a key out and brings it back. DryRun and the real
// call share one code path, so the preview cannot disagree with the write.
//
//ds:def id=editwithpreview-xu4xtfku
func editWithPreview(w io.Writer, dir string) error {
	path := filepath.Join(dir, stagFile)
	pairs := []dotenv.Pair{{Key: "SENTRY_ENV", Value: "staging"}}
	preview, err := envkit.Set(path, pairs, envkit.EditOptions{Anchor: "API_KEY", DryRun: true})
	if err != nil {
		return err
	}
	fmt.Fprintln(w, "preview:", preview.Diff, "written:", preview.Written)
	if _, err := envkit.Set(path, pairs, envkit.EditOptions{Anchor: "API_KEY"}); err != nil {
		return err
	}
	if _, err := envkit.Unset(path, []string{"SENTRY_ENV"}, envkit.EditOptions{}); err != nil {
		return err
	}
	res, err := envkit.Restore(path, []string{"SENTRY_ENV"}, envkit.EditOptions{})
	if err != nil {
		return err
	}
	fmt.Fprintln(w, "restore:", res.Diff)
	_, err = envkit.Unset(path, []string{"NEVER"}, envkit.EditOptions{})
	fmt.Fprintln(w, "unset absent key is ErrKeyNotFound:", errors.Is(err, envkit.ErrKeyNotFound))
	return nil
}

// diffFiles compares the effective (last-wins, expanded) view of two files.
// Comments and formatting are invisible on purpose.
//
//ds:def id=difffiles-d9y2f3t7
func diffFiles(w io.Writer, dir string) error {
	d, err := envkit.Diff(filepath.Join(dir, stagFile), filepath.Join(dir, prodFile), envkit.DiffOptions{Expand: true})
	if err != nil {
		return err
	}
	for _, p := range d.Added {
		fmt.Fprintln(w, "diff +", p.Key)
	}
	for _, p := range d.Removed {
		fmt.Fprintln(w, "diff -", p.Key)
	}
	for _, c := range d.Changed {
		fmt.Fprintf(w, "diff ~ %s: %s -> %s\n", c.Key, c.From, c.To)
	}
	return nil
}

// driftMatrix compares every non-contract file key by key and checks each
// against the contract. Values stay out of the result unless Reveal is set.
//
//ds:def id=driftmatrix-anaguvzr
func driftMatrix(w io.Writer, dir string) error {
	m, err := envkit.BuildMatrix(envkit.MatrixOptions{
		Dir:          dir,
		ContractPath: filepath.Join(dir, contractFile),
	})
	if err != nil {
		return err
	}
	m.Rows = envkit.DriftRows(m.Rows)
	fmt.Fprintln(w, "columns:", m.Envs)
	for _, row := range m.Rows {
		cells := make([]string, 0, len(row.Cells))
		for _, c := range row.Cells {
			cells = append(cells, c.Env+"="+string(c.State))
		}
		fmt.Fprintf(w, "drift %-13s %s\n", row.Key, strings.Join(cells, " "))
	}
	fmt.Fprintln(w, "contract gaps:", m.HasContractGaps(), m.ContractMissing["stag"])
	return nil
}

// childEnvironment builds the environment a child process would get: the
// base overlaid with the file's expanded values, the file winning.
//
//ds:def id=childenvironment-5m6hvqgp
func childEnvironment(w io.Writer, dir string) error {
	base := []string{"HOST=from-shell", "TERM=xterm"}
	env, err := envkit.ChildEnv(filepath.Join(dir, prodFile), base, nil)
	if err != nil {
		return err
	}
	slices.Sort(env)
	fmt.Fprintln(w, "child env:", env)
	return nil
}
