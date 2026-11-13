// Command reading shows the four ways into a .env file (Read, Open, Parse,
// ParseReader), what each read accessor returns for duplicates, disabled
// settings and inherited names, and where an opened file came from.
//
// Run it with `go run .`; it works in a fresh temp directory and prints each
// step to stdout.
package main

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ubgo/textdata/dotenv"
)

// envFile is the file every step writes inside the run's own directory.
const envFile = ".env"

// sharedMode is a deliberately wider mode than the 0600 default, to show that
// Open records the mode an existing file already has.
const sharedMode fs.FileMode = 0o640

// sample has one of everything the read accessors treat specially: DB_HOST is
// set twice, DB_USER is commented out, HOME is a name with no value.
const sample = `# database
DB_HOST=localhost
DB_PORT=5432
# DB_USER=admin
DB_HOST=db.internal
HOME
`

func main() {
	dir, err := os.MkdirTemp("", "dotenv-reading")
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

// run prints every step under a heading so the test can pin the whole output.
func run(w io.Writer, dir string) error {
	path := filepath.Join(dir, envFile)
	content := "DOMAIN=example.com\nAPI_URL=https://api.${DOMAIN}\n# DEBUG=true\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		return err
	}
	fmt.Fprintln(w, "== Read")
	if err := readValues(w, path); err != nil {
		return err
	}
	fmt.Fprintln(w, "== Open")
	if err := openForEdit(w, path); err != nil {
		return err
	}
	fmt.Fprintln(w, "== Parse and ParseReader")
	if err := parseContent(w); err != nil {
		return err
	}
	fmt.Fprintln(w, "== accessors")
	queryFile(w, dotenv.Parse(sample))
	fmt.Fprintln(w, "== Existed, Mode, Path")
	return whereFrom(w, dir)
}

// readValues is the one-call path for a program that only consumes
// configuration: every ${reference} resolved, comments and disabled settings
// skipped. A missing file is an error, because Read is for a file that must
// exist.
//
//ds:def id=readvalues-fe8hnc8t
func readValues(w io.Writer, path string) error {
	m, err := dotenv.Read(path)
	if err != nil {
		return err
	}
	// A map has no order; sort the keys so the output is stable.
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Fprintf(w, "%s=%s\n", k, m[k])
	}

	_, err = dotenv.Read(filepath.Join(filepath.Dir(path), "missing.env"))
	fmt.Fprintln(w, "missing file is an error:", err != nil)
	return nil
}

// openForEdit opens a file you may change and save. Values come back raw, so
// a ${reference} survives a save. A missing file is not an error: you get an
// empty File, and Save creates it.
//
//ds:def id=openforedit-7y5rgd8w
func openForEdit(w io.Writer, path string) error {
	f, err := dotenv.Open(path)
	if err != nil {
		return err
	}
	v, ok := f.Get("API_URL")
	fmt.Fprintln(w, v, ok, "existed:", f.Existed())

	fresh, err := dotenv.Open(filepath.Join(filepath.Dir(path), ".env.new"))
	if err != nil {
		return err
	}
	fmt.Fprintln(w, "missing file existed:", fresh.Existed(), "keys:", len(fresh.Keys()))
	fresh.Set("PORT", "8080")
	if err := fresh.Save(); err != nil {
		return err
	}
	data, err := os.ReadFile(fresh.Path)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "created: %q\n", data)
	return nil
}

// parseContent reads content already in memory. Parse never fails and gives
// the exact input back from Render; ParseReader copies a reader once, which is
// cheaper than io.ReadAll followed by Parse.
//
//ds:def id=parsecontent-p3m6q2zk
func parseContent(w io.Writer) error {
	const content = "# app\nPORT=8080\nexport NAME='my app'\n"
	f := dotenv.Parse(content)
	name, _ := f.Get("NAME")
	fmt.Fprintln(w, "NAME:", name)
	fmt.Fprintln(w, "round trip:", f.Render() == content)

	r, err := dotenv.ParseReader(strings.NewReader("PORT=8080\nHOST=localhost\n"))
	if err != nil {
		return err
	}
	fmt.Fprintln(w, "keys:", r.Keys())
	return nil
}

// queryFile runs all six accessors over sample. They differ only in how they
// treat a duplicated key; all six skip the disabled DB_USER and the inherited
// HOME, because neither is an active setting.
//
//ds:def id=queryfile-r9kd2v6c
func queryFile(w io.Writer, f *dotenv.File) {
	for _, key := range []string{"DB_HOST", "DB_USER", "HOME"} {
		v, ok := f.Get(key)
		fmt.Fprintf(w, "Get(%s) = %q %v, Has = %v, Count = %d\n", key, v, ok, f.Has(key), f.Count(key))
	}
	fmt.Fprintln(w, "Keys:", f.Keys())
	for _, p := range f.Pairs() {
		fmt.Fprintf(w, "Pairs: %s=%s\n", p.Key, p.Value)
	}
	m := f.Map()
	fmt.Fprintln(w, "Map:", len(m), "keys, DB_HOST =", m["DB_HOST"])
	for _, key := range f.Keys() {
		if n := f.Count(key); n > 1 {
			fmt.Fprintf(w, "duplicate: %s is set %d times\n", key, n)
		}
	}
}

// whereFrom shows the three facts that describe a file's place on disk:
// whether it existed, the mode Save will apply, and the Path Save writes to.
//
//ds:def id=wherefrom-h2xw7b4n
func whereFrom(w io.Writer, dir string) error {
	path := filepath.Join(dir, ".env.shared")
	if err := os.WriteFile(path, []byte("A=1\n"), sharedMode); err != nil {
		return err
	}
	// WriteFile's mode is filtered by the umask; set it exactly.
	if err := os.Chmod(path, sharedMode); err != nil {
		return err
	}
	existing, err := dotenv.Open(path)
	if err != nil {
		return err
	}
	missing, err := dotenv.Open(filepath.Join(dir, ".env.missing"))
	if err != nil {
		return err
	}
	parsed := dotenv.Parse("A=1\n")

	fmt.Fprintln(w, "existing:", existing.Existed(), existing.Mode(), filepath.Base(existing.Path))
	fmt.Fprintln(w, "missing: ", missing.Existed(), missing.Mode(), filepath.Base(missing.Path))
	fmt.Fprintf(w, "parsed:   %v %v %q\n", parsed.Existed(), parsed.Mode(), parsed.Path)
	return nil
}
