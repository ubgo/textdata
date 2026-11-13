package jsonc

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/ubgo/textdata/path"
)

// Raw is JSONC text inserted verbatim by Set, Append and Insert, for a value
// written exactly as given (comments, layout). It must parse as one value.
// Nested inside a map or slice, it must be plain JSON.
type Raw string

// defaultIndentUnit is used when the document shows no indentation to copy.
const defaultIndentUnit = "  "

// defaultSep is the key/value separator when no member shows one to copy.
const defaultSep = ": "

// lineCommentMark prefixes each line Disable comments out and SetComment
// writes; Restore removes exactly it.
const lineCommentMark = "// "

// splice replaces src[start:end] with text.
type splice struct {
	start, end int
	text       string
}

// apply performs non-overlapping splices and re-parses. An edit that would
// leave the document invalid is refused and the document is unchanged; that
// is a bug guard, every edit is built to stay valid.
func (d *Doc) apply(sp ...splice) error {
	sort.Slice(sp, func(i, j int) bool { return sp[i].start > sp[j].start })
	out := append([]byte(nil), d.src...)
	for _, s := range sp {
		out = append(out[:s.start], append([]byte(s.text), out[s.end:]...)...)
	}
	root, comments, errs := parse(out, d.opts.strict, d.opts.file)
	if len(errs) > 0 {
		return fmt.Errorf("jsonc: edit would leave the document invalid (%v); nothing changed", errs[0])
	}
	d.src, d.root, d.comments, d.errs = out, root, comments, nil
	d.idx.Store(nil)
	return nil
}

// Set replaces the value at path, or creates it (and any missing parent
// objects) at the end of its object, in that object's layout. value is
// encoded with encoding/json, or written verbatim when it is a Raw. Array
// elements are not created by Set; use Append or Insert.
func (d *Doc) Set(p string, value any) error {
	pp, err := d.editPath(p)
	if err != nil {
		return err
	}
	if loc, err := d.find(pp); err == nil {
		return d.replace(loc, value)
	}
	if d.root == nil {
		nested, err := nest(pp, value)
		if err != nil {
			return err
		}
		return d.createRoot(nested)
	}
	// Deepest existing ancestor.
	k, cur := 0, d.root
	for ; k < len(pp); k++ {
		i, ok := child(cur, pp[k])
		if !ok {
			break
		}
		cur = cur.items[i].value
	}
	if cur.kind != KindObject {
		if cur.kind == KindArray && pp[k].MatchesIndex() {
			return fmt.Errorf("%w: %q: index out of range (Set does not create array elements; use Append or Insert)", ErrNotFound, pp[:k+1].String())
		}
		return fmt.Errorf("%w: cannot create %q inside %s", ErrType, pp[:k+1].String(), article(cur.kind))
	}
	if !pp[k].MatchesKey() {
		return fmt.Errorf("%w: %q: an object has no index %d", ErrType, pp[:k+1].String(), pp[k].Index)
	}
	nested, err := nest(pp[k+1:], value)
	if err != nil {
		return err
	}
	return d.insertItem(cur, len(cur.items), pp[k].Key, true, nested)
}

// SetAfter sets path like Set, but a new key is placed right after the
// sibling key anchor instead of at the end. An existing key is updated where
// it is. The parent object must exist.
func (d *Doc) SetAfter(anchor, p string, value any) error { return d.setNear(anchor, p, value, true) }

// SetBefore is SetAfter placing a new key right before anchor.
func (d *Doc) SetBefore(anchor, p string, value any) error {
	return d.setNear(anchor, p, value, false)
}

func (d *Doc) setNear(anchor, p string, value any, after bool) error {
	pp, err := d.editPath(p)
	if err != nil {
		return err
	}
	if loc, err := d.find(pp); err == nil {
		return d.replace(loc, value)
	}
	parent, last, ok := pp.Parent()
	if !ok || !last.MatchesKey() {
		return fmt.Errorf("%w: %q is not an object member", ErrType, p)
	}
	ploc, err := d.find(parent)
	if err != nil {
		return err
	}
	if ploc.n.kind != KindObject {
		return fmt.Errorf("%w: %q is %s, not an object", ErrType, parent.String(), article(ploc.n.kind))
	}
	ai, ok := child(ploc.n, path.Key(anchor))
	if !ok {
		return fmt.Errorf("%w: anchor %q in %q", ErrNotFound, anchor, parent.String())
	}
	at := ai
	if after {
		at++
	}
	return d.insertItem(ploc.n, at, last.Key, true, value)
}

// Delete removes the value at path with its key, its attached comments and
// the comma that goes with it. Deleting a key Set just created restores the
// original bytes.
func (d *Doc) Delete(p string) error {
	pp, err := d.editPath(p)
	if err != nil {
		return err
	}
	loc, err := d.find(pp)
	if err != nil {
		return err
	}
	if loc.it == nil {
		return fmt.Errorf("%w: cannot delete the root value", ErrType)
	}
	return d.deleteItem(loc.parent, loc.index)
}

// Append adds value at the end of the array at path.
func (d *Doc) Append(p string, value any) error {
	arr, err := d.array(p)
	if err != nil {
		return err
	}
	return d.insertItem(arr, len(arr.items), "", false, value)
}

// Insert adds value at index i of the array at path (0 <= i <= length).
func (d *Doc) Insert(p string, i int, value any) error {
	arr, err := d.array(p)
	if err != nil {
		return err
	}
	if i < 0 || i > len(arr.items) {
		return fmt.Errorf("%w: index %d of %q (length %d)", ErrNotFound, i, p, len(arr.items))
	}
	return d.insertItem(arr, i, "", false, value)
}

// Remove deletes element i of the array at path.
func (d *Doc) Remove(p string, i int) error {
	arr, err := d.array(p)
	if err != nil {
		return err
	}
	if i < 0 || i >= len(arr.items) {
		return fmt.Errorf("%w: index %d of %q (length %d)", ErrNotFound, i, p, len(arr.items))
	}
	return d.deleteItem(arr, i)
}

// SetComment sets the comment attached above the value at path, replacing
// any existing leading comment. Each line of text becomes a `// ` line; for a
// value that shares its line with others, a /* */ block before it.
func (d *Doc) SetComment(p, text string) error {
	if d.opts.strict {
		return ErrStrict
	}
	it, err := d.itemAt(p)
	if err != nil {
		return err
	}
	if !ownLineStart(d.src, it.start()) {
		if strings.Contains(text, "*/") {
			return fmt.Errorf("jsonc: comment text contains \"*/\"")
		}
		return d.apply(splice{it.start(), it.start(), "/* " + text + " */ "})
	}
	indent := indentOf(d.src, it.start())
	eol := d.eolBefore(lineStart(d.src, it.start()))
	var b strings.Builder
	for _, line := range strings.Split(text, "\n") {
		b.WriteString(indent)
		b.WriteString(strings.TrimRight(lineCommentMark+line, " "))
		b.WriteString(eol)
	}
	from := lineStart(d.src, it.start())
	if len(it.leading) > 0 {
		from = lineStart(d.src, it.leading[0].start)
	}
	return d.apply(splice{from, lineStart(d.src, it.start()), b.String()})
}

// RemoveComment removes the leading and trailing comments attached to the
// value at path. No comment is not an error.
func (d *Doc) RemoveComment(p string) error {
	it, err := d.itemAt(p)
	if err != nil {
		return err
	}
	var sp []splice
	if len(it.leading) > 0 {
		from := it.leading[0].start
		to := it.start()
		if ownLineStart(d.src, from) && ownLineStart(d.src, to) {
			from, to = lineStart(d.src, from), lineStart(d.src, to)
		}
		sp = append(sp, splice{from, to, ""})
	}
	if c := it.trailing; c != nil {
		from := c.start
		for from > 0 && (d.src[from-1] == ' ' || d.src[from-1] == '\t') {
			from--
		}
		sp = append(sp, splice{from, c.end, ""})
	}
	if len(sp) == 0 {
		return nil
	}
	return d.apply(sp...)
}

// Disable turns the object member at path into `// ` comment lines, so
// it no longer counts but stays in the file; Restore brings it back byte for
// byte. The member must occupy lines of its own, with its comma (if any) on
// its value's last line (ErrLayout otherwise).
func (d *Doc) Disable(p string) error {
	if d.opts.strict {
		return ErrStrict
	}
	it, err := d.itemAt(p)
	if err != nil {
		return err
	}
	if it.keyStart < 0 {
		return fmt.Errorf("%w: %q is an array element; Disable works on object members", ErrType, p)
	}
	if !ownLineStart(d.src, it.start()) || !ownLineEnd(d.src, it.tailEnd()) {
		return fmt.Errorf("%w: %q", ErrLayout, p)
	}
	if it.comma >= 0 && !sameLine(d.src, it.value.end, it.comma) {
		// A comma alone on a later line would read back as a separate
		// "// ," comment line, which Restore cannot tell from one the
		// author wrote.
		return fmt.Errorf("%w: %q has its comma on a separate line", ErrLayout, p)
	}
	from, to := lineStart(d.src, it.start()), lineEnd(d.src, it.tailEnd())
	var b strings.Builder
	for i, line := range strings.Split(string(d.src[from:to]), "\n") {
		if i > 0 {
			b.WriteByte('\n')
		}
		ind := len(line) - len(strings.TrimLeft(line, " \t\r"))
		b.WriteString(line[:ind])
		b.WriteString(lineCommentMark)
		b.WriteString(line[ind:])
	}
	return d.apply(splice{from, to, b.String()})
}

// Restore brings back a member Disable commented out, byte for byte. It
// returns ErrNotFound when no disabled member with that key exists in the
// parent object and ErrExists when the key is live again.
func (d *Doc) Restore(p string) error {
	pp, err := d.editPath(p)
	if err != nil {
		return err
	}
	if _, err := d.find(pp); err == nil {
		return fmt.Errorf("%w: %q", ErrExists, p)
	}
	parent, last, ok := pp.Parent()
	if !ok || !last.MatchesKey() {
		return fmt.Errorf("%w: %q is not an object member", ErrType, p)
	}
	ploc, err := d.find(parent)
	if err != nil {
		return err
	}
	obj := ploc.n
	if obj.kind != KindObject {
		return fmt.Errorf("%w: %q is %s, not an object", ErrType, parent.String(), article(obj.kind))
	}
	cs := directComments(obj, d.comments)
	for j := len(cs) - 1; j >= 0; j-- {
		c := cs[j]
		if c.block || !ownLineStart(d.src, c.start) || !disabledKey(d.src[c.start:c.end], last.Key) {
			continue
		}
		if sp, ok := d.restoreFrom(obj, cs[j:], last.Key); ok {
			return d.apply(sp...)
		}
	}
	return fmt.Errorf("%w: no disabled %q", ErrNotFound, p)
}

// disabledKey reports whether a line comment reads `// "key"...`, comparing
// the decoded key so any escaping of it matches.
func disabledKey(c []byte, key string) bool {
	rest, ok := bytes.CutPrefix(c, []byte(lineCommentMark))
	if !ok || len(rest) == 0 || rest[0] != '"' {
		return false
	}
	p := &parser{src: rest}
	got, ok := p.str()
	return ok && got == key
}

// restoreFrom tries to restore the member whose disabled text starts at
// run[0], extending over consecutive comment lines until the text parses as
// exactly that one member.
func (d *Doc) restoreFrom(obj *node, run []comment, key string) ([]splice, bool) {
	from := lineStart(d.src, run[0].start)
	var stripped strings.Builder
	prevLine := -1
	for k, c := range run {
		if c.block || !ownLineStart(d.src, c.start) || !strings.HasPrefix(string(d.src[c.start:c.end]), lineCommentMark) {
			return nil, false
		}
		ls := lineStart(d.src, c.start)
		if k > 0 && ls != prevLine {
			return nil, false // not consecutive lines
		}
		le := lineEnd(d.src, c.start)
		if k > 0 {
			stripped.WriteByte('\n')
		}
		stripped.Write(d.src[ls:c.start])
		stripped.Write(d.src[c.start+len(lineCommentMark) : le])
		prevLine = le + 1
		text := stripped.String()
		root, _, errs := parse([]byte("{"+text+"\n}"), false, "") // the newline keeps "}" out of a trailing // comment
		if len(errs) > 0 || len(root.items) != 1 || root.items[0].key != key {
			continue
		}
		member := root.items[0]
		var sp []splice
		end := le
		// A live member after the block needs a comma after the restored one;
		// a live member before it needs its own comma.
		blockEnd := le
		var before, after *item
		for _, it := range obj.items {
			if it.start() < from {
				before = it
			} else if it.start() > blockEnd && after == nil {
				after = it
			}
		}
		if after != nil && member.comma < 0 {
			at := member.value.end - 1 // offset in text ("{" prefix removed)
			text = text[:at] + "," + text[at:]
		}
		if before != nil && before.comma < 0 {
			sp = append(sp, splice{before.value.end, before.value.end, ","})
		}
		sp = append(sp, splice{from, end, text})
		return sp, true
	}
	return nil, false
}

// directComments returns the comments inside n's brackets that are not
// inside any of its items' values.
func directComments(n *node, all []comment) []comment {
	end := n.close
	if end < 0 {
		end = n.end
	}
	var out []comment
	for _, c := range all {
		if c.start <= n.open || c.start >= end {
			continue
		}
		inside := false
		for _, it := range n.items {
			if c.start >= it.value.start && c.start < it.value.end {
				inside = true
				break
			}
		}
		if !inside {
			out = append(out, c)
		}
	}
	return out
}

// editPath checks the document can be edited and parses p.
func (d *Doc) editPath(p string) (path.Path, error) {
	if err := d.valid(); err != nil {
		return nil, err
	}
	return parsePath(p)
}

func (d *Doc) itemAt(p string) (*item, error) {
	pp, err := d.editPath(p)
	if err != nil {
		return nil, err
	}
	loc, err := d.find(pp)
	if err != nil {
		return nil, err
	}
	if loc.it == nil {
		return nil, fmt.Errorf("%w: the root value has no key or comment slot", ErrType)
	}
	return loc.it, nil
}

func (d *Doc) array(p string) (*node, error) {
	pp, err := d.editPath(p)
	if err != nil {
		return nil, err
	}
	loc, err := d.find(pp)
	if err != nil {
		return nil, err
	}
	if loc.n.kind != KindArray {
		return nil, d.typeErr(loc, p, "an array")
	}
	return loc.n, nil
}

// replace rewrites the value at loc in place.
func (d *Doc) replace(loc located, value any) error {
	at := loc.n.start
	multiline := loc.it == nil || ownLineStart(d.src, loc.it.start())
	text, err := d.render(value, indentOf(d.src, at), d.eolAt(at), multiline)
	if err != nil {
		return err
	}
	return d.apply(splice{loc.n.start, loc.n.end, text})
}

// createRoot writes a first value into an empty (or comment-only) document.
func (d *Doc) createRoot(value any) error {
	eol := d.docEOL()
	text, err := d.render(value, "", eol, true)
	if err != nil {
		return err
	}
	prefix := ""
	if len(d.src) > 0 && d.src[len(d.src)-1] != '\n' {
		prefix = eol
	}
	return d.apply(splice{len(d.src), len(d.src), prefix + text + eol})
}

// insertItem inserts a member (hasKey) or element before n.items[at] (or at
// the end when at == len), following the container's layout:
//
//   - items on lines of their own: the new item gets its own line with the
//     same indentation;
//   - items sharing a line: the new item joins the line, ", " separated;
//   - an empty container: right after the opening bracket, on a new line when
//     the brackets are on different lines.
//
// Delete reverses each case exactly.
func (d *Doc) insertItem(n *node, at int, key string, hasKey bool, value any) error {
	src := d.src
	unit := d.indentUnit()
	if len(n.items) == 0 {
		// On a new line after the opening bracket's line (and any comment
		// running from it), so such a comment stays where it is and is not
		// taken as the new item's trailing comment. When that runs onto the
		// closing bracket's line, fall through to the single-line form.
		if at := d.afterLine(n, n.open+1); at < n.close {
			indent := indentOf(src, n.open) + unit
			text, err := d.itemText(n, key, hasKey, value, indent, true)
			if err != nil {
				return err
			}
			return d.apply(splice{at, at, d.eolAt(at) + indent + text})
		}
		// Single line: just before the closing bracket, after any comment.
		text, err := d.itemText(n, key, hasKey, value, "", false)
		if err != nil {
			return err
		}
		return d.apply(splice{n.close, n.close, text})
	}

	if at < len(n.items) {
		y := n.items[at]
		if ownLineStart(src, y.spanStart()) {
			indent := indentOf(src, y.start())
			text, err := d.itemText(n, key, hasKey, value, indent, true)
			if err != nil {
				return err
			}
			ls := lineStart(src, y.spanStart())
			return d.apply(splice{ls, ls, indent + text + "," + d.eolBefore(ls)})
		}
		text, err := d.itemText(n, key, hasKey, value, "", false)
		if err != nil {
			return err
		}
		return d.apply(splice{y.spanStart(), y.spanStart(), text + ", "})
	}

	l := n.items[len(n.items)-1]
	if d.endsLine(l.tailEnd()) {
		// The last item ends its line (possibly with a // comment): the new
		// item gets a line of its own after it.
		indent := indentOf(src, l.start())
		if !ownLineStart(src, l.start()) {
			indent = indentOf(src, n.open) + unit
		}
		text, err := d.itemText(n, key, hasKey, value, indent, true)
		if err != nil {
			return err
		}
		ins := d.afterLine(n, l.tailEnd())
		eol := d.eolAt(ins)
		if l.comma < 0 {
			return d.apply(splice{l.value.end, ins, "," + string(src[l.value.end:ins]) + eol + indent + text})
		}
		return d.apply(splice{ins, ins, eol + indent + text + ","})
	}
	text, err := d.itemText(n, key, hasKey, value, "", false)
	if err != nil {
		return err
	}
	// After every comment that follows the last item, so none of them
	// becomes the new item's trailing comment.
	ins := l.tailEnd()
	for at := ins; at < n.close; {
		for at < n.close && onlySpace(d.src[at:at+1]) {
			at++
		}
		c, ok := d.commentAt(at)
		if !ok {
			break
		}
		at, ins = c.end, c.end
	}
	if l.comma < 0 {
		return d.apply(splice{ins, ins, ", " + text})
	}
	return d.apply(splice{ins, ins, " " + text + ","})
}

// endsLine reports whether only whitespace and comments follow off up to the
// end of its line.
func (d *Doc) endsLine(off int) bool {
	for {
		for off < len(d.src) && (d.src[off] == ' ' || d.src[off] == '\t' || d.src[off] == '\r') {
			off++
		}
		if off >= len(d.src) || d.src[off] == '\n' {
			return true
		}
		c, ok := d.commentAt(off)
		if !ok {
			return false
		}
		if !c.block {
			return true
		}
		off = c.end
	}
}

// commentAt returns the comment starting at off.
func (d *Doc) commentAt(off int) (comment, bool) {
	i := sort.Search(len(d.comments), func(i int) bool { return d.comments[i].start >= off })
	if i < len(d.comments) && d.comments[i].start == off {
		return d.comments[i], true
	}
	return comment{}, false
}

// afterLine returns where off's line ends (before a CR), moved past any
// comment of n that starts before that point and runs onto later lines.
func (d *Doc) afterLine(n *node, off int) int {
	at := lineEnd(d.src, off)
	for moved := true; moved; {
		moved = false
		for _, c := range directComments(n, d.comments) {
			if c.start < at && c.end > at {
				at = lineEnd(d.src, c.end)
				moved = true
			}
		}
	}
	if at > off && at < len(d.src) && d.src[at-1] == '\r' {
		at--
	}
	return at
}

// deleteItem removes n.items[i]; see insertItem for the layouts.
func (d *Doc) deleteItem(n *node, i int) error {
	src := d.src
	it := n.items[i]
	s, e := it.spanStart(), it.tailEnd()
	var prev *item
	if i > 0 {
		prev = n.items[i-1]
	}
	isLast := i == len(n.items)-1
	var sp []splice
	if ownLineStart(src, s) && ownLineEnd(src, e) {
		ls, le := lineStart(src, s), lineEnd(src, e)
		if le < len(src) {
			le++
		}
		sp = append(sp, splice{ls, le, ""})
		if isLast && it.comma < 0 && prev != nil && prev.comma >= 0 {
			sp = append(sp, splice{prev.comma, prev.comma + 1, ""})
		}
		return d.apply(sp...)
	}
	switch {
	case prev != nil:
		// From the blanks before the item, never over a comment that is
		// not the item's own.
		from := s
		for from > prev.tailEnd() && (src[from-1] == ' ' || src[from-1] == '\t') {
			from--
		}
		sp = append(sp, splice{from, e, ""})
		if it.comma < 0 && prev.comma >= 0 {
			sp = append(sp, splice{prev.comma, prev.comma + 1, ""})
		}
	case !isLast:
		sp = append(sp, splice{s, n.items[i+1].spanStart(), ""})
	default:
		sp = append(sp, splice{s, e, ""})
	}
	return d.apply(sp...)
}

// itemText renders `"key": value` (or just the value for an element).
// A keyedValue value supplies the key's source text, written as is.
func (d *Doc) itemText(n *node, key string, hasKey bool, value any, indent string, multiline bool) (string, error) {
	keyText := ""
	if kv, ok := value.(keyedValue); ok {
		keyText, value = kv.keyText, kv.value
	}
	v, err := d.render(value, indent, d.docEOL(), multiline)
	if err != nil {
		return "", err
	}
	if !hasKey {
		return v, nil
	}
	if keyText == "" {
		keyText = jsonString(key)
	}
	return keyText + separator(d.src, n) + v, nil
}

// render encodes value; multiline containers are indented under indent.
func (d *Doc) render(value any, indent, eol string, multiline bool) (string, error) {
	if cj, ok := value.(compactJSON); ok {
		s := string(cj)
		if multiline {
			var b bytes.Buffer
			if err := json.Indent(&b, cj, indent, d.indentUnit()); err != nil {
				return "", fmt.Errorf("jsonc: indent value: %w", err)
			}
			s = b.String()
			if eol != "\n" {
				s = strings.ReplaceAll(s, "\n", eol)
			}
		}
		return s, nil
	}
	if raw, ok := value.(Raw); ok {
		root, _, errs := parse([]byte(raw), d.opts.strict, "")
		if len(errs) > 0 || root == nil {
			return "", fmt.Errorf("jsonc: Raw value is not one valid value: %q", string(raw))
		}
		return string(raw), nil
	}
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if multiline {
		enc.SetIndent(indent, d.indentUnit())
	}
	if err := enc.Encode(rawSafe(value)); err != nil {
		return "", fmt.Errorf("jsonc: encode value: %w", err)
	}
	s := strings.TrimSuffix(b.String(), "\n")
	if eol != "\n" {
		s = strings.ReplaceAll(s, "\n", eol)
	}
	return s, nil
}

// rawSafe turns Raw values nested in the common container types into
// json.RawMessage so encoding/json writes them verbatim.
func rawSafe(v any) any {
	switch t := v.(type) {
	case Raw:
		return json.RawMessage(t)
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, x := range t {
			out[k] = rawSafe(x)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, x := range t {
			out[i] = rawSafe(x)
		}
		return out
	}
	return v
}

// nest wraps value in single-key objects for each remaining segment.
func nest(rest path.Path, value any) (any, error) {
	for i := len(rest) - 1; i >= 0; i-- {
		if !rest[i].MatchesKey() {
			return nil, fmt.Errorf("%w: %q: Set does not create array elements; use Append or Insert", ErrNotFound, rest[:i+1].String())
		}
		value = map[string]any{rest[i].Key: value}
	}
	return value, nil
}

func jsonString(s string) string {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(s) // a string always encodes
	return strings.TrimSuffix(b.String(), "\n")
}

// separator copies the key/value separator the object already uses.
func separator(src []byte, n *node) string {
	for _, it := range n.items {
		if it.keyStart < 0 {
			continue
		}
		sep := string(src[it.keyEnd:it.value.start])
		if strings.Count(sep, ":") == 1 && strings.Trim(sep, " \t:") == "" {
			return sep
		}
	}
	return defaultSep
}

// indentUnit is one level of indentation as the document writes it: the
// difference between the first own-line member and its container's line.
func (d *Doc) indentUnit() string {
	unit := ""
	walk(d.root, func(n *node) {
		if unit != "" {
			return
		}
		for _, it := range n.items {
			if !ownLineStart(d.src, it.start()) {
				continue
			}
			inner, outer := indentOf(d.src, it.start()), indentOf(d.src, n.open)
			if len(inner) > len(outer) && strings.HasPrefix(inner, outer) {
				unit = inner[len(outer):]
			}
			return
		}
	})
	if unit == "" {
		return defaultIndentUnit
	}
	return unit
}

// Line endings for inserted text. A line break inside a block comment is
// part of the comment's text, not the file's layout, so it never sets the
// style: a file whose only CRLF sits in a comment is an LF file.

// eolAt is the terminator of off's line: the break where text inserted at
// that line's end goes, so deleting the text again restores the input byte
// for byte. When the line has no terminator, or it lies inside a block
// comment, it is the document's line ending.
func (d *Doc) eolAt(off int) string {
	if le := lineEnd(d.src, off); le < len(d.src) && !d.inComment(le) {
		return eolOf(d.src, le)
	}
	return d.docEOL()
}

// eolBefore is the terminator of the line break just before the line start
// ls, for a line inserted at ls; at the start of the file, or when that
// break lies inside a block comment, it is the document's line ending.
func (d *Doc) eolBefore(ls int) string {
	if ls > 0 && d.src[ls-1] == '\n' && !d.inComment(ls-1) {
		return eolOf(d.src, ls-1)
	}
	return d.docEOL()
}

// docEOL is the document's line ending: that of its first line break
// outside a comment, else of its first line break, else LF.
func (d *Doc) docEOL() string {
	first := -1
	for i, c := range d.src {
		if c != '\n' {
			continue
		}
		if !d.inComment(i) {
			return eolOf(d.src, i)
		}
		if first < 0 {
			first = i
		}
	}
	if first >= 0 {
		return eolOf(d.src, first)
	}
	return "\n"
}

// eolOf is the terminator whose '\n' is at nl: CRLF or LF.
func eolOf(src []byte, nl int) string {
	if nl > 0 && src[nl-1] == '\r' {
		return "\r\n"
	}
	return "\n"
}

// inComment reports whether off lies strictly inside a comment. A line
// comment ends before its terminator, so only a block comment's own line
// breaks count as inside.
func (d *Doc) inComment(off int) bool {
	i := sort.Search(len(d.comments), func(i int) bool { return d.comments[i].end > off })
	return i < len(d.comments) && d.comments[i].start < off
}
