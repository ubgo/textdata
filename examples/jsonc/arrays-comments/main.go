// Command arrays-comments edits the elements of a JSONC array and the
// comments attached to values: Append, Insert, Remove, Raw for a value with
// its own comments, and SetComment / RemoveComment.
package main

import (
	"fmt"
	"io"
	"log"
	"os"

	"github.com/ubgo/textdata/jsonc"
)

// src is a config with a commented array.
//
//ds:def id=src-v5375csv
const src = `{
  // Hosts tried in order
  "hosts": [
    "a.internal", // primary
    "b.internal",
  ],
  "retries": 3,
}
`

func main() {
	if err := run(os.Stdout); err != nil {
		log.Fatal(err)
	}
}

// run executes every step in guide order and prints the file after each.
func run(w io.Writer) error {
	doc, err := jsonc.Parse([]byte(src))
	if err != nil {
		return err
	}
	steps := []struct {
		title string
		fn    func(*jsonc.Doc) error
	}{
		{"append and insert", growArray},
		{"remove", shrinkArray},
		{"raw value", appendRaw},
		{"comments", editComments},
	}
	for _, s := range steps {
		if err := s.fn(doc); err != nil {
			return fmt.Errorf("%s: %w", s.title, err)
		}
		fmt.Fprintf(w, "--- %s ---\n%s", s.title, doc.Render())
	}
	return readComment(w, doc)
}

// growArray adds elements: Append at the end, Insert at an index
// (0 <= i <= length). Set never creates array elements; these do.
//
//ds:def id=growarray-wk9rj7jt
func growArray(doc *jsonc.Doc) error {
	if err := doc.Append("hosts", "c.internal"); err != nil {
		return err
	}
	return doc.Insert("hosts", 0, "local")
}

// shrinkArray removes element 2 ("b.internal") with its comma.
//
//ds:def id=shrinkarray-pwtb9zs9
func shrinkArray(doc *jsonc.Doc) error {
	return doc.Remove("hosts", 2)
}

// appendRaw appends a value written as JSONC text, kept verbatim: comments
// and layout inside it survive. It must parse as exactly one value.
//
//ds:def id=appendraw-x7zj7axp
func appendRaw(doc *jsonc.Doc) error {
	return doc.Append("hosts", jsonc.Raw(`{"host": "d.internal" /* canary */}`))
}

// editComments sets the comment above one value (replacing the old one) and
// removes the comments around another.
//
//ds:def id=editcomments-zqx5vg3x
func editComments(doc *jsonc.Doc) error {
	if err := doc.SetComment("retries", "How often to retry\nbefore giving up"); err != nil {
		return err
	}
	if err := doc.SetComment("hosts", "Hosts tried in order, local first"); err != nil {
		return err
	}
	return doc.RemoveComment("hosts[1]")
}

// readComment reads the comment back as plain text, without the markers.
//
//ds:def id=readcomment-t7x4xa8a
func readComment(w io.Writer, doc *jsonc.Doc) error {
	c, err := doc.Comment("retries")
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "comment of retries: %q\n", c)
	return nil
}
