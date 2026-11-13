// Command merge-layers deep-merges an override file onto a base file, the
// usual "defaults plus local changes" layout, and shows Clone for merging
// into a copy while the base stays untouched.
package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log"
	"os"

	"github.com/ubgo/textdata/jsonc"
)

// baseSrc is the shared defaults file, with the comments a team keeps in it.
//
//ds:def id=basesrc-4jzfdp6r
const baseSrc = `{
  // HTTP listener
  "server": {
    "host": "0.0.0.0",
    "port": 8080,
  },
  "features": ["auth", "audit"],
}
`

// overrideSrc is one machine's local file: it changes one nested value,
// replaces the array whole and adds a new key with its own comment.
//
//ds:def id=overridesrc-cnk5hmx8
const overrideSrc = `{
  "server": {"port": 9090},
  "features": ["auth"],
  // Local tracing collector
  "tracing": "http://localhost:4318",
}
`

func main() {
	if err := run(os.Stdout); err != nil {
		log.Fatal(err)
	}
}

// run executes every step in guide order.
func run(w io.Writer) error {
	base, err := jsonc.Parse([]byte(baseSrc), jsonc.WithFileName("base.jsonc"))
	if err != nil {
		return err
	}
	override, err := jsonc.Parse([]byte(overrideSrc), jsonc.WithFileName("local.jsonc"))
	if err != nil {
		return err
	}
	merged, err := mergeLayers(base, override)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "--- merged ---\n%s", merged.Render())
	fmt.Fprintln(w, "base unchanged:", bytes.Equal(base.Render(), []byte(baseSrc)))
	if err := mergeAgain(w, merged, override); err != nil {
		return err
	}
	return refuseBroken(w, merged)
}

// mergeLayers clones the base and merges the override into the clone, so the
// base document can be reused or saved as it was.
//
//ds:def id=mergelayers-nrqw7asf
func mergeLayers(base, override *jsonc.Doc) (*jsonc.Doc, error) {
	merged := base.Clone()
	if err := merged.Merge(override); err != nil {
		return nil, err
	}
	return merged, nil
}

// mergeAgain shows a merge is idempotent: values already equal are left
// alone, so merging the same override twice changes nothing.
//
//ds:def id=mergeagain-fmwspgqs
func mergeAgain(w io.Writer, merged, override *jsonc.Doc) error {
	before := merged.Render()
	if err := merged.Merge(override); err != nil {
		return err
	}
	fmt.Fprintln(w, "second merge is a no-op:", bytes.Equal(before, merged.Render()))
	return nil
}

// refuseBroken shows Merge refusing an override with syntax errors (or a
// root that is not an object) and leaving the target unchanged.
//
//ds:def id=refusebroken-6qvehjs8
func refuseBroken(w io.Writer, merged *jsonc.Doc) error {
	// Parse always returns a Doc; broken's syntax error is the input under
	// test, so it is dropped here and Merge is left to refuse it.
	broken, _ := jsonc.Parse([]byte(`{"server": {port: 1}}`))
	array, err := jsonc.Parse([]byte(`[1, 2]`))
	if err != nil {
		return err
	}
	before := merged.Render()
	errBroken := merged.Merge(broken)
	errArray := merged.Merge(array)
	fmt.Fprintln(w, "broken is ErrInvalid:", errors.Is(errBroken, jsonc.ErrInvalid))
	fmt.Fprintln(w, "array root is ErrType:", errors.Is(errArray, jsonc.ErrType))
	fmt.Fprintln(w, "target unchanged:", bytes.Equal(before, merged.Render()))
	return nil
}
