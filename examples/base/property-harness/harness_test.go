package main

import (
	"bytes"
	"testing"

	"github.com/ubgo/textdata/texttest"
)

// Inputs this format adds on top of texttest.Corpus: shapes its own parser
// treats specially.
var formatSeeds = []string{sample, "=", "a=", "=b\n", "a=b=c\r\n", "no separator\n"}

// TestRoundTrip pins Parse(x).Render() == x for every awkward input.
//
//ds:def id=testroundtrip-yatrkq2c
func TestRoundTrip(t *testing.T) {
	render := func(src []byte) ([]byte, error) { return Parse(src).Render(), nil }
	for _, in := range append(texttest.Corpus, formatSeeds...) {
		texttest.RoundTrip(t, []byte(in), render)
	}
}

// FuzzRoundTrip runs the same property over generated inputs. Seed adds every
// Corpus entry plus this format's own seeds.
//
//ds:def id=fuzzroundtrip-56575zwy
func FuzzRoundTrip(f *testing.F) {
	texttest.Seed(f, formatSeeds...)
	f.Fuzz(func(t *testing.T, src []byte) {
		texttest.RoundTrip(t, src, func(b []byte) ([]byte, error) { return Parse(b).Render(), nil })
	})
}

// TestSetLeavesOtherBytes pins that an edit changes only the value it
// replaced: the comment, the blank line and every CRLF survive.
//
//ds:def id=testsetleavesotherbytes-sraxrcv2
func TestSetLeavesOtherBytes(t *testing.T) {
	before := []byte(sample)
	d := Parse(before)
	from, to, ok := d.Set("port", "6543")
	if !ok {
		t.Fatal("port not found")
	}
	texttest.CheckUntouched(t, before, d.Render(), from, to)
}

// TestUntouchedCatchesReflow shows the failure the harness exists for: an
// edit that also normalises line endings is reported, with where it differs.
//
//ds:def id=testuntouchedcatchesreflow-2cczecty
func TestUntouchedCatchesReflow(t *testing.T) {
	before := []byte(sample)
	from, to, _ := Parse(before).Set("port", "6543")
	err := texttest.Untouched(before, reflow(before, "port", "6543"), from, to)
	if err == nil {
		t.Fatal("a reflowing edit passed the untouched check")
	}
	t.Logf("caught: %v", err)
}

// TestDiffWindow finds the smallest window that differs between two renders,
// which is the edit's footprint when nothing else moved.
//
//ds:def id=testdiffwindow-nqg6djv4
func TestDiffWindow(t *testing.T) {
	before := []byte(sample)
	d := Parse(before)
	from, to, _ := d.Set("port", "6543")
	start, endA, endB := texttest.DiffWindow(before, d.Render())
	if start < from || endA > to {
		t.Fatalf("diff window [%d,%d) is wider than the edit [%d,%d)", start, endA, from, to)
	}
	t.Logf("edit [%d,%d), diff window before[%d:%d] after[%d:%d]", from, to, start, endA, start, endB)
}

// reflow is the bug texttest exists to catch: it edits a value correctly but
// also "tidies" every line ending to LF.
func reflow(src []byte, key, value string) []byte {
	d := Parse(src)
	d.Set(key, value)
	return bytes.ReplaceAll(d.Render(), []byte("\r\n"), []byte("\n"))
}
