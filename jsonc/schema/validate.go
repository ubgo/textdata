package schema

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/ubgo/textdata/jsonc"
	"github.com/ubgo/textdata/path"
	"github.com/ubgo/textdata/pos"
)

// maxRefHops bounds how many $ref a value may pass through without moving
// into a child value, so a reference loop ("a" → "b" → "a") ends with an
// error instead of a stack overflow.
const maxRefHops = 32

// maxChecks bounds the schema applications one Validate call makes, so a
// schema whose anyOf branches reference themselves cannot run for ever.
const maxChecks = 1_000_000

// maxListed bounds how many enum values a message lists.
const maxListed = 10

// Keywords used in issues that are not schema keywords.
const (
	keywordSyntax = "syntax"
	keywordFalse  = "false"
	rootLabel     = "(root)"
)

// Issue is one problem found by Validate.
type Issue struct {
	// File is the document's file name, when known.
	File string
	// Pos is where the offending value (or key) starts.
	Pos pos.Pos
	// Path is the value's canonical dotted path ("" for the root).
	Path string
	// Keyword is the schema keyword that failed ("type", "required", ...).
	Keyword string
	// Message is the plain description, without position or path.
	Message string

	// atKey: Pos is the member name's position, not the value's; positions
	// are filled in once at the end of Validate, so failed anyOf branches
	// never pay for them.
	atKey bool
	// pointer is Path as a JSON Pointer, the unambiguous form for lookups
	// (a key such as "/x" would read as a pointer in the dotted form).
	pointer string
}

// Error renders "file:line:col: path: message".
func (i Issue) Error() string { return i.posError().Error() }

func (i Issue) posError() *pos.Error {
	label := i.Path
	if label == "" {
		label = rootLabel
	}
	return &pos.Error{File: i.File, Pos: i.Pos, Msg: label + ": " + i.Message}
}

// Result is the outcome of Validate: errors fail validation, warnings
// (unknown keys, unsupported keywords) do not. Both are in document order.
type Result struct {
	Errors   []Issue
	Warnings []Issue
}

// OK reports whether there are no errors.
func (r *Result) OK() bool { return len(r.Errors) == 0 }

// Err returns nil when there are no errors, otherwise a pos.ErrorList with
// one entry per error.
func (r *Result) Err() error {
	var l pos.ErrorList
	for _, i := range r.Errors {
		l.Add(i.posError())
	}
	return l.Err()
}

// Option configures Validate and Plugin.
type Option func(*validateOptions)

type validateOptions struct {
	file      string
	onWarning func(Issue)
	// strictKeys turns "unknown key" warnings into errors (StrictKeys).
	strictKeys bool
}

// WithFileName sets the file name issues report. Without it Validate uses the
// document's own name (Open, or jsonc.WithFileName).
func WithFileName(name string) Option { return func(o *validateOptions) { o.file = name } }

// OnWarning receives every warning a Plugin's validation finds; the plugin
// interface can only return errors.
func OnWarning(fn func(Issue)) Option { return func(o *validateOptions) { o.onWarning = fn } }

// StrictKeys reports a key rejected by additionalProperties: false as an
// error instead of a warning, positioned at the key, so Result.OK and Err
// (and a Plugin's Validate and save guard) refuse a file with a typo'd key.
//
// Why it exists: by default an unknown key is only pointed out, so a newer
// option read by an older program does not refuse the file. A program that
// decodes the file into a typed struct usually wants the opposite: a
// misspelt key silently leaves its field at the zero value.
//
// Invariant: only the additionalProperties: false check changes; warnings
// for unsupported keywords stay warnings, and an issue is never reported in
// both lists.
// ds:def id=strictkeys-z4ruxr9j owner=@maintainers stability=stable
func StrictKeys() Option { return func(o *validateOptions) { o.strictKeys = true } }

// Validate checks doc against s. A document with syntax errors yields those
// errors; a schema that does not compile (bad $ref or pattern) yields one
// error naming it. s nil accepts everything. Validate never modifies s or doc.
func Validate(doc *jsonc.Doc, s *Schema, opts ...Option) *Result {
	o := validateOptions{}
	for _, fn := range opts {
		fn(&o)
	}
	if o.file == "" {
		o.file = docFile(doc)
	}
	res := &Result{}
	if err := doc.Err(); err != nil {
		var l pos.ErrorList
		if errors.As(err, &l) {
			for _, e := range l {
				res.Errors = append(res.Errors, Issue{File: e.File, Pos: e.Pos, Keyword: keywordSyntax, Message: e.Msg})
			}
		} else {
			res.Errors = append(res.Errors, Issue{File: o.file, Keyword: keywordSyntax, Message: err.Error()})
		}
		return res
	}
	if s == nil {
		return res
	}
	c, err := compile(s)
	if err != nil {
		res.Errors = append(res.Errors, Issue{File: o.file, Keyword: kwRef, Message: err.Error()})
		return res
	}
	v := &validator{doc: doc, file: o.file, c: c, res: res, warned: map[warnKey]bool{}, budget: new(int), strictKeys: o.strictKeys}
	if !doc.Has("") {
		if !(s.Bool != nil && *s.Bool) {
			res.Errors = append(res.Errors, Issue{File: o.file, Message: "the document is empty"})
		}
		return res
	}
	v.check(s, nil, 0)
	if *v.budget > maxChecks {
		v.errorf(nil, kwRef, "the schema is too complex to apply (more than %d checks); validation stopped", maxChecks)
	}
	v.locate(res.Errors)
	v.locate(res.Warnings)
	sortIssues(res.Errors)
	sortIssues(res.Warnings)
	return res
}

func sortIssues(l []Issue) {
	sort.SliceStable(l, func(i, j int) bool {
		if l[i].Pos.Line != l[j].Pos.Line {
			return l[i].Pos.Line < l[j].Pos.Line
		}
		return l[i].Pos.Col < l[j].Pos.Col
	})
}

// docFile is the name a document reports in its errors.
func docFile(doc *jsonc.Doc) string { return doc.FileName() }

type warnKey struct {
	s       *Schema
	keyword string
}

type validator struct {
	doc    *jsonc.Doc
	file   string
	c      *compiler
	res    *Result
	warned map[warnKey]bool
	// budget counts schema applications, shared with branch scratch copies.
	budget *int
	// strictKeys: an unknown key is an error, not a warning (StrictKeys).
	// Branch scratch copies inherit it, so an anyOf branch with a typo'd key
	// fails and explain reports the key from the branch that fits the type.
	strictKeys bool
}

func (v *validator) issue(at path.Path, keyword string, key bool, format string, args ...any) Issue {
	return Issue{File: v.file, Path: at.String(), Keyword: keyword, Message: fmt.Sprintf(format, args...), atKey: key, pointer: at.Pointer()}
}

// locate fills in the position of every issue.
func (v *validator) locate(l []Issue) {
	for i := range l {
		if l[i].atKey {
			l[i].Pos, _ = v.doc.KeyPos(l[i].pointer)
		} else {
			l[i].Pos, _ = v.doc.Pos(l[i].pointer)
		}
	}
}

func (v *validator) errorf(at path.Path, keyword, format string, args ...any) {
	v.res.Errors = append(v.res.Errors, v.issue(at, keyword, false, format, args...))
}

// check validates the value at p against s. hops counts $ref followed
// without descending.
func (v *validator) check(s *Schema, p path.Path, hops int) {
	if s == nil {
		return
	}
	if *v.budget++; *v.budget > maxChecks {
		return // Validate reports it once, at the root
	}
	if s.Bool != nil {
		if !*s.Bool {
			v.errorf(p, keywordFalse, "no value is allowed here")
		}
		return
	}
	n := v.c.nodes[s]
	if n == nil {
		return
	}
	ps := p.Pointer()
	kind, err := v.doc.Kind(ps)
	if err != nil {
		return
	}
	for _, k := range n.unsupported {
		if wk := (warnKey{s, k}); !v.warned[wk] {
			v.warned[wk] = true
			v.res.Warnings = append(v.res.Warnings, v.issue(p, k, false, "schema keyword %q is not supported and was not checked", k))
		}
	}
	if n.ref != nil {
		if hops >= maxRefHops {
			v.errorf(p, kwRef, "$ref %q nests more than %d deep without reaching a value (a reference loop?)", s.Ref, maxRefHops)
		} else {
			v.check(n.ref, p, hops+1)
		}
	}

	v.combinators(s, p, hops)

	var number num
	if kind == jsonc.KindNumber {
		text, _ := v.doc.ValueAt(ps)
		number, _ = parseNum(text)
	}
	got := typeName(kind, number)
	if len(s.Types) > 0 && !typeMatches(s.Types, got) {
		v.errorf(p, kwType, "must be %s, got %s", strings.Join(s.Types, " or "), got)
	}
	if s.Enum != nil || s.Const != nil {
		inst, err := v.instance(ps, kind)
		if err == nil {
			if s.Enum != nil && !inEnum(inst, s.Enum) {
				v.errorf(p, kwEnum, "must be one of %s; got %s", listRaw(s.Enum), compactJSON(inst))
			}
			if s.Const != nil && !rawEqual(inst, s.Const) {
				v.errorf(p, kwConst, "must be %s; got %s", string(s.Const), compactJSON(inst))
			}
		}
	}

	switch kind {
	case jsonc.KindNumber:
		v.bounds(p, s, n, number)
	case jsonc.KindString:
		str, _ := v.doc.GetString(ps)
		count := utf8.RuneCountInString(str)
		if s.MinLength != nil && count < *s.MinLength {
			v.errorf(p, kwMinLength, "must be at least %d characters long, got %d", *s.MinLength, count)
		}
		if s.MaxLength != nil && count > *s.MaxLength {
			v.errorf(p, kwMaxLength, "must be at most %d characters long, got %d", *s.MaxLength, count)
		}
		if n.re != nil && !n.re.MatchString(str) {
			v.errorf(p, kwPattern, "must match the pattern %q", s.Pattern)
		}
	case jsonc.KindArray:
		size, _ := v.doc.Len(ps)
		if s.MinItems != nil && size < *s.MinItems {
			v.errorf(p, kwMinItems, "must have at least %d items, got %d", *s.MinItems, size)
		}
		if s.MaxItems != nil && size > *s.MaxItems {
			v.errorf(p, kwMaxItems, "must have at most %d items, got %d", *s.MaxItems, size)
		}
		if s.Items != nil {
			for i := 0; i < size; i++ {
				v.check(s.Items, p.Append(path.Index(i)), 0)
			}
		}
	case jsonc.KindObject:
		v.object(s, p, ps)
	}
}

// combinators applies allOf, anyOf, oneOf and not.
func (v *validator) combinators(s *Schema, p path.Path, hops int) {
	for _, sub := range s.AllOf {
		v.check(sub, p, hops+1)
	}
	if len(s.AnyOf) > 0 {
		runs := v.branches(s.AnyOf, p, hops)
		if pass := passing(runs); len(pass) > 0 {
			v.res.Warnings = append(v.res.Warnings, runs[pass[0]].Warnings...)
		} else {
			v.explain(runs, p, kwAnyOf, "must match at least one of the anyOf schemas")
		}
	}
	if len(s.OneOf) > 0 {
		runs := v.branches(s.OneOf, p, hops)
		switch pass := passing(runs); len(pass) {
		case 1:
			v.res.Warnings = append(v.res.Warnings, runs[pass[0]].Warnings...)
		case 0:
			v.explain(runs, p, kwOneOf, "must match exactly one of the oneOf schemas, matches none")
		default:
			v.errorf(p, kwOneOf, "must match exactly one of the oneOf schemas, matches %d", len(pass))
		}
	}
	if s.Not != nil {
		if runs := v.branches([]*Schema{s.Not}, p, hops); len(passing(runs)) == 1 {
			v.errorf(p, kwNot, "must not match the schema under \"not\"")
		}
	}
}

// branches validates the value against each subschema separately.
func (v *validator) branches(subs []*Schema, p path.Path, hops int) []*Result {
	runs := make([]*Result, len(subs))
	for i, sub := range subs {
		scratch := *v
		scratch.res = &Result{}
		scratch.warned = map[warnKey]bool{}
		scratch.check(sub, p, hops+1)
		runs[i] = scratch.res
	}
	return runs
}

func passing(runs []*Result) []int {
	var out []int
	for i, r := range runs {
		if r.OK() {
			out = append(out, i)
		}
	}
	return out
}

// explain reports a failed anyOf/oneOf. When exactly one branch accepts the
// value's type, its own errors are the useful ones (a nullable section with
// a bad key inside), so they are reported with their positions; otherwise one
// summary error.
func (v *validator) explain(runs []*Result, p path.Path, keyword, msg string) {
	here := p.String()
	var typed []*Result
	for _, r := range runs {
		typeErr := false
		for _, e := range r.Errors {
			if e.Path == here && (e.Keyword == kwType || e.Keyword == keywordFalse) {
				typeErr = true
				break
			}
		}
		if !typeErr {
			typed = append(typed, r)
		}
	}
	if len(typed) == 1 {
		v.res.Errors = append(v.res.Errors, typed[0].Errors...)
		v.res.Warnings = append(v.res.Warnings, typed[0].Warnings...)
		return
	}
	v.errorf(p, keyword, "%s", msg)
}

func (v *validator) bounds(p path.Path, s *Schema, n *compiled, x num) {
	if n.minimum != nil && x.cmp(*n.minimum) < 0 {
		v.errorf(p, kwMinimum, "must be >= %s", s.Minimum)
	}
	if n.maximum != nil && x.cmp(*n.maximum) > 0 {
		v.errorf(p, kwMaximum, "must be <= %s", s.Maximum)
	}
	if n.exclMin != nil && x.cmp(*n.exclMin) <= 0 {
		v.errorf(p, kwExclusiveMinimum, "must be > %s", s.ExclusiveMinimum)
	}
	if n.exclMax != nil && x.cmp(*n.exclMax) >= 0 {
		v.errorf(p, kwExclusiveMaximum, "must be < %s", s.ExclusiveMaximum)
	}
}

func (v *validator) object(s *Schema, p path.Path, ps string) {
	keys, _ := v.doc.Keys(ps)
	present := make(map[string]bool, len(keys))
	for _, k := range keys {
		present[k] = true
	}
	for _, r := range s.Required {
		if !present[r] {
			v.errorf(p, kwRequired, "missing required key %q", r)
		}
	}
	for _, k := range keys {
		child := p.Append(path.Key(k))
		if ps, ok := s.Properties[k]; ok {
			v.check(ps, child, 0)
			continue
		}
		ap := s.AdditionalProperties
		if ap == nil {
			continue
		}
		if ap.Bool != nil && !*ap.Bool {
			unknown := v.issue(child, kwAdditional, true, "unknown key %q", k)
			if v.strictKeys {
				v.res.Errors = append(v.res.Errors, unknown)
			} else {
				v.res.Warnings = append(v.res.Warnings, unknown)
			}
			continue
		}
		v.check(ap, child, 0)
	}
}

// instance decodes the value at ps for enum and const: numbers stay
// json.Number so comparisons are exact.
func (v *validator) instance(ps string, kind jsonc.Kind) (any, error) {
	switch kind {
	case jsonc.KindString:
		return v.doc.GetString(ps)
	case jsonc.KindBool:
		return v.doc.GetBool(ps)
	case jsonc.KindNull:
		return nil, nil
	}
	text, _ := v.doc.ValueAt(ps)
	if kind == jsonc.KindNumber {
		return json.Number(text), nil
	}
	sub, err := jsonc.Parse([]byte(text))
	if err != nil {
		return nil, err
	}
	plain, err := sub.JSON()
	if err != nil {
		return nil, err
	}
	return decodeNumbers(plain)
}

func decodeNumbers(b []byte) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	var out any
	err := dec.Decode(&out)
	return out, err
}

// typeName is the JSON Schema type of a value; a number with no fraction is
// "integer".
func typeName(kind jsonc.Kind, n num) string {
	switch kind {
	case jsonc.KindObject:
		return TypeObject
	case jsonc.KindArray:
		return TypeArray
	case jsonc.KindString:
		return TypeString
	case jsonc.KindBool:
		return TypeBoolean
	case jsonc.KindNull:
		return TypeNull
	case jsonc.KindNumber:
		if n.isInt() {
			return TypeInteger
		}
		return TypeNumber
	}
	return ""
}

func typeMatches(want []string, got string) bool {
	for _, w := range want {
		if w == got || (w == TypeNumber && got == TypeInteger) {
			return true
		}
	}
	return false
}

func inEnum(inst any, enum []json.RawMessage) bool {
	for _, e := range enum {
		if rawEqual(inst, e) {
			return true
		}
	}
	return false
}

func rawEqual(inst any, raw json.RawMessage) bool {
	want, err := decodeNumbers(raw)
	if err != nil {
		return false
	}
	return jsonEqual(inst, want)
}

// jsonEqual is JSON Schema equality: numbers by value (1 == 1.0), objects
// regardless of key order.
func jsonEqual(a, b any) bool {
	switch x := a.(type) {
	case json.Number:
		y, ok := b.(json.Number)
		if !ok {
			return false
		}
		nx, ok1 := parseNum(string(x))
		ny, ok2 := parseNum(string(y))
		return ok1 && ok2 && nx.cmp(ny) == 0
	case []any:
		y, ok := b.([]any)
		if !ok || len(x) != len(y) {
			return false
		}
		for i := range x {
			if !jsonEqual(x[i], y[i]) {
				return false
			}
		}
		return true
	case map[string]any:
		y, ok := b.(map[string]any)
		if !ok || len(x) != len(y) {
			return false
		}
		for k, xv := range x {
			yv, ok := y[k]
			if !ok || !jsonEqual(xv, yv) {
				return false
			}
		}
		return true
	}
	return a == b
}

func compactJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprint(v)
	}
	return string(b)
}

func listRaw(enum []json.RawMessage) string {
	parts := make([]string, 0, min(len(enum), maxListed)+1)
	for i, e := range enum {
		if i == maxListed {
			parts = append(parts, fmt.Sprintf("… (%d more)", len(enum)-maxListed))
			break
		}
		parts = append(parts, string(e))
	}
	return strings.Join(parts, ", ")
}
