// Command paths shows the two small base packages every tree format leans on:
// package path (dotted paths and JSON Pointer) and package pos (byte offsets to
// line:col, and file:line:col errors). It finishes by reading the position of
// a real jsonc syntax error.
package main

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/ubgo/textdata/jsonc"
	"github.com/ubgo/textdata/path"
	"github.com/ubgo/textdata/pos"
)

// configName is the file name the demo errors are reported against.
const configName = "config.jsonc"

func main() {
	if err := run(os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// run executes every step of the guide in order.
func run(w io.Writer) error {
	if err := parsePaths(w); err != nil {
		return err
	}
	buildPaths(w)
	positions(w)
	errorLists(w)
	return jsoncErrorPosition(w)
}

// parsePaths reads the dotted form and a JSON Pointer to the same value, and
// shows that a quoted name keeps its dot.
//
//ds:def id=parsepaths-dseftc6n
func parsePaths(w io.Writer) error {
	dotted, err := path.Parse(`servers."eu.west"[0].port`)
	if err != nil {
		return err
	}
	pointer, err := path.ParsePointer("/servers/eu.west/0/port")
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "dotted:  %s  pointer form %s\n", dotted, dotted.Pointer())
	fmt.Fprintf(w, "pointer: %s  segment 2 index=%v key=%v\n", pointer, pointer[2].MatchesIndex(), pointer[2].MatchesKey())
	for _, in := range []string{"/a~1b/c", "a.b", ""} {
		p, err := path.ParseAny(in)
		if err != nil {
			return err
		}
		fmt.Fprintf(w, "ParseAny(%q) = %d segments, String %q\n", in, len(p), p.String())
	}
	_, err = path.Parse(`a[x]`)
	fmt.Fprintf(w, "bad path: %v (syntax=%v)\n", err, errors.Is(err, path.ErrSyntax))
	return nil
}

// buildPaths composes a path in code. Append returns a new path, so a base
// path can be shared; Parent walks back up.
//
//ds:def id=buildpaths-9ckr88zf
func buildPaths(w io.Writer) {
	base := path.MustParse("plugins")
	item := base.Append(path.Index(2), path.Key("with.dot"))
	parent, last, ok := item.Parent()
	fmt.Fprintf(w, "built: %s  parent %s  last key %q ok=%v  base still %s\n", item, parent, last.Key, ok, base)
}

// positions maps byte offsets to line:col. Columns count bytes, and a CRLF
// file numbers its lines exactly like an LF file.
//
//ds:def id=positions-udy9nhr3
func positions(w io.Writer) {
	src := []byte("{\r\n  \"port\": 80x\r\n}\r\n")
	idx := pos.NewIndex(src)
	p := idx.Pos(14) // the 'x'
	fmt.Fprintf(w, "offset 14 -> %s (line %d col %d), lines=%d, zero Pos prints %s\n", p, p.Line, p.Col, idx.Lines(), pos.Pos{})
}

// errorLists collects problems out of order, sorts them, and returns nil
// when there are none, so a function can always end with `return list.Err()`.
//
//ds:def id=errorlists-4uyzaxz7
func errorLists(w io.Writer) {
	idx := pos.NewIndex([]byte("a\nb\nc\n"))
	var list pos.ErrorList
	fmt.Fprintf(w, "empty list err is nil: %v\n", list.Err() == nil)
	list.Add(pos.Errorf(configName, idx.Pos(4), "second problem"))
	list.Add(pos.Errorf(configName, idx.Pos(0), "first problem"))
	list.Sort()
	fmt.Fprintf(w, "list: %v\nfirst: %v\n", list.Err(), pos.First(list.Err()))
}

// jsoncErrorPosition parses broken JSONC and pulls the first positioned error
// out of whatever the format returned.
//
//ds:def id=jsoncerrorposition-xdqrn438
func jsoncErrorPosition(w io.Writer) error {
	_, err := jsonc.Parse([]byte("{\n  \"a\": 1\n  \"b\": 2\n}\n"), jsonc.WithFileName(configName))
	first := pos.First(err)
	if first == nil {
		return fmt.Errorf("expected a positioned error, got %v", err)
	}
	fmt.Fprintf(w, "jsonc: %v\n  at line %d col %d: %s\n", err, first.Pos.Line, first.Pos.Col, first.Msg)
	return nil
}
