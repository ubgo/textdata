package jsonc

import "github.com/ubgo/textdata/pos"

// Pos returns where the value at path starts in the source (1-based line,
// byte column). Validators, lint rules, diffs and config sources use it to
// report file:line:col for a path without reaching into the tree. Pos and
// KeyPos are safe to call from several goroutines at once on a Doc that is
// not being edited.
func (d *Doc) Pos(p string) (pos.Pos, error) {
	loc, _, err := d.lookup(p)
	if err != nil {
		return pos.Pos{}, err
	}
	return d.at(loc.n.start), nil
}

// KeyPos returns where the member name of the value at path starts. For an
// array element or the root, which have no key, it is the value's position.
func (d *Doc) KeyPos(p string) (pos.Pos, error) {
	loc, _, err := d.lookup(p)
	if err != nil {
		return pos.Pos{}, err
	}
	if loc.it != nil && loc.it.keyStart >= 0 {
		return d.at(loc.it.keyStart), nil
	}
	return d.at(loc.n.start), nil
}
