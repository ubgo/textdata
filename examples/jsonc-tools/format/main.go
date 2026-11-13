// Command format re-lays out a hand-edited JSONC file in one style without
// losing a comment, checks whether a file is already formatted (what a
// `fmt --check` step in CI does), and minifies it to plain JSON.
//
// Run it with `go run .`; it prints each stage to stdout.
package main

import (
	"bytes"
	"fmt"
	"io"
	"os"

	"github.com/ubgo/textdata/jsonc"
	"github.com/ubgo/textdata/jsonc/format"
)

// fileName labels positions in errors; nothing is read from disk.
const fileName = "settings.jsonc"

// messy is a config as people leave it: mixed indentation, members on one
// line, a comment between a key and its value, a trailing comma.
const messy = `{
  // editor settings
    "tabSize":4, "insertSpaces" : true,


  "files": { "exclude": ["node_modules",".git"], },
  "theme": /* dark by default */ "dark"
}`

// indentWide is the indent of the custom style in step 3.
const indentWide = 4

func main() {
	if err := run(os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// run prints every stage, separated by a heading, so the test can pin the
// whole output.
func run(w io.Writer) error {
	d, err := jsonc.Parse([]byte(messy), jsonc.WithFileName(fileName))
	if err != nil {
		return err
	}
	tidy, err := formatDefault(d)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "== DefaultStyle\n%s", tidy)

	wide, err := formatCustom(d)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "== Indent 4, TrailingCommas\n%s", wide)

	before, err := isFormatted([]byte(messy))
	if err != nil {
		return err
	}
	after, err := isFormatted(tidy)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "== check\nmessy formatted: %t\ntidy formatted: %t\n", before, after)

	min, err := minify(d)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "== Minify\n%s\n", min)

	fmt.Fprintf(w, "== invalid input\n%v\n", formatInvalid())
	return nil
}

// formatDefault lays d out the way common editors write JSONC: two spaces,
// no trailing commas, a final newline. d itself is not changed.
//
//ds:def id=jt-fmt-default-qrdpm99e
func formatDefault(d *jsonc.Doc) ([]byte, error) {
	return format.Format(d, format.DefaultStyle())
}

// formatCustom shows the Style knobs: a wider indent and a trailing comma
// after the last member of every non-empty container.
//
//ds:def id=jt-fmt-custom-kpa5fn3a
func formatCustom(d *jsonc.Doc) ([]byte, error) {
	style := format.Style{Indent: indentWide, TrailingCommas: true}
	return format.Format(d, style)
}

// isFormatted is the `fmt --check` rule: a file is formatted when Format
// returns its own bytes. Format is idempotent, so formatted output always
// passes.
//
//ds:def id=jt-fmt-check-bptawujv
func isFormatted(src []byte) (bool, error) {
	d, err := jsonc.Parse(src, jsonc.WithFileName(fileName))
	if err != nil {
		return false, err
	}
	out, err := format.Format(d, format.DefaultStyle())
	if err != nil {
		return false, err
	}
	return bytes.Equal(out, src), nil
}

// minify turns d into compact plain JSON for a program that only reads
// JSON: comments and trailing commas are dropped.
//
//ds:def id=jt-fmt-minify-8fgnr54y
func minify(d *jsonc.Doc) ([]byte, error) {
	return format.Minify(d)
}

// formatInvalid shows that Format refuses a document with syntax errors
// instead of guessing; the error carries file:line:col.
//
//ds:def id=jt-fmt-invalid-mkh66wj7
func formatInvalid() error {
	// Parse always returns a Doc; its syntax error is the input under test,
	// so it is dropped here and Format is left to refuse the Doc.
	d, _ := jsonc.Parse([]byte(`{"port": 80 "host": "a"}`), jsonc.WithFileName(fileName))
	_, err := format.Format(d, format.DefaultStyle())
	return err
}
