package pos

import (
	"errors"
	"io"
	"testing"
)

func TestIndexPos(t *testing.T) {
	t.Parallel()
	src := []byte("ab\r\ncd\n\nx")
	x := NewIndex(src)
	tests := []struct {
		off  int
		want Pos
	}{
		{0, Pos{0, 1, 1}},
		{2, Pos{2, 1, 3}},  // the \r belongs to line 1
		{3, Pos{3, 1, 4}},  // the \n too
		{4, Pos{4, 2, 1}},  // c
		{7, Pos{7, 3, 1}},  // empty line
		{8, Pos{8, 4, 1}},  // x
		{9, Pos{9, 4, 2}},  // end of input
		{99, Pos{9, 4, 2}}, // clamped
		{-5, Pos{0, 1, 1}}, // clamped
	}
	for _, tt := range tests {
		if got := x.Pos(tt.off); got != tt.want {
			t.Errorf("Pos(%d) = %+v, want %+v", tt.off, got, tt.want)
		}
	}
	if x.Lines() != 4 {
		t.Errorf("Lines = %d, want 4", x.Lines())
	}
	if got := NewIndex(nil).Pos(0); got != (Pos{0, 1, 1}) {
		t.Errorf("empty source Pos(0) = %+v", got)
	}
}

func TestErrorString(t *testing.T) {
	t.Parallel()
	p := Pos{Offset: 10, Line: 12, Col: 5}
	tests := []struct {
		err  *Error
		want string
	}{
		{&Error{File: "config.jsonc", Pos: p, Msg: `missing comma after "port"`}, `config.jsonc:12:5: missing comma after "port"`},
		{&Error{Pos: p, Msg: "bad"}, "12:5: bad"},
		{&Error{File: "f", Msg: "bad"}, "f: bad"},
		{&Error{Msg: "bad"}, "bad"},
		{&Error{File: "f", Pos: p, Msg: "read", Err: io.EOF}, "f:12:5: read: EOF"},
		{&Error{Err: io.EOF}, "EOF"},
	}
	for _, tt := range tests {
		if got := tt.err.Error(); got != tt.want {
			t.Errorf("Error() = %q, want %q", got, tt.want)
		}
	}
	if !errors.Is(&Error{Err: io.EOF}, io.EOF) {
		t.Error("errors.Is through Unwrap failed")
	}
	if Pos.String(Pos{}) != "-" {
		t.Error("zero Pos String")
	}
}

func TestErrorList(t *testing.T) {
	t.Parallel()
	var l ErrorList
	if l.Err() != nil {
		t.Fatal("empty list Err() != nil")
	}
	if l.Error() != "no errors" {
		t.Errorf("empty Error = %q", l.Error())
	}
	l.Add(Errorf("b", Pos{5, 1, 6}, "second %d", 2))
	l.Add(Errorf("a", Pos{9, 2, 1}, "first"))
	l.Add(&Error{File: "b", Pos: Pos{1, 1, 2}, Msg: "third", Err: io.ErrUnexpectedEOF})
	l.Sort()
	if l[0].Msg != "first" || l[1].Msg != "third" || l[2].Msg != "second 2" {
		t.Errorf("Sort order = %q %q %q", l[0].Msg, l[1].Msg, l[2].Msg)
	}
	if got, want := l.Error(), "a:2:1: first (and 2 more errors)"; got != want {
		t.Errorf("Error = %q, want %q", got, want)
	}
	err := l.Err()
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Error("errors.Is through the list failed")
	}
	if First(err) != l[0] {
		t.Error("First returned the wrong error")
	}
	if First(io.EOF) != nil {
		t.Error("First of a foreign error != nil")
	}
	if ErrorList(l[:1]).Error() != "a:2:1: first" {
		t.Error("single-element Error")
	}
}

// TestErrorListCountWording pins the singular for one more error and the
// plural otherwise.
func TestErrorListCountWording(t *testing.T) {
	var l ErrorList
	l.Add(Errorf("f", Pos{0, 1, 1}, "first"))
	l.Add(Errorf("f", Pos{1, 1, 2}, "second"))
	if got, want := l.Error(), "f:1:1: first (and 1 more error)"; got != want {
		t.Errorf("Error = %q, want %q", got, want)
	}
	l.Add(Errorf("f", Pos{2, 1, 3}, "third"))
	if got, want := l.Error(), "f:1:1: first (and 2 more errors)"; got != want {
		t.Errorf("Error = %q, want %q", got, want)
	}
}
