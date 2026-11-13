package jsonc

import (
	"fmt"
	"sort"
	"strings"

	"github.com/ubgo/textdata/pos"
)

// LintRule names one Lint check. The set is closed: the constants below.
type LintRule string

// The lint rules.
const (
	// LintDuplicateKey: an object repeats a key. Readers keep the last one
	// (like encoding/json), so the earlier ones are dead text that looks live.
	LintDuplicateKey LintRule = "duplicate-key"
	// LintTrailingComma: a comma after the last member or element. Reported
	// only with ForbidTrailingCommas; JSONC allows it.
	LintTrailingComma LintRule = "trailing-comma"
	// LintMixedIndent: a member's indentation mixes tabs and spaces, or is
	// not its container's indentation plus the file's indent unit.
	LintMixedIndent LintRule = "mixed-indent"
	// LintOddComment: a comment between a key and its value or between a
	// value and its comma. It belongs to no member, so formatters and other
	// tools tend to move or drop it.
	LintOddComment LintRule = "odd-comment"
)

// AllLintRules returns every rule.
func AllLintRules() []LintRule {
	return []LintRule{LintDuplicateKey, LintTrailingComma, LintMixedIndent, LintOddComment}
}

// Issue is one Lint finding.
type Issue struct {
	File string // the document's file name, or ""
	Pos  pos.Pos
	Rule LintRule
	Msg  string
}

// String formats the issue as `file:line:col: message (rule)`.
func (i Issue) String() string {
	return (&pos.Error{File: i.File, Pos: i.Pos, Msg: fmt.Sprintf("%s (%s)", i.Msg, i.Rule)}).Error()
}

// LintOption configures Lint.
type LintOption func(*lintOptions)

type lintOptions struct {
	rules          map[LintRule]bool // nil: all
	forbidTrailing bool
}

// LintOnly limits Lint to the given rules.
func LintOnly(rules ...LintRule) LintOption {
	return func(o *lintOptions) {
		o.rules = map[LintRule]bool{}
		for _, r := range rules {
			o.rules[r] = true
		}
	}
}

// ForbidTrailingCommas makes LintTrailingComma report every trailing comma,
// for a style (or a consumer) that does not accept them.
func ForbidTrailingCommas() LintOption { return func(o *lintOptions) { o.forbidTrailing = true } }

// Lint checks style and likely mistakes that still parse: duplicate keys,
// trailing commas (with ForbidTrailingCommas), inconsistent indentation and
// comments in places tools lose them. Issues come sorted by position. Syntax
// errors are not lint issues (see Doc.Err); on a document with syntax errors
// Lint checks the part that parsed. Lint never changes the document.
func Lint(d *Doc, opts ...LintOption) []Issue {
	var o lintOptions
	for _, opt := range opts {
		if opt != nil {
			opt(&o)
		}
	}
	on := func(r LintRule) bool { return o.rules == nil || o.rules[r] }
	l := &linter{d: d, idx: pos.NewIndex(d.src)}
	unit := d.indentUnit()
	walk(d.root, func(n *node) {
		if on(LintDuplicateKey) && n.kind == KindObject {
			l.duplicates(n)
		}
		if on(LintTrailingComma) && o.forbidTrailing && n.close >= 0 {
			if last := lastItem(n); last != nil && last.comma >= 0 {
				l.add(last.comma, LintTrailingComma, "trailing comma after the last %s", member(n))
			}
		}
		if on(LintMixedIndent) {
			l.indent(n, unit)
		}
		if on(LintOddComment) {
			l.oddComments(n)
		}
	})
	sort.SliceStable(l.issues, func(i, j int) bool { return l.issues[i].Pos.Offset < l.issues[j].Pos.Offset })
	return l.issues
}

type linter struct {
	d      *Doc
	idx    *pos.Index
	issues []Issue
	lines  map[int]bool // lines already reported by mixed-indent
}

func (l *linter) add(off int, r LintRule, format string, args ...any) {
	l.issues = append(l.issues, Issue{File: l.d.opts.file, Pos: l.idx.Pos(off), Rule: r, Msg: fmt.Sprintf(format, args...)})
}

func member(n *node) string {
	if n.kind == KindObject {
		return "member"
	}
	return "element"
}

func (l *linter) duplicates(n *node) {
	first := map[string]int{}
	for _, it := range n.items {
		if at, ok := first[it.key]; ok {
			l.add(it.keyStart, LintDuplicateKey, "duplicate key %q (first at %s); the last one wins", it.key, l.idx.Pos(at))
			continue
		}
		first[it.key] = it.keyStart
	}
}

// indent checks every item that starts its own line against its container's
// indentation plus unit, and for a mix of tabs and spaces.
func (l *linter) indent(n *node, unit string) {
	src := l.d.src
	outer := indentOf(src, n.open)
	for _, it := range n.items {
		at := it.start()
		if !ownLineStart(src, at) {
			continue
		}
		ls := lineStart(src, at)
		if l.lines == nil {
			l.lines = map[int]bool{}
		}
		if l.lines[ls] {
			continue
		}
		inner := indentOf(src, at)
		switch {
		case strings.Contains(inner, " ") && strings.Contains(inner, "\t"):
			l.lines[ls] = true
			l.add(ls, LintMixedIndent, "indentation mixes tabs and spaces")
		case strings.Contains(outer, " ") && strings.Contains(outer, "\t"):
			// The container's own line is mixed and reported on its own.
		case inner != outer+unit:
			l.lines[ls] = true
			l.add(ls, LintMixedIndent, "indented %s, expected %s (the file indents by %s)", describeIndent(inner), describeIndent(outer+unit), describeIndent(unit))
		}
	}
}

// describeIndent names an indentation for a message: "4 spaces", "1 tab".
func describeIndent(s string) string {
	switch {
	case s == "":
		return "not at all"
	case strings.Trim(s, " ") == "":
		return plural(len(s), "space")
	case strings.Trim(s, "\t") == "":
		return plural(len(s), "tab")
	}
	return fmt.Sprintf("%q", s)
}

func plural(n int, what string) string {
	if n == 1 {
		return "1 " + what
	}
	return fmt.Sprintf("%d %ss", n, what)
}

// oddComments reports comments between a key and its value, and between a
// value and its comma.
func (l *linter) oddComments(n *node) {
	cs := directComments(n, l.d.comments)
	for _, c := range cs {
		for _, it := range n.items {
			switch {
			case it.keyStart >= 0 && c.start >= it.keyEnd && c.start < it.value.start:
				l.add(c.start, LintOddComment, "comment between key %q and its value; formatters and other tools may move or drop it", it.key)
			case it.comma >= 0 && c.start >= it.value.end && c.start < it.comma:
				l.add(c.start, LintOddComment, "comment between a value and its comma; formatters and other tools may move or drop it")
			default:
				continue
			}
			break
		}
	}
}
