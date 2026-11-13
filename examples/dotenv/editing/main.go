// Command editing changes a .env file in memory the way a tool should: Set in
// place, place a new key next to a related one, comment a setting out and back
// in, list disabled and inherited names, and generate a file from entries.
// Every line it does not touch keeps its exact bytes.
//
// Run it with `go run .`; nothing touches the disk.
package main

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/ubgo/textdata/dotenv"
)

// app is the hand-written file the editing steps work on.
const app = `# app
export PORT = 8080 # dev only
HOST=localhost
DB_HOST=db.internal
#DEBUG=true
`

func main() {
	if err := run(os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// run prints every step under a heading so the test can pin the whole output.
func run(w io.Writer) error {
	fmt.Fprintln(w, "== Set")
	setValues(w)
	fmt.Fprintln(w, "== SetAfter and SetBefore")
	if err := placeKeys(w); err != nil {
		return err
	}
	fmt.Fprintln(w, "== Unset and Restore")
	toggle(w)
	fmt.Fprintln(w, "== Inherited")
	if err := inherited(w); err != nil {
		return err
	}
	fmt.Fprintln(w, "== generate")
	if err := generate(w); err != nil {
		return err
	}
	fmt.Fprintln(w, "== Append vs Set")
	appendVsSet(w)
	return nil
}

// setValues changes a value in place, keeping the export prefix, spacing and
// inline comment; appends a missing key; and shows that setting the same value
// again changes no byte. Set picks the quoting for you.
//
//ds:def id=setvalues-m6tc9w2k
func setValues(w io.Writer) {
	f := dotenv.Parse(app)
	created := f.Set("PORT", "9090")
	fmt.Fprintln(w, "PORT created:", created)
	created = f.Set("LOG_LEVEL", "info")
	fmt.Fprintln(w, "LOG_LEVEL created:", created)

	before := f.Render()
	f.Set("HOST", "localhost")
	fmt.Fprintln(w, "same value is a no-op:", f.Render() == before)

	f.Set("GREETING", "hello world")
	f.Set("PASSWORD", "p$$w")
	f.Set("MOTD", "line one\nline two")
	fmt.Fprint(w, f.Render())
}

// placeKeys puts a new key next to a related one. A key that already exists
// is updated where it is, not moved. A missing anchor is an error that
// changes nothing; falling back to Set is the caller's decision.
//
//ds:def id=placekeys-y3fd7h5n
func placeKeys(w io.Writer) error {
	f := dotenv.Parse(app)
	if _, err := f.SetAfter("DB_HOST", "DB_PORT", "5432"); err != nil {
		return err
	}
	if _, err := f.SetBefore("PORT", "APP_NAME", "shop"); err != nil {
		return err
	}
	_, err := f.SetAfter("NOPE", "CACHE_URL", "redis://cache")
	if errors.Is(err, dotenv.ErrAnchorNotFound) {
		fmt.Fprintln(w, "anchor missing:", err)
		f.Set("CACHE_URL", "redis://cache")
	}
	fmt.Fprint(w, f.Render())
	return nil
}

// toggle comments a setting out with Unset(key, false), lists what is
// disabled, then brings a line back with Restore, which removes exactly the
// marker that was recorded, so the original bytes come back.
//
//ds:def id=toggle-r8kb2q6v
func toggle(w io.Writer) {
	f := dotenv.Parse(app)
	fmt.Fprintln(w, "Unset HOST:", f.Unset("HOST", false))
	fmt.Fprintln(w, "Unset HOST again:", f.Unset("HOST", false))
	fmt.Fprintln(w, "Unset DB_HOST, delete:", f.Unset("DB_HOST", true))
	fmt.Fprintln(w, "Disabled:", f.Disabled())
	fmt.Fprint(w, f.Render())

	fmt.Fprintln(w, "Restore HOST:", f.Restore("HOST"))
	fmt.Fprintln(w, "Restore DEBUG:", f.Restore("DEBUG"))
	fmt.Fprint(w, f.Render())
}

// inherited lists Compose's name-only declarations. The library never reads
// the process environment; the caller decides where the values come from
// (os.LookupEnv in a real program, a fixed map here).
//
//ds:def id=inherited-j4wz6c8p
func inherited(w io.Writer) error {
	f := dotenv.Parse("PORT=8080\nHOME\nexport AWS_REGION\nHOME\n")
	source := map[string]string{"HOME": "/home/app"}
	for _, name := range f.Inherited() {
		v, ok := source[name]
		fmt.Fprintf(w, "%s: %q found=%v\n", name, v, ok)
	}
	v, _, err := f.GetExpanded("HOME")
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "GetExpanded(HOME) = %q\n", v)
	return nil
}

// generate builds a .env.example from constructors. Append places exactly the
// entries given; InsertAfter places a block next to an anchor as one unit. An
// entry taken from a parsed file is copied verbatim, which is how a ${ref}
// line is written (Set would store it literally).
//
//ds:def id=generate-e9ts3m7f
func generate(w io.Writer) error {
	f := dotenv.Parse("")
	f.Append(
		dotenv.NewComment("Copy to .env and fill in.", "# Do not commit .env."),
		dotenv.NewBlank(),
		dotenv.NewPair("HOST", "localhost"),
		dotenv.NewPair("GREETING", "hello world"),
	)
	ref := dotenv.Parse("URL=https://${HOST}/v1 # derived\n").Entries()[0]
	if err := f.InsertAfter("HOST", dotenv.NewComment("derived"), ref); err != nil {
		return err
	}
	fmt.Fprint(w, f.Render())

	crlf := dotenv.Parse("A=1\r\n")
	crlf.Append(dotenv.NewPair("B", "2"))
	fmt.Fprintf(w, "CRLF file: %q\n", crlf.Render())
	return nil
}

// appendVsSet shows why the two families must not be mixed up: Append adds a
// second entry for a key that exists, Set updates the one that is there.
//
//ds:def id=appendvsset-h7nr4x2c
func appendVsSet(w io.Writer) {
	upsert := dotenv.Parse("PORT=8080\n")
	upsert.Set("PORT", "9090")
	literal := dotenv.Parse("PORT=8080\n")
	literal.Append(dotenv.NewPair("PORT", "9090"))
	fmt.Fprintf(w, "Set:    %q Count=%d\n", upsert.Render(), upsert.Count("PORT"))
	fmt.Fprintf(w, "Append: %q Count=%d\n", literal.Render(), literal.Count("PORT"))
}
