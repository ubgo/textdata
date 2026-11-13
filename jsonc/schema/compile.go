package schema

import (
	"fmt"
	"math"
	"math/big"
	"regexp"
	"strconv"
	"strings"
)

// maxExactExponent bounds the decimal exponent compared exactly; beyond it a
// number is compared as float64, so "1e999999999" cannot make big.Rat
// allocate gigabytes.
const maxExactExponent = 1000

// num is a JSON number: exact as a big.Rat when its exponent is modest,
// otherwise approximated as a float64 (possibly ±Inf).
type num struct {
	rat *big.Rat
	f   float64
}

// parseNum parses JSON number text.
func parseNum(s string) (num, bool) {
	_, exp, hasExp := strings.Cut(strings.ToLower(s), "e")
	if hasExp {
		e, err := strconv.Atoi(exp)
		if err != nil || e > maxExactExponent || e < -maxExactExponent {
			f, err := strconv.ParseFloat(s, 64)
			if err != nil && !isRangeErr(err) {
				return num{}, false
			}
			return num{f: f}, true
		}
	}
	r, ok := new(big.Rat).SetString(s)
	if !ok {
		return num{}, false
	}
	return num{rat: r}, true
}

func isRangeErr(err error) bool {
	ne, ok := err.(*strconv.NumError)
	return ok && ne.Err == strconv.ErrRange
}

func (n num) float() float64 {
	if n.rat != nil {
		f, _ := n.rat.Float64()
		return f
	}
	return n.f
}

// cmp compares exactly when both sides are exact.
func (n num) cmp(o num) int {
	if n.rat != nil && o.rat != nil {
		return n.rat.Cmp(o.rat)
	}
	a, b := n.float(), o.float()
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

func (n num) isInt() bool {
	if n.rat != nil {
		return n.rat.IsInt()
	}
	return !math.IsInf(n.f, 0) && n.f == math.Trunc(n.f)
}

func (n num) sign() int {
	if n.rat != nil {
		return n.rat.Sign()
	}
	switch {
	case n.f < 0:
		return -1
	case n.f > 0:
		return 1
	}
	return 0
}

// clampInt returns an integer value clamped to the int range.
func (n num) clampInt() int {
	if n.rat != nil && n.rat.IsInt() && n.rat.Num().IsInt64() {
		v := n.rat.Num().Int64()
		if v > math.MaxInt32 {
			return math.MaxInt32
		}
		return int(v)
	}
	return math.MaxInt32
}

// compiled is the checked form of one schema node, built per Validate call
// so the Schema itself is never written to.
type compiled struct {
	re                                 *regexp.Regexp
	ref                                *Schema
	minimum, maximum, exclMin, exclMax *num
	unsupported                        []string
}

// compiler resolves references and compiles patterns for a schema tree.
type compiler struct {
	root  *Schema
	nodes map[*Schema]*compiled
}

// compile checks every node reachable from root.
func compile(root *Schema) (*compiler, error) {
	c := &compiler{root: root, nodes: map[*Schema]*compiled{}}
	if err := c.visit(root, "#"); err != nil {
		return nil, err
	}
	return c, nil
}

func (c *compiler) fail(loc, format string, args ...any) error {
	return fmt.Errorf("%w: %s: %s", ErrSchema, loc, fmt.Sprintf(format, args...))
}

func (c *compiler) visit(s *Schema, loc string) error {
	if s == nil || s.Bool != nil {
		return nil
	}
	if _, done := c.nodes[s]; done {
		return nil
	}
	n := &compiled{}
	c.nodes[s] = n
	if s.Pattern != "" {
		re, err := regexp.Compile(s.Pattern)
		if err != nil {
			return c.fail(loc+"/"+kwPattern, "pattern does not compile as a Go (RE2) regexp: %v", err)
		}
		n.re = re
	}
	for _, b := range []struct {
		kw  string
		src string
		dst **num
	}{
		{kwMinimum, string(s.Minimum), &n.minimum},
		{kwMaximum, string(s.Maximum), &n.maximum},
		{kwExclusiveMinimum, string(s.ExclusiveMinimum), &n.exclMin},
		{kwExclusiveMaximum, string(s.ExclusiveMaximum), &n.exclMax},
	} {
		if b.src == "" {
			continue
		}
		v, ok := parseNum(b.src)
		if !ok {
			return c.fail(loc+"/"+b.kw, "%q is not a number", b.src)
		}
		*b.dst = &v
	}
	for _, t := range s.Types {
		if !typeNames[t] {
			return c.fail(loc+"/"+kwType, "unknown type %q", t)
		}
	}
	for _, k := range sortedKeys(s.Extra) {
		if !annotations[k] {
			n.unsupported = append(n.unsupported, k)
		}
	}
	if s.Ref != "" {
		target, err := c.resolve(s.Ref)
		if err != nil {
			return c.fail(loc+"/"+kwRef, "%v", err)
		}
		n.ref = target
	}
	for _, name := range sortedKeys(s.Defs) {
		if err := c.visit(s.Defs[name], loc+"/"+kwDefs+"/"+escape(name)); err != nil {
			return err
		}
	}
	for _, name := range sortedKeys(s.Properties) {
		if err := c.visit(s.Properties[name], loc+"/"+kwProperties+"/"+escape(name)); err != nil {
			return err
		}
	}
	if err := c.visit(s.Items, loc+"/"+kwItems); err != nil {
		return err
	}
	if err := c.visit(s.AdditionalProperties, loc+"/"+kwAdditional); err != nil {
		return err
	}
	if err := c.visit(s.Not, loc+"/"+kwNot); err != nil {
		return err
	}
	for _, g := range []struct {
		kw   string
		list []*Schema
	}{{kwAllOf, s.AllOf}, {kwAnyOf, s.AnyOf}, {kwOneOf, s.OneOf}} {
		for i, sub := range g.list {
			if err := c.visit(sub, loc+"/"+g.kw+"/"+strconv.Itoa(i)); err != nil {
				return err
			}
		}
	}
	if n.ref != nil {
		return c.visit(n.ref, s.Ref)
	}
	return nil
}

// resolve follows a local reference: "#" or a JSON Pointer into $defs,
// properties, items and additionalProperties.
func (c *compiler) resolve(ref string) (*Schema, error) {
	if !strings.HasPrefix(ref, "#") {
		return nil, fmt.Errorf("$ref %q: only local references (\"#...\") are supported", ref)
	}
	frag := ref[1:]
	if frag == "" {
		return c.root, nil
	}
	if !strings.HasPrefix(frag, "/") {
		return nil, fmt.Errorf("$ref %q: anchors are not supported, use a JSON Pointer", ref)
	}
	toks := strings.Split(frag[1:], "/")
	cur := c.root
	for i := 0; i < len(toks); i++ {
		if cur == nil || cur.Bool != nil {
			return nil, fmt.Errorf("$ref %q does not resolve", ref)
		}
		tok := unescape(toks[i])
		switch tok {
		case kwItems:
			cur = cur.Items
			continue
		case kwAdditional:
			cur = cur.AdditionalProperties
			continue
		case kwNot:
			cur = cur.Not
			continue
		case kwAllOf, kwAnyOf, kwOneOf:
			list := map[string][]*Schema{kwAllOf: cur.AllOf, kwAnyOf: cur.AnyOf, kwOneOf: cur.OneOf}[tok]
			if i+1 >= len(toks) {
				return nil, fmt.Errorf("$ref %q does not resolve", ref)
			}
			i++
			idx, err := strconv.Atoi(toks[i])
			if err != nil || idx < 0 || idx >= len(list) {
				return nil, fmt.Errorf("$ref %q does not resolve", ref)
			}
			cur = list[idx]
			continue
		case kwDefs, kwProperties:
			if i+1 >= len(toks) {
				return nil, fmt.Errorf("$ref %q does not resolve", ref)
			}
			m := cur.Defs
			if tok == kwProperties {
				m = cur.Properties
			}
			i++
			cur = m[unescape(toks[i])]
			continue
		}
		return nil, fmt.Errorf("$ref %q: cannot follow %q", ref, tok)
	}
	if cur == nil {
		return nil, fmt.Errorf("$ref %q does not resolve", ref)
	}
	return cur, nil
}

func escape(s string) string { return strings.NewReplacer("~", "~0", "/", "~1").Replace(s) }

func unescape(s string) string { return strings.NewReplacer("~1", "/", "~0", "~").Replace(s) }
