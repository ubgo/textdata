package jsonc

import (
	"context"
	"fmt"
	"strings"

	"github.com/ubgo/textdata/expand"
	"github.com/ubgo/textdata/path"
	"github.com/ubgo/textdata/plugin"
	"github.com/ubgo/textdata/pos"
	"github.com/ubgo/textdata/shell"
)

// errPrefix names this package in plugin errors.
const errPrefix = "jsonc"

// Expanded returns a copy of the document with every string value expanded
// and transformed; the receiver is never modified, and Render of the
// original still writes the references.
//
// In string values: $VAR, ${VAR} and the ${VAR:-default} family resolve
// through the installed Lookupers (WithLookup, plugins) — the package never
// reads the environment; an unknown name is "". ${scheme:ref} goes to the
// Resolver that owns the scheme. $(command) runs only with command
// substitution on. $$ is a literal dollar. Keys are never expanded. After
// expansion every string passes through the ValueTransformers, with its
// canonical path.
func (d *Doc) Expanded() (*Doc, error) {
	if err := d.valid(); err != nil {
		return nil, err
	}
	var sp []splice
	var firstErr error
	visitStrings(d.root, nil, func(n *node, p path.Path) {
		if firstErr != nil {
			return
		}
		v, err := d.expandString(n, p.String())
		if err != nil {
			firstErr = err
			return
		}
		if v != n.str {
			sp = append(sp, splice{n.start, n.end, jsonString(v)})
		}
	})
	if firstErr != nil {
		return nil, firstErr
	}
	c := d.Clone()
	if len(sp) == 0 {
		return c, nil
	}
	if err := c.apply(sp...); err != nil {
		return nil, err
	}
	return c, nil
}

// GetExpanded is Get on the expanded document: the value at path with every
// string inside it expanded and transformed.
func (d *Doc) GetExpanded(p string) (any, error) {
	loc, pp, err := d.lookup(p)
	if err != nil {
		return nil, err
	}
	if loc.n.kind == KindString {
		return d.expandString(loc.n, pp.String())
	}
	x, err := d.Expanded()
	if err != nil {
		return nil, err
	}
	return x.Get(p)
}

// expandString expands the string value n at canonical path at. A failure
// is a *pos.Error at the value's position (file:line:col) wrapping the cause,
// so errors.Is / errors.As still reach expand.RequiredError, plugin.Error and
// the plugin sentinels.
func (d *Doc) expandString(n *node, at string) (string, error) {
	eng := expand.Engine{Lookup: d.lookupVar}
	if d.opts.shell {
		eng.Run = d.opts.command()
	}
	if len(d.opts.resolvers) > 0 {
		eng.Resolve = d.resolveScheme
	}
	if len(d.opts.observers) > 0 {
		eng.Observe = func(name, resolved string) {
			for _, o := range d.opts.observers {
				o.ObserveExpand(at, name, resolved)
			}
		}
	}
	v, err := eng.Expand(n.str, nil)
	if err != nil {
		return "", d.posErr(n.start, err, "expand %q", at)
	}
	for i, t := range d.opts.transformers {
		out, err := t.TransformValue(at, v)
		if err != nil {
			return "", d.posErr(n.start, plugin.Errorf(errPrefix, d.opts.reg.NameFor(plugin.CapValueTransformer, i), "transform "+at, err), "")
		}
		v = out
	}
	return v, nil
}

// posErr wraps err in a *pos.Error at offset off of the document.
func (d *Doc) posErr(off int, err error, format string, args ...any) error {
	return &pos.Error{File: d.opts.file, Pos: d.at(off), Msg: fmt.Sprintf(format, args...), Err: err}
}

// lookupVar asks the Lookupers in install order; the first answer wins and an
// error aborts. A name already being expanded (a lookup value referring back
// to itself) reads as unset, so every chain ends.
func (d *Doc) lookupVar(name string, st *expand.Stack) (expand.Var, error) {
	if _, on := st.At(name); on {
		return expand.Var{}, nil
	}
	for i, l := range d.opts.lookupers {
		v, ok, err := l.Lookup(name)
		if err != nil {
			return expand.Var{}, plugin.Errorf(errPrefix, d.opts.reg.NameFor(plugin.CapLookup, i), "lookup "+name, err)
		}
		if ok {
			return expand.Var{Value: v, Set: true, At: expand.AtLookup}, nil
		}
	}
	return expand.Var{}, nil
}

func (d *Doc) resolveScheme(scheme, ref string) (string, bool, error) {
	for i, r := range d.opts.resolvers {
		for _, s := range r.Schemes() {
			if !strings.EqualFold(s, scheme) {
				continue
			}
			v, err := r.Resolve(scheme, ref)
			if err != nil {
				return "", true, plugin.Errorf(errPrefix, d.opts.reg.NameFor(plugin.CapResolver, i), "resolve "+scheme+":"+ref, err)
			}
			return v, true, nil
		}
	}
	return "", false, nil
}

// visitStrings calls fn for every string value (not keys) with its path.
func visitStrings(n *node, p path.Path, fn func(*node, path.Path)) {
	if n == nil {
		return
	}
	switch n.kind {
	case KindString:
		fn(n, p)
	case KindObject:
		for _, it := range n.items {
			visitStrings(it.value, p.Append(path.Key(it.key)), fn)
		}
	case KindArray:
		for i, it := range n.items {
			visitStrings(it.value, p.Append(path.Index(i)), fn)
		}
	}
}

// Validate runs every installed plugin.Validator and returns the first
// failure, naming the plugin. It never runs on its own.
func (d *Doc) Validate() error {
	if err := d.valid(); err != nil {
		return err
	}
	for i, v := range d.opts.validators {
		if err := v.Validate(d.Clone()); err != nil {
			return plugin.Errorf(errPrefix, d.opts.reg.NameFor(plugin.CapValidator, i), "validate", err)
		}
	}
	return nil
}

// shellRun is the default command runner: shell.Run with no deadline, which
// trims trailing newlines like a shell substitution. WithCommandContext bounds
// it; see options.command.
func shellRun(cmd string) (string, error) {
	return shell.Run(context.Background(), cmd)
}
