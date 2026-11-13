package jsonc

import (
	"fmt"
	"sort"
	"strings"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/ubgo/textdata/pos"
)

// maxDepth is the deepest nesting accepted, the same limit encoding/json uses,
// so a hostile file cannot overflow the stack and Strict() agrees with
// json.Valid on every input.
const maxDepth = 10000

// bom is the UTF-8 byte order mark. JSONC accepts and keeps it; strict JSON
// rejects it, as encoding/json does.
const bom = "\xef\xbb\xbf"

// Kind is the type of a JSON value.
type Kind int

// The value kinds.
const (
	KindInvalid Kind = iota
	KindObject
	KindArray
	KindString
	KindNumber
	KindBool
	KindNull
)

// String implements fmt.Stringer.
func (k Kind) String() string {
	switch k {
	case KindObject:
		return "object"
	case KindArray:
		return "array"
	case KindString:
		return "string"
	case KindNumber:
		return "number"
	case KindBool:
		return "bool"
	case KindNull:
		return "null"
	default:
		return "invalid"
	}
}

// node is one value: a span over the source plus, for containers, its items.
// The source is the only copy of the text; nodes never hold their own bytes
// except a string's decoded value.
type node struct {
	kind       Kind
	start, end int // value span [start, end)
	// open and close are the bracket offsets of a container; close is -1 when
	// the container is unterminated (tolerant parse).
	open, close int
	items       []*item
	str         string // decoded value of a KindString
}

// item is one object member or array element.
type item struct {
	// key is the decoded member name; keyStart is -1 for array elements.
	key              string
	keyStart, keyEnd int
	value            *node
	// comma is the offset of the comma after the value, or -1.
	comma int
	// leading are the comment lines directly above the item; trailing is the
	// comment starting on the line its value ends on.
	leading  []comment
	trailing *comment
}

// start is where the item's own text begins (its key, or its value).
func (it *item) start() int {
	if it.keyStart >= 0 {
		return it.keyStart
	}
	return it.value.start
}

// spanStart includes the attached leading comments.
func (it *item) spanStart() int {
	if len(it.leading) > 0 {
		return it.leading[0].start
	}
	return it.start()
}

// tailEnd is the end of the value, its comma and its trailing comment.
func (it *item) tailEnd() int {
	end := it.value.end
	if it.comma >= 0 && it.comma+1 > end {
		end = it.comma + 1
	}
	if it.trailing != nil && it.trailing.end > end {
		end = it.trailing.end
	}
	return end
}

// comment is a // line or /* block */ comment span. A line comment's end
// excludes the newline.
type comment struct {
	start, end int
	block      bool
}

// text returns the comment's content without markers and surrounding spaces.
func (c comment) text(src []byte) string {
	s := string(src[c.start:c.end])
	if c.block {
		s = strings.TrimSuffix(strings.TrimPrefix(s, "/*"), "*/")
	} else {
		s = strings.TrimPrefix(s, "//")
	}
	return strings.TrimSpace(s)
}

// parser is a tolerant recursive-descent parser. It stops at the first
// syntax error, keeping every node built so far.
type parser struct {
	src      []byte
	i        int
	strict   bool
	file     string
	idx      *pos.Index
	errs     pos.ErrorList
	failed   bool
	comments []comment
	depth    int
}

// parse builds the tree for src. root is nil for an empty (or comment-only)
// document.
func parse(src []byte, strict bool, file string) (root *node, comments []comment, errs pos.ErrorList) {
	p := &parser{src: src, strict: strict, file: file}
	if strings.HasPrefix(string(src[:min(len(src), len(bom))]), bom) {
		if strict {
			p.errorf(0, "byte order mark is not allowed in strict JSON")
		}
		p.i = len(bom)
	}
	p.skipTrivia()
	if p.i >= len(src) {
		if strict && !p.failed {
			p.errorf(p.i, "empty document: expected a JSON value")
		}
		return nil, p.comments, p.errs
	}
	if !p.failed {
		root = p.value()
	}
	if !p.failed {
		p.skipTrivia()
		if p.i < len(src) && !p.failed {
			p.errorf(p.i, "unexpected %s after the end of the document", p.describe(p.i))
		}
	}
	if root != nil {
		attach(src, root, p.comments)
	}
	return root, p.comments, p.errs
}

func (p *parser) errorf(at int, format string, args ...any) {
	if p.idx == nil {
		p.idx = pos.NewIndex(p.src)
	}
	p.errs.Add(&pos.Error{File: p.file, Pos: p.idx.Pos(at), Msg: fmt.Sprintf(format, args...)})
	p.failed = true
}

// describe names the byte at i for an error message.
func (p *parser) describe(i int) string {
	if i >= len(p.src) {
		return "end of input"
	}
	r, _ := utf8.DecodeRune(p.src[i:])
	if r == utf8.RuneError {
		return fmt.Sprintf("byte 0x%02x", p.src[i])
	}
	return fmt.Sprintf("%q", r)
}

// skipTrivia skips whitespace and comments, recording comments.
func (p *parser) skipTrivia() {
	for p.i < len(p.src) && !p.failed {
		switch c := p.src[p.i]; c {
		case ' ', '\t', '\n', '\r':
			p.i++
		case '/':
			if p.i+1 >= len(p.src) || (p.src[p.i+1] != '/' && p.src[p.i+1] != '*') {
				return
			}
			start := p.i
			if p.strict {
				p.errorf(start, "comments are not allowed in strict JSON")
				return
			}
			if p.src[p.i+1] == '/' {
				end := start
				for end < len(p.src) && p.src[end] != '\n' {
					end++
				}
				if end > start && p.src[end-1] == '\r' && end < len(p.src) {
					end-- // the \r of a CRLF belongs to the line break
				}
				p.comments = append(p.comments, comment{start: start, end: end})
				p.i = end
				continue
			}
			close := strings.Index(string(p.src[start+2:]), "*/")
			if close < 0 {
				p.errorf(start, "unterminated block comment")
				return
			}
			end := start + 2 + close + 2
			p.comments = append(p.comments, comment{start: start, end: end, block: true})
			p.i = end
		default:
			return
		}
	}
}

// value parses one value at p.i (trivia already skipped).
func (p *parser) value() *node {
	if p.i >= len(p.src) {
		p.errorf(p.i, "unexpected end of input: expected a value")
		return nil
	}
	switch c := p.src[p.i]; {
	case c == '{':
		return p.container(KindObject, '}')
	case c == '[':
		return p.container(KindArray, ']')
	case c == '"':
		start := p.i
		s, ok := p.str()
		if !ok {
			return nil
		}
		return &node{kind: KindString, start: start, end: p.i, str: s}
	case c == '-' || (c >= '0' && c <= '9'):
		return p.number()
	case c == '\'':
		p.errorf(p.i, "single-quoted strings are not JSON (that is JSON5); use double quotes")
		return nil
	default:
		return p.literal()
	}
}

func (p *parser) container(kind Kind, closeCh byte) *node {
	n := &node{kind: kind, start: p.i, open: p.i, close: -1}
	p.depth++
	defer func() { p.depth-- }()
	if p.depth > maxDepth {
		p.errorf(p.i, "nesting deeper than %d levels", maxDepth)
		return n
	}
	p.i++
	what := "array"
	if kind == KindObject {
		what = "object"
	}
	for {
		p.skipTrivia()
		if p.failed {
			n.end = p.i
			return n
		}
		if p.i >= len(p.src) {
			p.errorf(n.open, "unclosed %s: missing %q", what, closeCh)
			n.end = p.i
			return n
		}
		if p.src[p.i] == closeCh {
			if last := lastItem(n); last != nil && last.comma >= 0 && p.strict {
				p.errorf(last.comma, "trailing comma is not allowed in strict JSON")
			}
			n.close = p.i
			p.i++
			n.end = p.i
			return n
		}
		if p.src[p.i] == ',' {
			p.errorf(p.i, "unexpected ',': expected a value or %q", closeCh)
			n.end = p.i
			return n
		}
		if last := lastItem(n); last != nil && last.comma < 0 {
			p.errorf(p.i, "missing comma before %s", p.describe(p.i))
			n.end = p.i
			return n
		}
		it := &item{keyStart: -1, keyEnd: -1, comma: -1}
		if kind == KindObject {
			if !p.key(it) {
				n.end = p.i
				return n
			}
		}
		v := p.value()
		if v == nil {
			n.end = p.i
			return n
		}
		it.value = v
		n.items = append(n.items, it)
		p.skipTrivia()
		if p.failed {
			n.end = p.i
			return n
		}
		if p.i < len(p.src) && p.src[p.i] == ',' {
			it.comma = p.i
			p.i++
		}
	}
}

func lastItem(n *node) *item {
	if len(n.items) == 0 {
		return nil
	}
	return n.items[len(n.items)-1]
}

// key parses `"name" :` into it.
func (p *parser) key(it *item) bool {
	switch c := p.src[p.i]; {
	case c == '"':
	case c == '\'':
		p.errorf(p.i, "single-quoted keys are not JSON (that is JSON5); use double quotes")
		return false
	case isIdentStart(c):
		p.errorf(p.i, "object keys must be double-quoted (unquoted keys are JSON5)")
		return false
	default:
		p.errorf(p.i, "unexpected %s: expected a double-quoted key", p.describe(p.i))
		return false
	}
	it.keyStart = p.i
	s, ok := p.str()
	if !ok {
		return false
	}
	it.key, it.keyEnd = s, p.i
	p.skipTrivia()
	if p.failed {
		return false
	}
	if p.i >= len(p.src) || p.src[p.i] != ':' {
		p.errorf(p.i, "missing ':' after key %q", s)
		return false
	}
	p.i++
	p.skipTrivia()
	return !p.failed
}

// str scans a string starting at the opening quote and returns its decoded
// value. Invalid UTF-8 is kept as raw bytes, as encoding/json accepts it.
func (p *parser) str() (string, bool) {
	start := p.i
	// Fast path: no escapes and no control characters, one allocation.
	for j := start + 1; j < len(p.src); j++ {
		c := p.src[j]
		if c == '"' {
			p.i = j + 1
			return string(p.src[start+1 : j]), true
		}
		if c == '\\' || c < 0x20 {
			break
		}
	}
	p.i++
	var b strings.Builder
	for p.i < len(p.src) {
		c := p.src[p.i]
		switch {
		case c == '"':
			p.i++
			return b.String(), true
		case c < 0x20:
			p.errorf(p.i, "control character 0x%02x in string: escape it", c)
			return "", false
		case c == '\\':
			if p.i+1 >= len(p.src) {
				p.i++
				continue // reported as unterminated below
			}
			e := p.src[p.i+1]
			switch e {
			case '"', '\\', '/':
				b.WriteByte(e)
			case 'b':
				b.WriteByte('\b')
			case 'f':
				b.WriteByte('\f')
			case 'n':
				b.WriteByte('\n')
			case 'r':
				b.WriteByte('\r')
			case 't':
				b.WriteByte('\t')
			case 'u':
				r, ok := p.hex4(p.i + 2)
				if !ok {
					p.errorf(p.i, `invalid \u escape: need four hex digits`)
					return "", false
				}
				p.i += 6
				if utf16.IsSurrogate(r) {
					if r2, ok2 := p.hex4(p.i + 2); ok2 && p.i+1 < len(p.src) && p.src[p.i] == '\\' && p.src[p.i+1] == 'u' {
						if dec := utf16.DecodeRune(r, r2); dec != utf8.RuneError {
							b.WriteRune(dec)
							p.i += 6
							continue
						}
					}
					r = utf8.RuneError
				}
				b.WriteRune(r)
				continue
			default:
				p.errorf(p.i, "invalid escape %q in string", "\\"+string(rune(e)))
				return "", false
			}
			p.i += 2
		default:
			b.WriteByte(c)
			p.i++
		}
	}
	p.errorf(start, "unterminated string")
	return "", false
}

func (p *parser) hex4(at int) (rune, bool) {
	if at+4 > len(p.src) {
		return 0, false
	}
	var r rune
	for _, c := range p.src[at : at+4] {
		var d byte
		switch {
		case c >= '0' && c <= '9':
			d = c - '0'
		case c >= 'a' && c <= 'f':
			d = c - 'a' + 10
		case c >= 'A' && c <= 'F':
			d = c - 'A' + 10
		default:
			return 0, false
		}
		r = r<<4 | rune(d)
	}
	return r, true
}

// number scans -?(0|[1-9]\d*)(\.\d+)?([eE][+-]?\d+)?
func (p *parser) number() *node {
	start := p.i
	if p.src[p.i] == '-' {
		p.i++
	}
	digits := func() int {
		n := 0
		for p.i < len(p.src) && p.src[p.i] >= '0' && p.src[p.i] <= '9' {
			p.i++
			n++
		}
		return n
	}
	switch {
	case p.i >= len(p.src):
		p.errorf(start, "invalid number: '-' needs digits")
		return nil
	case p.src[p.i] == '0':
		p.i++
		if p.i < len(p.src) {
			switch c := p.src[p.i]; {
			case c >= '0' && c <= '9':
				p.errorf(start, "invalid number: leading zeros are not allowed")
				return nil
			case c == 'x' || c == 'X':
				p.errorf(start, "hex numbers are not JSON (that is JSON5)")
				return nil
			}
		}
	case p.src[p.i] >= '1' && p.src[p.i] <= '9':
		digits()
	case p.src[p.i] == 'I':
		p.errorf(start, "Infinity is not JSON (that is JSON5)")
		return nil
	default:
		p.errorf(start, "invalid number: '-' needs digits")
		return nil
	}
	if p.i < len(p.src) && p.src[p.i] == '.' {
		p.i++
		if digits() == 0 {
			p.errorf(start, "invalid number: '.' needs digits after it")
			return nil
		}
	}
	if p.i < len(p.src) && (p.src[p.i] == 'e' || p.src[p.i] == 'E') {
		p.i++
		if p.i < len(p.src) && (p.src[p.i] == '+' || p.src[p.i] == '-') {
			p.i++
		}
		if digits() == 0 {
			p.errorf(start, "invalid number: exponent needs digits")
			return nil
		}
	}
	return &node{kind: KindNumber, start: start, end: p.i}
}

// literal parses true, false or null, and names the JSON5 forms it refuses.
func (p *parser) literal() *node {
	start := p.i
	end := p.i
	for end < len(p.src) && isIdentChar(p.src[end]) {
		end++
	}
	word := string(p.src[start:end])
	switch word {
	case "true", "false":
		p.i = end
		return &node{kind: KindBool, start: start, end: end}
	case "null":
		p.i = end
		return &node{kind: KindNull, start: start, end: end}
	case "NaN", "Infinity":
		p.errorf(start, "%s is not JSON (that is JSON5)", word)
	case "":
		p.errorf(start, "unexpected %s: expected a value", p.describe(start))
	default:
		p.errorf(start, "unexpected word %q: expected a value (strings need double quotes)", word)
	}
	return nil
}

func isIdentStart(c byte) bool {
	return c == '_' || c == '$' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func isIdentChar(c byte) bool { return isIdentStart(c) || (c >= '0' && c <= '9') }

// attach assigns comments to the items they describe, recursively.
//
// Trailing: the first comment in the gap after an item that starts on the
// line its value ends on. Leading: the run of comments directly above an
// item, each on its own line, with no blank line inside the run or between
// it and the item. Comments that fit neither stay unattached.
func attach(src []byte, n *node, all []comment) {
	if n.kind != KindObject && n.kind != KindArray {
		return
	}
	end := n.close
	if end < 0 {
		end = n.end
	}
	// Comments directly inside n: within its brackets, outside every child.
	lo := sort.Search(len(all), func(i int) bool { return all[i].start > n.open })
	var direct []comment
	k := 0
	for _, c := range all[lo:] {
		if c.start >= end {
			break
		}
		for k < len(n.items) && n.items[k].value.end <= c.start {
			k++
		}
		if k < len(n.items) && c.start >= n.items[k].value.start && c.start < n.items[k].value.end {
			continue
		}
		direct = append(direct, c)
	}

	ci := 0
	gapStart := n.open + 1
	for i, it := range n.items {
		// Comments before this item (after the previous item's value).
		var before []comment
		for ci < len(direct) && direct[ci].start < it.start() {
			c := direct[ci]
			ci++
			if c.start < gapStart {
				continue
			}
			if i > 0 && n.items[i-1].trailing == nil && sameLine(src, n.items[i-1].value.end, c.start) {
				cc := c
				n.items[i-1].trailing = &cc
				continue
			}
			before = append(before, c)
		}
		it.leading = leadingRun(src, before, it.start())
		gapStart = it.value.end
		attach(src, it.value, all)
	}
	if last := lastItem(n); last != nil && last.trailing == nil {
		for ; ci < len(direct); ci++ {
			if c := direct[ci]; c.start >= last.value.end && sameLine(src, last.value.end, c.start) {
				cc := c
				last.trailing = &cc
				break
			}
		}
	}
}

// leadingRun returns the suffix of cs that sits directly above target.
func leadingRun(src []byte, cs []comment, target int) []comment {
	next := target
	first := len(cs)
	for j := len(cs) - 1; j >= 0; j-- {
		c := cs[j]
		if !ownLineStart(src, c.start) || strings.Count(string(src[c.end:next]), "\n") > 1 || !onlySpace(src[c.end:next]) {
			break
		}
		first = j
		next = c.start
	}
	if first == len(cs) {
		return nil
	}
	return append([]comment(nil), cs[first:]...)
}

// sameLine reports whether no newline lies between a and b.
func sameLine(src []byte, a, b int) bool {
	if a > b {
		a, b = b, a
	}
	return !strings.Contains(string(src[a:b]), "\n")
}

// ownLineStart reports whether only whitespace precedes off on its line.
func ownLineStart(src []byte, off int) bool {
	return onlySpace(src[lineStart(src, off):off])
}

// ownLineEnd reports whether only whitespace follows off on its line.
func ownLineEnd(src []byte, off int) bool {
	return onlySpace(src[off:lineEnd(src, off)])
}

func onlySpace(b []byte) bool {
	for _, c := range b {
		if c != ' ' && c != '\t' && c != '\r' && c != '\n' {
			return false
		}
	}
	return true
}

// lineStart is the offset of the first byte of off's line.
func lineStart(src []byte, off int) int {
	for off > 0 && src[off-1] != '\n' {
		off--
	}
	return off
}

// lineEnd is the offset of the '\n' ending off's line, or len(src).
func lineEnd(src []byte, off int) int {
	for off < len(src) && src[off] != '\n' {
		off++
	}
	return off
}

// indentOf returns the leading whitespace of off's line.
func indentOf(src []byte, off int) string {
	s := lineStart(src, off)
	e := s
	for e < len(src) && (src[e] == ' ' || src[e] == '\t') {
		e++
	}
	return string(src[s:e])
}
