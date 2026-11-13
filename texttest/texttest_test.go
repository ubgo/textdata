package texttest

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestDiffWindow(t *testing.T) {
	t.Parallel()
	tests := []struct {
		a, b              string
		start, endA, endB int
	}{
		{"abc", "abc", 3, 3, 3},
		{"abc", "aXc", 1, 2, 2},
		{"abc", "abXYc", 2, 2, 4},
		{"", "x", 0, 0, 1},
		{"aaa", "aa", 2, 3, 2},
	}
	for _, tt := range tests {
		s, ea, eb := DiffWindow([]byte(tt.a), []byte(tt.b))
		if s != tt.start || ea != tt.endA || eb != tt.endB {
			t.Errorf("DiffWindow(%q,%q) = %d,%d,%d want %d,%d,%d", tt.a, tt.b, s, ea, eb, tt.start, tt.endA, tt.endB)
		}
	}
}

func TestUntouched(t *testing.T) {
	t.Parallel()
	before := []byte("# head\nA=1\nB=2\n# tail\n")
	from := bytes.Index(before, []byte("B=2"))
	to := from + len("B=2")
	ok := bytes.Replace(before, []byte("B=2"), []byte("B=22222"), 1)
	if err := Untouched(before, ok, from, to); err != nil {
		t.Errorf("valid edit rejected: %v", err)
	}
	if err := Untouched(before, bytes.Replace(ok, []byte("# head"), []byte("#head"), 1), from, to); err == nil || !strings.Contains(err.Error(), "before") {
		t.Errorf("head change = %v", err)
	}
	if err := Untouched(before, bytes.Replace(ok, []byte("# tail"), []byte("#tail"), 1), from, to); err == nil || !strings.Contains(err.Error(), "after") {
		t.Errorf("tail change = %v", err)
	}
	if err := Untouched(before, []byte("# h"), from, to); err == nil {
		t.Error("truncated output accepted")
	}
	if err := Untouched(before, before, 5, 2); err == nil {
		t.Error("bad window accepted")
	}
	// Deleting exactly the window is fine.
	del := append(append([]byte(nil), before[:from]...), before[to:]...)
	CheckUntouched(t, before, del, from, to)
}

func TestRoundTripAndSeed(t *testing.T) {
	t.Parallel()
	for _, s := range Corpus {
		RoundTrip(t, []byte(s), func(b []byte) ([]byte, error) { return append([]byte(nil), b...), nil })
	}
	ft := &fakeTB{TB: t}
	RoundTrip(ft, []byte("abc"), func([]byte) ([]byte, error) { return []byte("abd"), nil })
	if !ft.failed || !strings.Contains(ft.msg, "offset 2") {
		t.Errorf("mismatch not reported: %q", ft.msg)
	}
	ft = &fakeTB{TB: t}
	RoundTrip(ft, []byte("x"), func([]byte) ([]byte, error) { return nil, errors.New("boom") })
	if !ft.failed || !strings.Contains(ft.msg, "boom") {
		t.Errorf("error not reported: %q", ft.msg)
	}
	ft = &fakeTB{TB: t}
	CheckUntouched(ft, []byte("ab"), []byte("xb"), 1, 2)
	if !ft.failed {
		t.Error("CheckUntouched did not fail")
	}
	if got := clip(bytes.Repeat([]byte("y"), 500)); len(got) != clipLen+len("…") {
		t.Errorf("clip len = %d", len(got))
	}
}

func FuzzSeedIsUsable(f *testing.F) {
	Seed(f, "extra")
	f.Fuzz(func(t *testing.T, b []byte) {
		RoundTrip(t, b, func(in []byte) ([]byte, error) { return bytes.Clone(in), nil })
	})
}

// fakeTB records a Fatalf instead of stopping the real test.
type fakeTB struct {
	testing.TB
	failed bool
	msg    string
}

func (f *fakeTB) Helper() {}

func (f *fakeTB) Fatalf(format string, args ...any) {
	f.failed = true
	f.msg = fmt.Sprintf(format, args...)
}
