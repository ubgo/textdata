// Command property-harness is a toy line format ("key=value" lines) written
// the way every textdata format is: the document keeps its source bytes and an
// edit splices only the bytes of the value it changes. The guarantees are
// checked in harness_test.go with package texttest (RoundTrip, CheckUntouched,
// Untouched, DiffWindow, and a fuzz target seeded by Seed); texttest imports
// testing, so it belongs in _test.go files only, never in main.
package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
)

// separator splits a key from its value on a line of the toy format.
const separator = '='

// sample is a file with a comment, a blank line and CRLF endings: the bytes a
// careless editor would normalise.
const sample = "# service\r\nhost=db.internal\r\n\r\nport=5432\r\n"

func main() {
	if err := run(os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// Doc is a parsed document: the source bytes, kept whole so Render can return
// them unchanged.
//
//ds:def id=doc-pbus3bse
type Doc struct{ src []byte }

// Parse never fails: any input, valid or not, is kept so it renders back.
//
//ds:def id=parse-nejpc267
func Parse(src []byte) *Doc { return &Doc{src: bytes.Clone(src)} }

// Render returns the document's bytes.
func (d *Doc) Render() []byte { return bytes.Clone(d.src) }

// valueSpan returns the byte range [from, to) of key's value, or ok=false.
//
//ds:def id=doc-valuespan-mxcapbk5
func (d *Doc) valueSpan(key string) (from, to int, ok bool) {
	for start := 0; start < len(d.src); {
		end := bytes.IndexByte(d.src[start:], '\n')
		if end < 0 {
			end = len(d.src) - start
		}
		line := bytes.TrimRight(d.src[start:start+end], "\r")
		if k, _, found := bytes.Cut(line, []byte{separator}); found && string(k) == key {
			return start + len(k) + 1, start + len(line), true
		}
		start += end + 1
	}
	return 0, 0, false
}

// Set replaces key's value by splicing only its bytes, and returns the range
// of the old source it replaced so a test can check nothing else moved.
//
//ds:def id=doc-set-m9smkbsq
func (d *Doc) Set(key, value string) (from, to int, ok bool) {
	from, to, ok = d.valueSpan(key)
	if !ok {
		return 0, 0, false
	}
	d.src = append(d.src[:from:from], append([]byte(value), d.src[to:]...)...)
	return from, to, true
}

// run edits one value and prints the file before and after.
func run(w io.Writer) error {
	d := Parse([]byte(sample))
	from, to, ok := d.Set("port", "6543")
	if !ok {
		return fmt.Errorf("port not found")
	}
	fmt.Fprintf(w, "before: %q\nafter:  %q\nreplaced bytes [%d,%d)\n", sample, d.Render(), from, to)
	return nil
}
