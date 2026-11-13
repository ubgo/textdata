package jsonc

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/ubgo/textdata/texttest"
)

// seed adds the shared corpus, the edit layouts, the samples and a slice of
// the JSONTestSuite files.
func seed(f *testing.F) {
	texttest.Seed(f, append(append([]string{}, layouts...), sample, dynamic,
		"{\n  \"a\": 1,\n  // \"b\": 2\n}", "[1, [2, [3, {}]],]", "{\"a\":{\"b\":{\"c\":[]}}}")...)
	files, _ := filepath.Glob("testdata/JSONTestSuite/*.json")
	for i, file := range files {
		if i%4 != 0 {
			continue
		}
		if b, err := os.ReadFile(file); err == nil && len(b) < 4096 {
			f.Add(b)
		}
	}
}

// FuzzParseRender: any input parses without panicking, renders byte for
// byte in both modes, and an accepted document's JSON() is valid JSON.
func FuzzParseRender(f *testing.F) {
	seed(f)
	f.Fuzz(func(t *testing.T, b []byte) {
		for _, strict := range []bool{false, true} {
			var opts []Option
			if strict {
				opts = append(opts, Strict())
			}
			d, err := Parse(b, opts...)
			texttest.RoundTrip(t, b, func([]byte) ([]byte, error) { return d.Render(), nil })
			if err != nil {
				continue
			}
			j, err := d.JSON()
			if err != nil || !json.Valid(j) {
				t.Fatalf("accepted %q but JSON() = %q, %v", b, j, err)
			}
			if strict && !bytes.Equal(j, b) {
				t.Fatalf("strict JSON() changed plain JSON %q -> %q", b, j)
			}
		}
	})
}

// FuzzStrictMatchesEncodingJSON: Strict() accepts exactly what
// encoding/json.Valid accepts.
func FuzzStrictMatchesEncodingJSON(f *testing.F) {
	seed(f)
	f.Fuzz(func(t *testing.T, b []byte) {
		_, err := Parse(b, Strict())
		if got, want := err == nil, json.Valid(b); got != want {
			t.Fatalf("Strict accepted=%v, json.Valid=%v for %q (err %v)", got, want, b, err)
		}
	})
}

// fuzzKey is a key no seed uses, so Set always creates it.
const fuzzKey = "zzéfuzz"

// FuzzSetDelete: on any valid document with an object root, Set of a new key
// changes one window of the input, reads back, and Delete restores the
// original bytes.
func FuzzSetDelete(f *testing.F) {
	seed(f)
	f.Fuzz(func(t *testing.T, b []byte) {
		d, err := Parse(b)
		if err != nil || d.root == nil || d.root.kind != KindObject || d.Has(fuzzKey) {
			return
		}
		for _, v := range []any{1, "x\ny", []any{map[string]any{"k": nil}}} {
			if err := d.Set(fuzzKey, v); err != nil {
				t.Fatalf("Set on %q: %v", b, err)
			}
			after := d.Render()
			s, e, _ := texttest.DiffWindow(b, after)
			texttest.CheckUntouched(t, b, after, s, e)
			if _, err := d.Get(fuzzKey); err != nil {
				t.Fatalf("Get after Set on %q: %v", b, err)
			}
			if err := d.Delete(fuzzKey); err != nil {
				t.Fatalf("Delete on %q: %v", after, err)
			}
			if !bytes.Equal(d.Render(), b) {
				t.Fatalf("Set+Delete changed bytes\n  in: %q\n set: %q\n out: %q", b, after, d.Render())
			}
		}
	})
}

// FuzzEditSequence applies edits chosen by the fuzzer. Every edit either
// fails and leaves the bytes alone, or succeeds and leaves a valid document;
// Disable then Restore of a member is byte-identical.
func FuzzEditSequence(f *testing.F) {
	for _, s := range layouts {
		f.Add([]byte(s), []byte{0, 1, 2, 3, 4, 5, 6, 7})
	}
	f.Add([]byte(sample), []byte{3, 3, 1, 4, 7, 2})
	f.Fuzz(func(t *testing.T, b, ops []byte) {
		d, err := Parse(b)
		if err != nil || len(ops) > 16 {
			return
		}
		for i, op := range ops {
			paths := d.ValuePaths()
			target := "a"
			if len(paths) > 0 {
				target = paths[int(op)%len(paths)]
			}
			before := d.Render()
			var err error
			switch op % 8 {
			case 0:
				err = d.Set(target, i)
			case 1:
				err = d.Set(fuzzKey, []any{i})
			case 2:
				err = d.Delete(target)
			case 3:
				err = d.Append(target, "v")
			case 4:
				err = d.Insert(target, 0, i)
			case 5:
				err = d.SetComment(target, "c")
			case 6:
				err = d.RemoveComment(target)
			case 7:
				if err = d.Disable(target); err == nil && !d.Has(target) && disabledCopies(d, target) == 1 {
					mid := d.Render()
					if err := d.Restore(target); err != nil {
						t.Fatalf("Restore(%q) after Disable on %q (disabled %q): %v", target, before, mid, err)
					}
					if !bytes.Equal(d.Render(), before) {
						t.Fatalf("Disable+Restore(%q) not identical\n  in: %q\n out: %q", target, before, d.Render())
					}
				}
			}
			if err != nil && !bytes.Equal(d.Render(), before) {
				t.Fatalf("failed edit %d changed bytes: %v", op%8, err)
			}
			if _, perr := Parse(d.Render()); perr != nil {
				t.Fatalf("edit %d left an invalid document %q: %v", op%8, d.Render(), perr)
			}
		}
	})
}

// disabledCopies counts the disabled members for target's key in its parent:
// Restore is byte-identical only when Disable made the one copy.
func disabledCopies(d *Doc, target string) int {
	pp, err := parsePath(target)
	if err != nil {
		return 0
	}
	parent, last, ok := pp.Parent()
	if !ok {
		return 0
	}
	loc, err := d.find(parent)
	if err != nil {
		return 0
	}
	n := 0
	for _, c := range directComments(loc.n, d.comments) {
		if !c.block && disabledKey(d.src[c.start:c.end], last.Key) {
			n++
		}
	}
	return n
}
