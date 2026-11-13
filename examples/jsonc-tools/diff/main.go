// Command diff compares two JSONC configs by meaning: which values were
// added, removed or changed, at which path and at which line and column of
// each file. Comments, layout, key order and number spelling do not count.
//
// Run it with `go run .`; it prints each stage to stdout.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/ubgo/textdata/jsonc"
	"github.com/ubgo/textdata/jsonc/diff"
)

// File names label positions; nothing is read from disk.
const (
	baseName = "base.jsonc"
	prodName = "prod.jsonc"
)

// base is the checked-in default config.
const base = `{
  // shared defaults
  "server": {"port": 8080, "host": "0.0.0.0"},
  "debug": true,
  "replicas": 1.0,
  "hosts": ["a.example", "b.example"],
  "log.level": "info",
}`

// prod reorders keys, drops the comments and spells 1.0 as 1: none of that
// is a change. The port, debug flag and hosts list really differ.
const prod = `{
  "replicas": 1,
  "server": {"host": "0.0.0.0", "port": 443, "tls": true},
  "hosts": ["z.example", "a.example", "b.example"],
  "log.level": "warn"
}`

// sameAgain is base with another layout and a different comment only.
const sameAgain = `{"debug": true, "replicas": 1, /* moved */ "hosts": ["a.example", "b.example"],
  "server": {"host": "0.0.0.0", "port": 8080}, "log.level": "info"}`

func main() {
	if err := run(os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// run prints every stage under a heading so the test can pin the output.
func run(w io.Writer) error {
	a, err := jsonc.Parse([]byte(base), jsonc.WithFileName(baseName))
	if err != nil {
		return err
	}
	b, err := jsonc.Parse([]byte(prod), jsonc.WithFileName(prodName))
	if err != nil {
		return err
	}
	fmt.Fprintln(w, "== base -> prod")
	if err := printChanges(w, a, b); err != nil {
		return err
	}

	fmt.Fprintln(w, "== by kind")
	if err := summarize(w, a, b); err != nil {
		return err
	}

	c, err := jsonc.Parse([]byte(sameAgain))
	if err != nil {
		return err
	}
	n, err := countChanges(a, c)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "== base -> same meaning, new layout\nchanges: %d\n", n)

	fmt.Fprintln(w, "== Equal")
	compareValues(w)
	return nil
}

// printChanges lists every difference with its kind, dotted path, old and
// new value, and the line:col of the value in each file ("-" on the side
// that has none).
//
//ds:def id=jt-diff-print-th7pw4gm
func printChanges(w io.Writer, a, b *jsonc.Doc) error {
	changes, err := diff.Diff(a, b)
	if err != nil {
		return err // a document with syntax errors is refused
	}
	for _, c := range changes {
		fmt.Fprintf(w, "%-7s %-13s %v -> %v (%s %s, %s %s)\n",
			c.Kind, c.Path, c.Old, c.New, baseName, c.OldPos, prodName, c.NewPos)
	}
	return nil
}

// kindOrder is the order the summary lists the closed set of kinds in.
var kindOrder = []diff.Kind{diff.Added, diff.Removed, diff.Changed}

// summarize groups the changes by Kind and prints each path twice: the
// dotted Path for people and the JSON Pointer, the unambiguous form to pass
// back to jsonc.Doc methods (a key containing a dot is quoted in Path).
//
//ds:def id=jt-diff-kinds-bxuyq8qf
func summarize(w io.Writer, a, b *jsonc.Doc) error {
	changes, err := diff.Diff(a, b)
	if err != nil {
		return err
	}
	byKind := map[diff.Kind][]diff.Change{}
	for _, c := range changes {
		byKind[c.Kind] = append(byKind[c.Kind], c)
	}
	for _, k := range kindOrder {
		fmt.Fprintf(w, "%s: %d\n", k, len(byKind[k]))
		for _, c := range byKind[k] {
			fmt.Fprintf(w, "  %-13s %s\n", c.Path, c.Pointer)
		}
	}
	return nil
}

// countChanges is the "does the config differ at all" check: an empty slice
// means the two documents mean the same thing.
//
//ds:def id=jt-diff-count-y4exrkhe
func countChanges(a, b *jsonc.Doc) (int, error) {
	changes, err := diff.Diff(a, b)
	return len(changes), err
}

// compareValues uses the rule Diff uses on values decoded outside a
// document (encoding/json with UseNumber): numbers by value, integers
// exactly at any size, objects regardless of key order.
//
//ds:def id=jt-diff-equal-f2kuyava
func compareValues(w io.Writer) {
	pairs := []struct{ x, y any }{
		{json.Number("1"), json.Number("1.0")},
		{json.Number("9007199254740993"), json.Number("9007199254740992")},
		{map[string]any{"a": true, "b": nil}, map[string]any{"b": nil, "a": true}},
		{[]any{"a", "b"}, []any{"b", "a"}},
	}
	for _, p := range pairs {
		fmt.Fprintf(w, "%v == %v: %t\n", p.x, p.y, diff.Equal(p.x, p.y))
	}
}
