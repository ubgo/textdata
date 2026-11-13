package jsonc

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/ubgo/textdata/path"
	"github.com/ubgo/textdata/plugin"
	"github.com/ubgo/textdata/pos"
)

// ExtendsKey is the root member that names base documents:
//
//	{ "$extends": "../base.jsonc", "port": 8080 }
//	{ "$extends": ["base.jsonc", "local.jsonc"] }
//
// It is an ordinary key until a plugin.Includer is installed (for example
// WithPlugin(extends.New()) from github.com/ubgo/textdata/plugins/extends);
// only then does Extended act on it.
const ExtendsKey = "$extends"

// MaxExtendsDepth bounds how many $extends hops a chain may take, so a cycle
// the location comparison cannot see (two spellings of one file through a
// symlink) still ends.
const MaxExtendsDepth = 32

// Errors returned (wrapped) by Extended.
var (
	// ErrNoIncluder: the document has $extends but no plugin.Includer is
	// installed, so Extended cannot load the bases.
	ErrNoIncluder = errors.New("jsonc: $extends needs an installed Includer")
	// ErrExtendsCycle: a document extends itself, directly or through others.
	ErrExtendsCycle = errors.New("jsonc: $extends cycle")
	// ErrExtendsDepth: the chain is longer than MaxExtendsDepth.
	ErrExtendsDepth = errors.New("jsonc: $extends chain too deep")
	// ErrExtendsValue: $extends is not a string or an array of strings.
	ErrExtendsValue = errors.New("jsonc: $extends must be a string or an array of strings")
)

// extendsRef is one reference and where it is written.
type extendsRef struct {
	ref string
	at  int // offset of the string value
}

// Extends returns the references the root $extends member names, in order,
// as written (nil when there is none). It never loads anything. A value that
// is not a string or an array of strings is ErrExtendsValue at its position.
func (d *Doc) Extends() ([]string, error) {
	if err := d.valid(); err != nil {
		return nil, err
	}
	refs, err := d.extendsRefs()
	if err != nil {
		return nil, err
	}
	var out []string
	for _, r := range refs {
		out = append(out, r.ref)
	}
	return out, nil
}

// Extended returns a new document: every base named by the root $extends
// member, loaded through the installed plugin.Includer, deep-merged in order
// (see Merge), then this document on top, with $extends removed. Each base
// may extend others; its references are resolved from its own location.
//
// The receiver is never modified: Render still writes "$extends" as written,
// and Save of the receiver never bakes the bases in. The result carries the
// receiver's options and path; it is for reading (Get, Decode, Expanded), not
// for saving over the original.
//
// A document without $extends yields a plain clone, with or without an
// Includer. With $extends and no Includer it is ErrNoIncluder. Errors carry
// file:line:col of the offending $extends value: a reference that cannot be
// loaded (the Includer's error, naming the plugin), a base with syntax
// errors, a cycle (ErrExtendsCycle, naming the chain), a chain longer than
// MaxExtendsDepth, or a base whose root is not an object.
//
// When several Includers are installed they are tried in install order and
// the first that does not answer plugin.ErrNotFound wins.
//
// Expansion is separate: call Expanded on the result to expand the merged
// values, so references in a base read the same variables as the document.
func (d *Doc) Extended() (*Doc, error) {
	return d.extended(nil)
}

// extendIfInstalled is Extended when an Includer is installed, and d itself
// otherwise: the hook for one-call readers (Read) so installing an Includer is
// what turns $extends on.
func (d *Doc) extendIfInstalled() (*Doc, error) {
	if len(plugin.Filter[plugin.Includer](&d.opts.reg)) == 0 {
		return d, nil
	}
	return d.Extended()
}

// location is the name this document goes by in the chain and is the base for
// its relative references: the file it was opened from, else the name given
// with WithFileName, else "" (the Includer's default folder).
func (d *Doc) location() string {
	if d.path != "" {
		return d.path
	}
	return d.opts.file
}

// extended does the work; chain holds the locations of the documents that
// (transitively) extend d, outermost first.
func (d *Doc) extended(chain []string) (*Doc, error) {
	if err := d.valid(); err != nil {
		return nil, err
	}
	refs, err := d.extendsRefs()
	if err != nil {
		return nil, err
	}
	self := d.Clone()
	if err := self.stripExtends(); err != nil {
		return nil, err
	}
	if len(refs) == 0 {
		return self, nil
	}
	includers := plugin.Filter[plugin.Includer](&d.opts.reg)
	if len(includers) == 0 {
		return nil, d.extendsErr(refs[0].at, ErrNoIncluder, "")
	}
	from := d.location()
	chain = append(chain[:len(chain):len(chain)], chainName(from))
	if len(chain) > MaxExtendsDepth {
		return nil, d.extendsErr(refs[0].at, ErrExtendsDepth, fmt.Sprintf("more than %d levels", MaxExtendsDepth))
	}

	var acc *Doc
	for _, r := range refs {
		base, err := d.loadBase(includers, from, r, chain)
		if err != nil {
			return nil, err
		}
		if acc == nil {
			acc = base
			continue
		}
		if err := acc.Merge(base); err != nil {
			return nil, d.extendsErr(r.at, err, fmt.Sprintf("%s %q", ExtendsKey, r.ref))
		}
	}
	if err := acc.Merge(self); err != nil {
		return nil, d.extendsErr(refs[0].at, err, "merge onto the bases")
	}
	acc.opts, acc.path, acc.existed, acc.mode = d.opts, d.path, d.existed, d.mode
	return acc, nil
}

// loadBase includes one reference, parses it and resolves its own $extends.
func (d *Doc) loadBase(includers []plugin.Includer, from string, r extendsRef, chain []string) (*Doc, error) {
	what := fmt.Sprintf("%s %q", ExtendsKey, r.ref)
	data, loc, err := d.include(includers, from, r.ref)
	if err != nil {
		return nil, d.extendsErr(r.at, err, what)
	}
	name := chainName(loc)
	for i, c := range chain {
		if c == name {
			cyc := append(append([]string(nil), chain[i:]...), name)
			return nil, d.extendsErr(r.at, ErrExtendsCycle, what+" closes a cycle: "+strings.Join(display(cyc), " -> "))
		}
	}
	base := &Doc{src: data, opts: d.opts, path: loc, mode: defaultFileMode}
	base.opts.file = loc
	base.reparse()
	if err := base.errs.Err(); err != nil {
		return nil, d.extendsErr(r.at, err, what)
	}
	if base.root != nil && base.root.kind != KindObject {
		return nil, d.extendsErr(r.at, ErrType, fmt.Sprintf("%s: the root of %s is %s, not an object", what, loc, article(base.root.kind)))
	}
	out, err := base.extended(chain)
	if err != nil {
		// The inner error already names its own file:line:col; keep the
		// outer position too so the whole path to the problem is visible.
		return nil, d.extendsErr(r.at, err, what)
	}
	return out, nil
}

// include asks the Includers in install order; plugin.ErrNotFound passes to
// the next one, any other error stops.
func (d *Doc) include(includers []plugin.Includer, from, ref string) ([]byte, string, error) {
	var last error
	for i, inc := range includers {
		data, loc, err := inc.Include(from, ref)
		if err == nil {
			return data, loc, nil
		}
		last = plugin.Errorf(errPrefix, d.opts.reg.NameFor(plugin.CapIncluder, i), "include "+ref, err)
		if !errors.Is(err, plugin.ErrNotFound) {
			break
		}
	}
	return nil, "", last
}

// extendsRefs reads the root $extends member (the last one, on duplicates).
func (d *Doc) extendsRefs() ([]extendsRef, error) {
	if d.root == nil || d.root.kind != KindObject {
		return nil, nil
	}
	i, ok := child(d.root, path.Key(ExtendsKey))
	if !ok {
		return nil, nil
	}
	v := d.root.items[i].value
	switch v.kind {
	case KindString:
		return []extendsRef{{ref: v.str, at: v.start}}, nil
	case KindArray:
		refs := make([]extendsRef, 0, len(v.items))
		for _, it := range v.items {
			if it.value.kind != KindString {
				return nil, d.extendsErr(it.value.start, ErrExtendsValue, "found "+article(it.value.kind)+" in the list")
			}
			refs = append(refs, extendsRef{ref: it.value.str, at: it.value.start})
		}
		return refs, nil
	default:
		return nil, d.extendsErr(v.start, ErrExtendsValue, "found "+article(v.kind))
	}
}

// stripExtends deletes every root $extends member (duplicates included).
func (d *Doc) stripExtends() error {
	for d.root != nil && d.root.kind == KindObject {
		i, ok := child(d.root, path.Key(ExtendsKey))
		if !ok {
			return nil
		}
		if err := d.deleteItem(d.root, i); err != nil {
			return fmt.Errorf("jsonc: remove %s: %w", ExtendsKey, err)
		}
	}
	return nil
}

// extendsErr is a file:line:col error at offset off of d.
func (d *Doc) extendsErr(off int, err error, msg string) error {
	if msg == "" {
		msg = ExtendsKey
	}
	return &pos.Error{File: d.opts.file, Pos: d.at(off), Msg: msg, Err: err}
}

// chainName is the comparable form of a location.
func chainName(loc string) string {
	if loc == "" {
		return ""
	}
	return filepath.Clean(loc)
}

// display names "" as <input> in a cycle message.
func display(chain []string) []string {
	out := make([]string, len(chain))
	for i, c := range chain {
		if c == "" {
			c = "<input>"
		}
		out[i] = c
	}
	return out
}
