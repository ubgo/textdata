// Package texttest is the property harness every textdata format runs its
// guarantees through: a parse→render round trip is byte-exact, and an edit
// leaves every byte outside what it touched unchanged.
//
// It is a test helper package (it imports testing) and is meant to be used
// only from _test.go files.
package texttest

import (
	"bytes"
	"fmt"
	"testing"
)

// Corpus is a set of awkward inputs every format must survive: empty input,
// BOMs, every line-ending style, NUL bytes, invalid UTF-8, long lines and
// unbalanced delimiters. Formats add their own seeds on top.
var Corpus = []string{
	"",
	"\n",
	"\r\n",
	"\r",
	"\ufeff",
	"\ufeff\n",
	"a",
	"a\n",
	"a\r\nb\r\n",
	"a\rb",
	"\x00",
	"a\x00b\n",
	"\xff\xfe\xfd",
	"\xc3\x28",
	" \t \n\t\n",
	"{",
	"}",
	"[",
	"\"",
	"'",
	"`",
	"/*",
	"//",
	"#",
	"${",
	"$(",
	"\\",
	string(bytes.Repeat([]byte("x"), 70000)),
}

// Seed adds every Corpus entry and every extra seed to a fuzz target that
// takes one []byte argument.
func Seed(f *testing.F, extra ...string) {
	f.Helper()
	for _, s := range Corpus {
		f.Add([]byte(s))
	}
	for _, s := range extra {
		f.Add([]byte(s))
	}
}

// RoundTrip fails t unless render(input) returns input byte for byte.
func RoundTrip(t testing.TB, input []byte, render func([]byte) ([]byte, error)) {
	t.Helper()
	got, err := render(input)
	if err != nil {
		t.Fatalf("round trip of %q: %v", clip(input), err)
		return
	}
	if !bytes.Equal(got, input) {
		start, endA, endB := DiffWindow(input, got)
		t.Fatalf("round trip changed bytes at offset %d:\n input: %q\noutput: %q",
			start, clip(input[start:endA]), clip(got[start:endB]))
	}
}

// DiffWindow returns the smallest window outside which a and b are equal:
// a[:start] == b[:start] and a[endA:] == b[endB:]. For equal inputs the window
// is empty (start == endA == endB == len(a)).
func DiffWindow(a, b []byte) (start, endA, endB int) {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	for start < n && a[start] == b[start] {
		start++
	}
	endA, endB = len(a), len(b)
	for endA > start && endB > start && a[endA-1] == b[endB-1] {
		endA--
		endB--
	}
	return start, endA, endB
}

// Untouched returns an error unless every byte of before outside [from, to)
// survives in after: after must equal before[:from] + something +
// before[to:]. An edit that only touches [from, to) always passes; one that
// also reflows an unrelated line fails.
func Untouched(before, after []byte, from, to int) error {
	if from < 0 || to < from || to > len(before) {
		return fmt.Errorf("texttest: window [%d,%d) outside input of %d bytes", from, to, len(before))
	}
	if !bytes.HasPrefix(after, before[:from]) {
		start, _, _ := DiffWindow(before[:from], after[:min(from, len(after))])
		return fmt.Errorf("texttest: bytes before the edit changed at offset %d", start)
	}
	tail := before[to:]
	if len(after)-from < len(tail) || !bytes.HasSuffix(after, tail) {
		return fmt.Errorf("texttest: bytes after the edit (from offset %d of the input) changed", to)
	}
	return nil
}

// CheckUntouched fails t when Untouched reports an error.
func CheckUntouched(t testing.TB, before, after []byte, from, to int) {
	t.Helper()
	if err := Untouched(before, after, from, to); err != nil {
		t.Fatalf("%v\nbefore: %q\n after: %q", err, clip(before), clip(after))
	}
}

// clipLen bounds how much of an input a failure message prints, so a 70 KB
// fuzz input does not drown the report.
const clipLen = 200

func clip(b []byte) []byte {
	if len(b) <= clipLen {
		return b
	}
	return append(append([]byte(nil), b[:clipLen]...), "…"...)
}
