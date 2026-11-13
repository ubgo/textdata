package format

import (
	"strings"
)

// tokKind is a lexical token kind.
type tokKind int

const (
	tokPunct   tokKind = iota // { } [ ] , :
	tokScalar                 // string, number, true, false, null
	tokComment                // // line or /* block */
)

// token is one lexical token. nl is the number of line breaks in the
// whitespace right before it, which decides own-line and blank-line layout.
type token struct {
	kind tokKind
	text string
	nl   int
}

// lex splits a valid JSONC document (BOM removed) into tokens. Validity is
// checked by the caller, so the lexer only needs to find token boundaries.
func lex(src []byte) []token {
	var toks []token
	nl := 0
	for i := 0; i < len(src); {
		c := src[i]
		switch {
		case c == '\n':
			nl++
			i++
		case c == ' ' || c == '\t' || c == '\r':
			i++
		case c == '/' && i+1 < len(src) && src[i+1] == '/':
			j := i
			for j < len(src) && src[j] != '\n' {
				j++
			}
			toks = append(toks, token{tokComment, strings.TrimRight(string(src[i:j]), " \t\r"), nl})
			nl, i = 0, j
		case c == '/' && i+1 < len(src) && src[i+1] == '*':
			j := strings.Index(string(src[i+2:]), "*/")
			end := len(src)
			if j >= 0 {
				end = i + 2 + j + 2
			}
			toks = append(toks, token{tokComment, string(src[i:end]), nl})
			nl, i = 0, end
		case c == '"':
			j := i + 1
			for j < len(src) && src[j] != '"' {
				if src[j] == '\\' {
					j++
				}
				j++
			}
			j = min(j+1, len(src))
			toks = append(toks, token{tokScalar, string(src[i:j]), nl})
			nl, i = 0, j
		case strings.IndexByte("{}[],:", c) >= 0:
			toks = append(toks, token{tokPunct, string(c), nl})
			nl, i = 0, i+1
		default:
			j := i
			for j < len(src) && strings.IndexByte(" \t\r\n{}[],:\"/", src[j]) < 0 {
				j++
			}
			if j == i {
				j++ // a lone '/' cannot occur in a valid document; never stall
			}
			toks = append(toks, token{tokScalar, string(src[i:j]), nl})
			nl, i = 0, j
		}
	}
	return toks
}

// cmt is a comment with the line breaks before it in the source.
type cmt struct {
	text string
	nl   int
}

func (c cmt) line() bool { return strings.HasPrefix(c.text, "//") }

// value is a scalar or a container.
type value struct {
	scalar    string // set for a scalar
	open      byte   // '{' or '[' for a container
	openTrail []cmt  // comments on the opening bracket's line
	items     []*member
	dangling  []cmt // comments after the last member, before the close
}

// member is an object member or an array element.
type member struct {
	lead  []cmt  // comment lines above it
	nl    int    // line breaks before its first token (key or value)
	key   string // raw key token; "" for an array element
	value *value
	trail []cmt // comments on its last line, after the comma
}

// doc is the whole document.
type doc struct {
	lead  []cmt
	nl    int
	root  *value // nil for an empty or comment-only document
	trail []cmt
	after []cmt
}

// parser builds the layout tree from tokens of a valid document.
type parser struct {
	toks []token
	i    int
}

func (p *parser) peek() (token, bool) {
	if p.i < len(p.toks) {
		return p.toks[p.i], true
	}
	return token{}, false
}

// comments consumes a run of comments.
func (p *parser) comments() []cmt {
	var out []cmt
	for p.i < len(p.toks) && p.toks[p.i].kind == tokComment {
		out = append(out, cmt{p.toks[p.i].text, p.toks[p.i].nl})
		p.i++
	}
	return out
}

// sameLine splits cs into the comments that stay on the current line (no
// line break before them, and none after a line comment) and the rest, which
// go on lines of their own.
func sameLine(cs []cmt) (trail, rest []cmt) {
	k := 0
	for k < len(cs) && cs[k].nl == 0 {
		k++
		if cs[k-1].line() {
			break
		}
	}
	rest = cs[k:]
	if len(rest) > 0 && rest[0].nl == 0 {
		rest = append([]cmt(nil), rest...)
		rest[0].nl = 1 // it followed a line comment: now on its own line
	}
	return cs[:k], rest
}

func (p *parser) document() *doc {
	d := &doc{lead: p.comments()}
	t, ok := p.peek()
	if !ok {
		return d
	}
	d.nl = t.nl
	d.root = p.value()
	d.trail, d.after = sameLine(p.comments())
	return d
}

func (p *parser) value() *value {
	t := p.toks[p.i]
	p.i++
	if t.kind != tokPunct {
		return &value{scalar: t.text}
	}
	v := &value{open: t.text[0]}
	closeCh := "}"
	if v.open == '[' {
		closeCh = "]"
	}
	var pending []cmt
	v.openTrail, pending = sameLine(p.comments())
	for {
		t, ok := p.peek()
		if !ok {
			return v // unreachable for a valid document
		}
		if t.kind == tokPunct && t.text == closeCh {
			p.i++
			v.dangling = pending
			return v
		}
		m := &member{lead: pending, nl: t.nl}
		pending = nil
		if v.open == '{' {
			m.key = t.text
			p.i++
			// Comments between key and colon, or colon and value, have no
			// line of their own there: they move above the member.
			m.lead = append(m.lead, ownLine(p.comments())...)
			p.i++ // ':'
			m.lead = append(m.lead, ownLine(p.comments())...)
		}
		m.value = p.value()
		post := p.comments()
		if t, ok := p.peek(); ok && t.kind == tokPunct && t.text == "," {
			p.i++
			// Comments before the comma end the member's line, wherever
			// they were; the comma moves in front of them.
			for i := range post {
				post[i].nl = 0
			}
			post = append(post, p.comments()...)
		}
		m.trail, pending = sameLine(post)
		v.items = append(v.items, m)
	}
}

// ownLine marks comments as starting their own line.
func ownLine(cs []cmt) []cmt {
	for i := range cs {
		if cs[i].nl == 0 {
			cs[i].nl = 1
		}
	}
	return cs
}

// printer writes the layout tree.
type printer struct {
	b        strings.Builder
	unit     string
	eol      string
	trailing bool
}

func (p *printer) document(d *doc) {
	first := true
	for _, c := range d.lead {
		p.ownLine(c.text, c.nl, "", first)
		first = false
	}
	if d.root != nil {
		if !first {
			p.newline(d.nl)
		}
		p.value(d.root, "")
		p.trail(d.trail)
	}
	for _, c := range d.after {
		p.ownLine(c.text, c.nl, "", first && d.root == nil)
		first = false
	}
}

// ownLine writes a comment on a line of its own. Except at the very start,
// it begins with a line break, two when the source had a blank line.
func (p *printer) ownLine(text string, nl int, indent string, atStart bool) {
	if !atStart {
		p.newline(nl)
	}
	p.b.WriteString(indent)
	p.b.WriteString(text)
}

// newline writes a line break, plus a blank line when nl >= 2.
func (p *printer) newline(nl int) {
	p.b.WriteString(p.eol)
	if nl >= 2 {
		p.b.WriteString(p.eol)
	}
}

func (p *printer) trail(cs []cmt) {
	for _, c := range cs {
		p.b.WriteByte(' ')
		p.b.WriteString(c.text)
	}
}

// value writes v; indent is the indentation of the line it starts on.
func (p *printer) value(v *value, indent string) {
	if v.open == 0 {
		p.b.WriteString(v.scalar)
		return
	}
	closeCh := byte('}')
	if v.open == '[' {
		closeCh = ']'
	}
	p.b.WriteByte(v.open)
	if len(v.items) == 0 && len(v.openTrail) == 0 && len(v.dangling) == 0 {
		p.b.WriteByte(closeCh)
		return
	}
	p.trail(v.openTrail)
	inner := indent + p.unit
	first := true // no blank line right after the opening bracket
	for i, m := range v.items {
		for _, c := range m.lead {
			p.ownLine(c.text, blank(c.nl, first), inner, false)
			first = false
		}
		p.newline(blank(m.nl, first))
		first = false
		p.b.WriteString(inner)
		if m.key != "" {
			p.b.WriteString(m.key)
			p.b.WriteString(": ")
		}
		p.value(m.value, inner)
		if i < len(v.items)-1 || p.trailing {
			p.b.WriteByte(',')
		}
		p.trail(m.trail)
	}
	for _, c := range v.dangling {
		p.ownLine(c.text, blank(c.nl, first), inner, false)
		first = false
	}
	p.newline(1)
	p.b.WriteString(indent)
	p.b.WriteByte(closeCh)
}

// blank drops a blank line at the top of a container.
func blank(nl int, first bool) int {
	if first {
		return 1
	}
	return nl
}
