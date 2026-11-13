// Command entries shows how the dotenv library sees a file line by line: the
// six entry kinds, the four Entry fields, a walk over Entries() that reports
// what the read accessors skip, and the lines whose kind surprises people.
//
// Run it with `go run .`; nothing touches the disk.
package main

import (
	"fmt"
	"io"
	"os"

	"github.com/ubgo/textdata/dotenv"
)

// everyKind holds one line of each of the six kinds, in the order of the
// table in the guide.
const everyKind = `PORT=8080
# a note

not a setting!
# DEBUG=true
export AWS_REGION
`

// audited is the file the linter step reports on.
const audited = `# app
PORT=8080
# DB_USER=admin
MY KEY=value
HOME
CERT="line one
line two"
`

func main() {
	if err := run(os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// run prints every step under a heading so the test can pin the whole output.
func run(w io.Writer) error {
	fmt.Fprintln(w, "== kinds")
	printKinds(w)
	fmt.Fprintln(w, "== fields")
	printFields(w)
	fmt.Fprintln(w, "== lint")
	lint(w, dotenv.Parse(audited))
	fmt.Fprintln(w, "== surprises")
	surprises(w)
	return nil
}

// printKinds parses one line of each kind and prints the Kind (a
// fmt.Stringer), the Key and whether the line is active, that is visible to
// Get, Keys and Map.
//
//ds:def id=printkinds-s8vq3j6t
func printKinds(w io.Writer) {
	f := dotenv.Parse(everyKind)
	for _, e := range f.Entries() {
		fmt.Fprintf(w, "%-13s key=%-12q active=%v\n", e.Kind, e.Key, e.Kind == dotenv.KindPair)
	}
	fmt.Fprintln(w, "out of range:", dotenv.Kind(99))
}

// printFields shows the difference between Raw (the exact physical lines,
// several for a multi-line value) and Value (decoded, quotes stripped, lines
// joined with \n, references left as written).
//
//ds:def id=printfields-g5hn2w9r
func printFields(w io.Writer) {
	f := dotenv.Parse("URL=\"https://${HOST}/v1\"\nCERT=\"line one\nline two\"\n")
	for _, e := range f.Entries() {
		fmt.Fprintf(w, "%s %s: Raw=%q Value=%q\n", e.Kind, e.Key, e.Raw, e.Value)
	}
}

// lint walks every entry and reports what the read accessors silently skip:
// disabled settings, unrecognised lines and inherited names. Switching on Kind
// covers the closed set.
//
//ds:def id=lint-x7cm4b1z
func lint(w io.Writer, f *dotenv.File) {
	for i, e := range f.Entries() {
		switch e.Kind {
		case dotenv.KindPair:
			fmt.Fprintf(w, "entry %d: %s is set (%d line(s))\n", i, e.Key, len(e.Raw))
		case dotenv.KindDisabledPair:
			fmt.Fprintf(w, "entry %d: %s is disabled\n", i, e.Key)
		case dotenv.KindInherited:
			fmt.Fprintf(w, "entry %d: %s comes from the environment\n", i, e.Key)
		case dotenv.KindOther:
			fmt.Fprintf(w, "entry %d: not a setting: %q\n", i, e.Raw[0])
		case dotenv.KindComment, dotenv.KindBlank:
			// Prose and spacing: nothing to report.
		}
	}
	fmt.Fprintln(w, "Disabled():", f.Disabled(), "Inherited():", f.Inherited())
}

// surprises prints the kind of each line people misread: prose shaped like an
// assignment, a commented colon line with a sentence or a URL after it, a
// bare URL, a key with a space, and a commented-out multi-line value.
//
//ds:def id=surprises-d2pw8k5v
func surprises(w io.Writer) {
	src := "# TODO=fix this\n" +
		"# Note: rotate monthly\n" +
		"# KEY: value\n" +
		"# http://example.com\n" +
		"http://example.com\n" +
		"MY KEY=value\n" +
		"# CERT=\"one\n" +
		"# two\"\n"
	for _, e := range dotenv.Parse(src).Entries() {
		fmt.Fprintf(w, "%-13s key=%-7q %q\n", e.Kind, e.Key, e.Raw[0])
	}
}
