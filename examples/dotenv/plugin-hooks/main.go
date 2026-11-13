// Command plugin-hooks installs every dotenv hook that changes how a file
// reads: the function options (WithLookup, WithValueTransform, WithSaveGuard),
// a Resolver for ${scheme:ref}, an EntryObserver, an ExpandObserver, and shows
// how several plugins combine and that no hook can change the saved bytes.
//
// Run it with `go run .`; every store is an in-memory map.
package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ubgo/textdata/dotenv"
	"github.com/ubgo/textdata/plugin"
)

// cipherPrefix marks a value the transform must decrypt.
const cipherPrefix = "encrypted:"

// memScheme is the ${scheme:ref} scheme the example Resolver owns.
const memScheme = "mem"

func main() {
	dir, err := os.MkdirTemp("", "dotenv-firewall")
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

// step names one stage of the walk-through and the function that prints it.
type step struct {
	title string
	show  func(io.Writer) error
}

// run prints every step under a heading so the test can pin the whole output.
func run(w io.Writer, dir string) error {
	steps := []step{
		{"function options", functionOptions},
		{"Resolver", resolver},
		{"observers", observers},
		{"ordering", ordering},
		{"firewall", func(w io.Writer) error { return firewall(w, dir) }},
	}
	for _, s := range steps {
		fmt.Fprintln(w, "==", s.title)
		if err := s.show(w); err != nil {
			return err
		}
	}
	return nil
}

// functionOptions installs three one-function hooks. The lookup fills only
// what the file does not define; the transform decrypts after expansion (also
// on single-quoted values); the guard may refuse a save. Get still returns
// the ciphertext, which is what would be saved.
//
//ds:def id=functionoptions-u5cr9k2w
func functionOptions(w io.Writer) error {
	env := map[string]string{"REGION": "eu-west-1", "ZONE": "ignored"}
	keys := map[string]string{"c2VjcmV0": "hunter2"}
	f := dotenv.Parse("ZONE=${REGION}-a\nPASS='encrypted:c2VjcmV0'\n",
		dotenv.WithLookup(func(name string) (string, bool) {
			v, ok := env[name]
			return v, ok
		}),
		dotenv.WithValueTransform(func(key, v string) (string, error) {
			id, ok := strings.CutPrefix(v, cipherPrefix)
			if !ok {
				return v, nil
			}
			if plain, found := keys[id]; found {
				return plain, nil
			}
			return "", fmt.Errorf("%s: no key for %q", key, id)
		}),
		dotenv.WithSaveGuard(func(*dotenv.File) error { return errors.New("read-only") }),
	)
	zone, _, err := f.GetExpanded("ZONE")
	if err != nil {
		return err
	}
	// PASS's error is printed, not returned: a nil error here is the point.
	pass, _, passErr := f.GetExpanded("PASS")
	raw, _ := f.Get("PASS")
	fmt.Fprintln(w, "ZONE:", zone, "PASS:", pass, "Get(PASS):", raw, "err:", passErr)
	f.Path = "never-written.env" // the guard refuses before any write
	fmt.Fprintln(w, "save:", f.Save())
	return nil
}

// memStore resolves ${mem:ref} from a map. A scheme it does not own never
// reaches it; a missing ref is an error, never an empty value.
type memStore map[string]string

func (memStore) Name() string      { return "mem-store" }
func (memStore) Schemes() []string { return []string{memScheme} }

//ds:def id=memstoreresolve-h5nx8q2w
func (m memStore) Resolve(scheme, ref string) (string, error) {
	if v, ok := m[ref]; ok {
		return v, nil
	}
	return "", fmt.Errorf("%w: %s:%s", plugin.ErrNotFound, scheme, ref)
}

var _ dotenv.Resolver = memStore(nil)

// resolver installs memStore. ${mem:...} reads as the stored value; another
// scheme reads as an unset name; a single-quoted reference stays literal.
//
//ds:def id=resolver-g8wn2v5j
func resolver(w io.Writer) error {
	store := memStore{"db/password": "pa55"}
	src := "PASS=${mem:db/password}\nOTHER=[${vault:x}]\nLIT='${mem:db/password}'\nBAD=${mem:nope}\n"
	f := dotenv.Parse(src, dotenv.WithPlugin(store))
	for _, key := range []string{"PASS", "OTHER", "LIT"} {
		v, _, err := f.GetExpanded(key)
		if err != nil {
			return err
		}
		fmt.Fprintf(w, "%s=%s\n", key, v)
	}
	_, _, err := f.GetExpanded("BAD")
	fmt.Fprintln(w, err)
	return nil
}

// auditor is an EntryObserver and an ExpandObserver: it records disabled
// settings while parsing and which variables are referenced while expanding.
type auditor struct {
	disabled []string
	used     map[string]bool
}

var (
	_ dotenv.EntryObserver  = (*auditor)(nil)
	_ dotenv.ExpandObserver = (*auditor)(nil)
)

func (a *auditor) Name() string { return "auditor" }

//ds:def id=auditorobserveentry-w2gk6t4c
func (a *auditor) ObserveEntry(e *dotenv.Entry) {
	if e.Kind == dotenv.KindDisabledPair {
		a.disabled = append(a.disabled, e.Key)
	}
}

func (a *auditor) ObserveExpand(key, name, resolved string) { a.used[name] = true }

// observers runs the auditor: entry events fire while parsing, expand events
// only once something is expanded, so ExpandedMap is called first. A key no
// value references is a dead-key candidate.
//
//ds:def id=observers-d3qp9w6r
func observers(w io.Writer) error {
	a := &auditor{used: map[string]bool{}}
	f := dotenv.Parse("HOST=db\nPORT=5432\nURL=pg://${HOST}\n# DEBUG=true\n", dotenv.WithPlugin(a))
	if _, err := f.ExpandedMap(); err != nil {
		return err
	}
	var unused []string
	for _, key := range f.Keys() {
		if !a.used[key] {
			unused = append(unused, key)
		}
	}
	sort.Strings(unused)
	fmt.Fprintln(w, "disabled:", a.disabled, "never referenced:", unused)
	return nil
}

// ordering installs two of each chained or first-wins hook. Lookupers: first
// to answer wins. Transformers: chained in install order. Command runners:
// the last installed replaces the others.
//
//ds:def id=ordering-y2kc6f9t
func ordering(w io.Writer) error {
	look := func(answer string) func(string) (string, bool) {
		return func(string) (string, bool) { return answer, answer != "" }
	}
	wrap := func(tag string) func(string, string) (string, error) {
		return func(_, v string) (string, error) { return tag + "(" + v + ")", nil }
	}
	runner := func(tag string) func(string) (string, error) {
		return func(string) (string, error) { return tag, nil }
	}
	f := dotenv.Parse("A=${X}\nB=$(cmd)\n",
		dotenv.WithLookup(look("")), dotenv.WithLookup(look("second")), dotenv.WithLookup(look("third")),
		dotenv.WithValueTransform(wrap("t1")), dotenv.WithValueTransform(wrap("t2")),
		dotenv.WithCommandRunner(runner("first-runner")), dotenv.WithCommandRunner(runner("last-runner")),
	)
	m, err := f.ExpandedMap()
	if err != nil {
		return err
	}
	fmt.Fprintln(w, "A:", m["A"], "B:", m["B"])
	return nil
}

// meddler tries to change what is written: its guard calls Set on the file it
// is handed, and its observer rewrites each entry. Both get copies.
type meddler struct{}

func (meddler) Name() string                               { return "meddler" }
func (meddler) GuardSave(f *dotenv.File) error             { f.Set("A", "changed"); return nil }
func (meddler) ObserveEntry(e *dotenv.Entry)               { e.Value, e.Raw = "x", []string{"x"} }
func (meddler) TransformValue(_, v string) (string, error) { return strings.ToUpper(v), nil }

// firewall shows the rule every hook obeys: a hook may change what a value
// reads as, never what is written. Render and the saved bytes match the
// input whatever is installed.
//
//ds:def id=firewall-v7bn4h2m
func firewall(w io.Writer, dir string) error {
	const src = "A=secret\n# B=off\n"
	f := dotenv.Parse(src, dotenv.WithPlugin(meddler{}))
	read, _, err := f.GetExpanded("A")
	if err != nil {
		return err
	}
	f.Path = filepath.Join(dir, ".env")
	if err := f.Save(); err != nil {
		return err
	}
	saved, err := os.ReadFile(f.Path)
	if err != nil {
		return err
	}
	fmt.Fprintln(w, "reads as:", read, "render unchanged:", f.Render() == src, "saved unchanged:", string(saved) == src)
	return nil
}
