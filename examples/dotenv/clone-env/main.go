// Command clone-env derives .env.prod and .env.qa from .env.staging: it clones
// the loaded file, changes only what differs, and writes each variant with
// SaveAs. Every ${reference} stays a reference, so derived values keep
// following their source. It also shows what goes wrong when a file is built
// from ExpandedMap instead: every reference is baked into a fixed copy.
//
// Run it with `go run .`; it works in a fresh temp directory.
package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

	"github.com/ubgo/textdata/dotenv"
)

// File names of the source and the derived environments.
const (
	stagingFile = ".env.staging"
	prodFile    = ".env.prod"
	qaFile      = ".env.qa"
	bakedFile   = ".env.baked"
)

// staging is the source environment: shared settings, and derived values that
// must stay references to DOMAIN.
const staging = `# shared
DOMAIN=staging.acme.io
LOG_LEVEL=debug

# derived: keep these as references
API_URL=https://api.${DOMAIN}
CDN_URL=https://cdn.${DOMAIN}
`

func main() {
	dir, err := os.MkdirTemp("", "dotenv-clone-env")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	err = run(os.Stdout, dir)
	// Cleanup is best-effort: the run's result matters more than a leftover
	// temp directory.
	_ = os.RemoveAll(dir)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// run writes the staging file, derives the variants, and prints each file and
// its expanded API_URL so the test can pin the whole output.
func run(w io.Writer, dir string) error {
	src := filepath.Join(dir, stagingFile)
	if err := os.WriteFile(src, []byte(staging), 0o600); err != nil {
		return err
	}
	if err := deriveVariants(src); err != nil {
		return err
	}
	if err := bake(src, filepath.Join(dir, bakedFile)); err != nil {
		return err
	}
	for _, name := range []string{stagingFile, prodFile, qaFile, bakedFile} {
		if err := printFile(w, filepath.Join(dir, name)); err != nil {
			return err
		}
	}
	return nil
}

// deriveVariants opens staging once and writes two variants from it. Each
// Clone is independent of the source and of the other clone; SaveAs writes
// elsewhere and leaves the clone's Path (still the staging path) alone.
//
//ds:def id=derivevariants-t6qx2n8d
func deriveVariants(src string) error {
	base, err := dotenv.Open(src)
	if err != nil {
		return err
	}
	dir := filepath.Dir(src)

	prod := base.Clone()
	prod.Set("DOMAIN", "acme.io")
	prod.Set("LOG_LEVEL", "warn")
	if err := prod.SaveAs(filepath.Join(dir, prodFile)); err != nil {
		return err
	}

	qa := base.Clone()
	qa.Set("DOMAIN", "qa.acme.io")
	return qa.SaveAs(filepath.Join(dir, qaFile))
}

// bake is the mistake to avoid: building a file from ExpandedMap writes each
// reference's current result, so API_URL no longer follows DOMAIN.
//
//ds:def id=bake-p5kc8v3w
func bake(src, dst string) error {
	f, err := dotenv.Open(src)
	if err != nil {
		return err
	}
	m, err := f.ExpandedMap()
	if err != nil {
		return err
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := dotenv.Parse("")
	for _, k := range keys {
		out.Append(dotenv.NewPair(k, m[k]))
	}
	out.Set("DOMAIN", "acme.io")
	return out.SaveAs(dst)
}

// printFile prints a file's bytes and what API_URL expands to in it.
func printFile(w io.Writer, path string) error {
	f, err := dotenv.Open(path)
	if err != nil {
		return err
	}
	api, _, err := f.GetExpanded("API_URL")
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "== %s (API_URL -> %s)\n%s", filepath.Base(path), api, f.Render())
	return nil
}
