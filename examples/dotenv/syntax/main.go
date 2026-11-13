// Command syntax walks the .env grammar the dotenv parser applies: the four
// quoting styles, multi-line values, comments, the `:` delimiter, key names,
// duplicates, encrypted values and the two escape modes. Each step prints a
// line as Get (decoded) and GetExpanded (references resolved) so the effect of
// each rule is visible.
//
// Run it with `go run .`; nothing touches the disk.
package main

import (
	"fmt"
	"io"
	"os"

	"github.com/ubgo/textdata/dotenv"
)

func main() {
	if err := run(os.Stdout); err != nil {
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
func run(w io.Writer) error {
	steps := []step{
		{"quoting", quoting},
		{"multi-line", multiline},
		{"comments", comments},
		{"colon delimiter", colonDelimiter},
		{"key names", keyNames},
		{"duplicates", duplicates},
		{"encrypted values", encrypted},
		{"escape modes", escapeModes},
	}
	for _, s := range steps {
		fmt.Fprintln(w, "==", s.title)
		if err := s.show(w); err != nil {
			return err
		}
	}
	return nil
}

// show prints Get and GetExpanded for every active key of f with %q, so
// trailing spaces, newlines and tabs are visible.
//
//ds:def id=synshow-q4tz8m2d
func show(w io.Writer, f *dotenv.File) error {
	for _, key := range f.Keys() {
		raw, _ := f.Get(key)
		expanded, _, err := f.GetExpanded(key)
		if err != nil {
			return err
		}
		fmt.Fprintf(w, "%s: Get=%q GetExpanded=%q\n", key, raw, expanded)
	}
	return nil
}

// quoting shows the four ways to write a value. Bare values are trimmed and
// expanded; double quotes keep whitespace and expand; single quotes and
// backticks keep everything literally.
//
//ds:def id=quoting-c7wn3k5e
func quoting(w io.Writer) error {
	src := "NAME=world\n" +
		"BARE=hello $NAME   \n" +
		"DOUBLE=\"hello $NAME  \"\n" +
		"SINGLE='hello $NAME'\n" +
		"BACKTICK=`it's \"hi\" $NAME`\n" +
		"export EXPORTED=yes\n" +
		"  SPACED  =   padded value  \n" +
		"EMPTY=\n"
	return show(w, dotenv.Parse(src))
}

// multiline shows that a quoted value runs on until its closing quote, and
// that a missing closing quote swallows every line after it.
//
//ds:def id=multiline-v6pb9t1x
func multiline(w io.Writer) error {
	src := "CERT=\"line one\nline two\"\n" +
		"PEM=`-----BEGIN KEY-----\nab'c\"d\n-----END KEY-----`\n"
	if err := show(w, dotenv.Parse(src)); err != nil {
		return err
	}

	broken := "A=1\nB=\"never closed\nC=3\nD=4\n"
	f := dotenv.Parse(broken)
	fmt.Fprintln(w, "unterminated keys:", f.Keys(), "round trip:", f.Render() == broken)
	return nil
}

// comments shows where a bare value ends: `#` starts a comment only after a
// space or tab, never inside quotes, and text after a closing quote is not
// part of the value. A commented assignment is a disabled setting.
//
//ds:def id=comments-k2rf5y8j
func comments(w io.Writer) error {
	src := "# a full-line comment\n" +
		"PASSWORD=a#b\n" +
		"PORT=8080 # inline comment\n" +
		"QUOTED=\"a # b\" # c\n" +
		"AFTER=\"kept\" dropped\n" +
		"# DISABLED=off\n"
	f := dotenv.Parse(src)
	if err := show(w, f); err != nil {
		return err
	}
	fmt.Fprintln(w, "disabled:", f.Disabled())
	return nil
}

// colonDelimiter shows Compose's `KEY: value` form, and the trap it sets for a
// line that is only a URL.
//
//ds:def id=colondelimiter-u8dh4n6q
func colonDelimiter(w io.Writer) error {
	src := "HOST: db.internal\n" +
		"export PORT:5432\n" +
		"URL=http://example.com:8080\n" +
		"http://example.com\n"
	return show(w, dotenv.Parse(src))
}

// keyNames shows Docker Compose's key charset, and that a line which is not a
// pair becomes a KindOther entry rather than an error.
//
//ds:def id=keynames-a5mx2c7w
func keyNames(w io.Writer) error {
	src := "2FA_SECRET=a\n" +
		"spring.datasource.url=b\n" +
		"MY-SERVICE-TOKEN=c\n" +
		"SERVICES[0]=d\n" +
		"HAS KEY=e\n" +
		"=no key\n" +
		"HOME\n"
	for _, e := range dotenv.Parse(src).Entries() {
		fmt.Fprintf(w, "%-9s key=%q raw=%q\n", e.Kind, e.Key, e.Raw[0])
	}
	return nil
}

// duplicates shows that the last line wins, and that Count and Pairs still
// see the earlier ones so a lint step can report them.
//
//ds:def id=duplicates-n3ge6r9b
func duplicates(w io.Writer) error {
	f := dotenv.Parse("LEVEL=debug\nLEVEL=info\n")
	v, _ := f.Get("LEVEL")
	fmt.Fprintln(w, "Get:", v, "Count:", f.Count("LEVEL"), "Pairs:", f.Pairs())
	return nil
}

// encrypted shows that an `encrypted:` value is an ordinary string, and why
// ciphertext containing `$` must be single-quoted.
//
//ds:def id=encrypted-w9js1f4h
func encrypted(w io.Writer) error {
	src := "BARE=encrypted:ab$cd\n" +
		"SINGLE='encrypted:ab$cd'\n"
	return show(w, dotenv.Parse(src))
}

// synEscapes is decoded twice, once per escape mode.
const synEscapes = `NEWLINE="a\nb"
WINPATH="C:\new\temp"
QUOTE="say \"hi\""
SINGLE='a\nb'
`

// escapeModes shows that backslash escapes are decoded only inside double
// quotes, and that the two modes disagree about \n, \r and \t.
//
//ds:def id=escapemodes-b4kt7p2m
func escapeModes(w io.Writer) error {
	for _, mode := range []dotenv.EscapeMode{dotenv.EscapeExtended, dotenv.EscapeCompose} {
		fmt.Fprintln(w, "mode:", mode)
		f := dotenv.Parse(synEscapes, dotenv.WithEscapes(mode))
		for _, key := range f.Keys() {
			v, _ := f.Get(key)
			fmt.Fprintf(w, "  %s=%q\n", key, v)
		}
	}
	return nil
}
