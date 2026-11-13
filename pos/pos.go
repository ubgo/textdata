// Package pos maps byte offsets to line and column and renders errors as
// file:line:col.
//
// Every textdata format reports errors through this package so a message reads
// the same whichever format raised it: "config.jsonc:12:5: missing comma".
package pos

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// Pos is a position in a source text.
//
// Line and Col are 1-based; Col counts bytes, not runes, so it matches what
// editors that jump to "line:col" by byte do and stays exact for invalid UTF-8.
// The zero Pos means "no position".
type Pos struct {
	// Offset is the 0-based byte offset into the source.
	Offset int
	// Line is the 1-based line number.
	Line int
	// Col is the 1-based byte column.
	Col int
}

// IsValid reports whether p carries a position (Line > 0).
func (p Pos) IsValid() bool { return p.Line > 0 }

// String renders "line:col", or "-" for the zero Pos.
func (p Pos) String() string {
	if !p.IsValid() {
		return "-"
	}
	return strconv.Itoa(p.Line) + ":" + strconv.Itoa(p.Col)
}

// Index maps byte offsets of one source to positions. Build it once per source
// with NewIndex; lookups are O(log lines).
//
// A line ends at '\n'. A '\r' before it belongs to the line, so CRLF files
// report the same line numbers as LF files.
type Index struct {
	// starts holds the offset of the first byte of each line.
	starts []int
	size   int
}

// NewIndex scans src once and records where each line starts.
func NewIndex(src []byte) *Index {
	starts := []int{0}
	for i, c := range src {
		if c == '\n' {
			starts = append(starts, i+1)
		}
	}
	return &Index{starts: starts, size: len(src)}
}

// Pos returns the position of offset. Offsets outside the source are clamped
// to its bounds, so a position is always printable.
func (x *Index) Pos(offset int) Pos {
	if offset < 0 {
		offset = 0
	}
	if offset > x.size {
		offset = x.size
	}
	line := sort.Search(len(x.starts), func(i int) bool { return x.starts[i] > offset }) - 1
	return Pos{Offset: offset, Line: line + 1, Col: offset - x.starts[line] + 1}
}

// Lines returns how many lines the source has (a trailing newline starts an
// empty last line, as editors show it).
func (x *Index) Lines() int { return len(x.starts) }

// Error is one problem at one position in one file.
type Error struct {
	// File is the file name as the caller wants it shown; empty for sources
	// that did not come from a file.
	File string
	// Pos is where the problem is.
	Pos Pos
	// Msg is the plain message, without position or file.
	Msg string
	// Err is an optional cause, reachable through errors.Is/As.
	Err error
}

// Error renders "file:line:col: msg", dropping the parts that are unknown.
func (e *Error) Error() string {
	var b strings.Builder
	if e.File != "" {
		b.WriteString(e.File)
		b.WriteByte(':')
	}
	if e.Pos.IsValid() {
		b.WriteString(e.Pos.String())
		b.WriteByte(':')
	}
	if b.Len() > 0 {
		b.WriteByte(' ')
	}
	b.WriteString(e.Msg)
	if e.Err != nil {
		if e.Msg != "" {
			b.WriteString(": ")
		}
		b.WriteString(e.Err.Error())
	}
	return b.String()
}

// Unwrap returns the cause.
func (e *Error) Unwrap() error { return e.Err }

// Errorf builds an *Error with a formatted message.
func Errorf(file string, p Pos, format string, args ...any) *Error {
	return &Error{File: file, Pos: p, Msg: fmt.Sprintf(format, args...)}
}

// ErrorList is every problem found in one source, in source order.
type ErrorList []*Error

// Add appends an error.
func (l *ErrorList) Add(e *Error) { *l = append(*l, e) }

// Sort orders the list by file, then offset. Stable, so errors at the same
// position keep the order they were found in.
func (l ErrorList) Sort() {
	sort.SliceStable(l, func(i, j int) bool {
		if l[i].File != l[j].File {
			return l[i].File < l[j].File
		}
		return l[i].Pos.Offset < l[j].Pos.Offset
	})
}

// Error renders the first error and how many more there are.
func (l ErrorList) Error() string {
	switch len(l) {
	case 0:
		return "no errors"
	case 1:
		return l[0].Error()
	}
	if len(l) == 2 {
		return l[0].Error() + " (and 1 more error)"
	}
	return fmt.Sprintf("%s (and %d more errors)", l[0].Error(), len(l)-1)
}

// Err returns nil for an empty list and the list otherwise, so a function can
// `return list.Err()` without returning a typed-nil error.
func (l ErrorList) Err() error {
	if len(l) == 0 {
		return nil
	}
	return l
}

// Unwrap exposes every error to errors.Is and errors.As.
func (l ErrorList) Unwrap() []error {
	out := make([]error, len(l))
	for i, e := range l {
		out[i] = e
	}
	return out
}

// First returns the first *Error inside err (an *Error or an ErrorList), or
// nil.
func First(err error) *Error {
	var e *Error
	if errors.As(err, &e) {
		return e
	}
	return nil
}
