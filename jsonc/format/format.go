// Package format re-lays out a JSONC document in one consistent style and
// minifies it to plain JSON.
//
// Format promises, pinned by tests and fuzzing:
//
//   - every comment is kept, with its text (a line comment loses only its
//     trailing blanks); a comment in an odd spot (between a key and its value,
//     or before a comma) moves to the nearest line it can own;
//   - the meaning is unchanged: json.Compact of JSON() is byte-equal before
//     and after, so key order, duplicates, number spelling and string escapes
//     all survive;
//   - the output re-parses with no errors under the document's own options;
//   - it is idempotent: formatting the output again returns it unchanged;
//   - a Strict() document never gains a comment or a trailing comma.
//
// The layout is the one common editors produce: every member and element on
// a line of its own, `"key": value`, one blank line kept where the source had
// one or more between members, empty containers as `{}` / `[]`, the
// document's line ending (LF or CRLF) and a final newline.
package format

import (
	"bytes"
	"encoding/json"
	"strings"

	"github.com/ubgo/textdata/jsonc"
)

// Style is how Format lays a document out. The zero Style is the default:
// two spaces, no trailing commas, a final newline.
type Style struct {
	// Indent is the number of spaces per level; below 1 means 2. Ignored
	// when Tabs is set.
	Indent int
	// Tabs indents with one tab per level.
	Tabs bool
	// TrailingCommas puts a comma after the last member or element of every
	// non-empty container. Ignored for a Strict() document.
	TrailingCommas bool
	// NoFinalNewline leaves the newline after the last line out.
	NoFinalNewline bool
}

// DefaultStyle is the layout most editors write: two spaces, no trailing
// commas, a final newline.
func DefaultStyle() Style { return Style{Indent: 2} }

// unit is one level of indentation.
func (s Style) unit() string {
	if s.Tabs {
		return "\t"
	}
	if s.Indent < 1 {
		return "  "
	}
	return strings.Repeat(" ", s.Indent)
}

// Format returns d laid out in style s, keeping every comment. A document
// with syntax errors is refused with an error wrapping jsonc.ErrInvalid. The
// document itself is not changed; parse the result to keep editing it.
func Format(d *jsonc.Doc, s Style) ([]byte, error) {
	if _, err := d.JSON(); err != nil {
		return nil, err
	}
	src := d.Render()
	bomPrefix := ""
	if bytes.HasPrefix(src, []byte(bom)) {
		bomPrefix, src = bom, src[len(bom):]
	}
	p := &parser{toks: lex(src)}
	doc := p.document()
	pr := &printer{
		unit:     s.unit(),
		eol:      eolOf(src),
		trailing: s.TrailingCommas && !d.IsStrict(),
	}
	pr.b.WriteString(bomPrefix)
	pr.document(doc)
	if pr.b.Len() > len(bomPrefix) && !s.NoFinalNewline {
		pr.b.WriteString(pr.eol)
	}
	return []byte(pr.b.String()), nil
}

// Minify returns d as plain JSON with no insignificant whitespace: byte for
// byte json.Compact of d.JSON(). Comments and trailing commas are dropped.
// A document with syntax errors is refused (jsonc.ErrInvalid).
func Minify(d *jsonc.Doc) ([]byte, error) {
	j, err := d.JSON()
	if err != nil {
		return nil, err
	}
	var b bytes.Buffer
	if err := json.Compact(&b, j); err != nil {
		return nil, err // unreachable: JSON() of a valid document is JSON
	}
	return b.Bytes(), nil
}

// bom is the UTF-8 byte order mark, kept when the document has one.
const bom = "\xef\xbb\xbf"

// eolOf is the document's line ending: that of its first line break
// outside a block comment and a string (a line comment's own break counts),
// else of its first line break, else
// LF. A break inside a block comment is the comment's text, not the file's
// layout, so a file whose only CRLF sits in a comment is formatted as LF.
func eolOf(src []byte) string {
	first := -1
	for i := 0; i < len(src); i++ {
		switch c := src[i]; {
		case c == '\n':
			return breakAt(src, i)
		case c == '"':
			for i++; i < len(src) && src[i] != '"'; i++ {
				if src[i] == '\\' {
					i++
				}
			}
		case c == '/' && i+1 < len(src) && src[i+1] == '/':
			// A line comment runs to its line break, which is the file's.
			for i+1 < len(src) && src[i+1] != '\n' {
				i++
			}
		case c == '/' && i+1 < len(src) && src[i+1] == '*':
			end := bytes.Index(src[i+2:], []byte("*/"))
			stop := len(src)
			if end >= 0 {
				stop = i + 2 + end + 1
			}
			if first < 0 {
				if j := bytes.IndexByte(src[i:stop], '\n'); j >= 0 {
					first = i + j
				}
			}
			i = stop
		}
	}
	if first >= 0 {
		return breakAt(src, first)
	}
	return "\n"
}

// breakAt is the line break whose '\n' is at nl: CRLF or LF.
func breakAt(src []byte, nl int) string {
	if nl > 0 && src[nl-1] == '\r' {
		return "\r\n"
	}
	return "\n"
}
