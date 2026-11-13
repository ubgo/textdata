package dotenv

import (
	"fmt"
	"strings"

	"github.com/ubgo/textdata/expand"
)

// ds:def id=maxexpanddepth-hr5dfhv2 owner=@maintainers stability=stable

// maxExpandDepth bounds how deep a chain of references may nest.
//
// Real chains are one hop deep — a per-service variable pointing at a shared
// global — so 32 is astronomically generous.
//
// Depth alone is NOT enough to guarantee termination. A value that references
// itself twice, such as A=$A$A, branches at every level: bounded at depth 32
// that is still 2^32 expansions, which hangs rather than returns. Cycles are
// therefore broken by name via the expansion stack (see source), and the depth
// bound is only a backstop for deep-but-acyclic chains.
const maxExpandDepth = expand.DefaultMaxDepth

// RequiredError reports a ${VAR:?message} or ${VAR?message} reference whose
// variable was not satisfied.
//
// These forms exist precisely to fail loudly: silently yielding "" would defeat
// the only reason an author writes one.
type RequiredError struct {
	// Key is the referenced variable that was unset or empty.
	Key string
	// Message is the text after the operator. Empty when the author wrote none.
	Message string
	// Empty distinguishes ":?" (unset OR empty) from "?" (unset only).
	Empty bool
}

func (e *RequiredError) Error() string {
	reason := "is not set"
	if e.Empty {
		reason = "is not set or is empty"
	}
	if e.Message == "" {
		return fmt.Sprintf("dotenv: required variable %s %s", e.Key, reason)
	}
	return fmt.Sprintf("dotenv: required variable %s %s: %s", e.Key, reason, e.Message)
}

// operator, varRef, parseVarToken, matchBrace and matchParen are the names
// this package used before the expansion engine moved to the shared base
// (github.com/ubgo/textdata/expand). They stay so the package's own tests keep
// pinning the grammar through dotenv, unchanged.
type operator = expand.Op

const (
	opNone         = expand.OpNone         // ${VAR}
	opDefault      = expand.OpDefault      // ${VAR-default}   default when unset
	opDefaultEmpty = expand.OpDefaultEmpty // ${VAR:-default}  default when unset or empty
	opAlternate    = expand.OpAlternate    // ${VAR+alt}       alt when set
	opAlternateSet = expand.OpAlternateSet // ${VAR:+alt}      alt when set and non-empty
	opRequired     = expand.OpRequired     // ${VAR?err}       error when unset
	opRequiredSet  = expand.OpRequiredSet  // ${VAR:?err}      error when unset or empty
)

// varRef is a parsed ${...} reference.
type varRef struct {
	varName string
	op      operator
	// arg is the text after the operator: a default, an alternate, or an error
	// message, depending on op.
	arg string
}

// parseVarToken splits the interior of a ${...} reference into a variable
// name and its operator; see expand.ParseRef.
func parseVarToken(tok string) varRef {
	r := expand.ParseRef(tok)
	return varRef{varName: r.Name, op: r.Op, arg: r.Arg}
}

// matchBrace returns the index of the `}` closing the `{` at s[0]; see
// expand.MatchBrace.
func matchBrace(s string) int { return expand.MatchBrace(s) }

// matchParen returns the index of the `)` closing the `(` at s[0]; see
// expand.MatchParen.
func matchParen(s string) int { return expand.MatchParen(s) }

// expandFor expands s, the value of the entry with the given key, through the
// shared engine with dotenv's rules plugged in: the file's own entries first
// (see source), $(command) only when substitution is on, ${scheme:ref} only for
// an installed Resolver's scheme, and *RequiredError for ${VAR:?msg}.
//
// seen carries the expansion stack; expandEntry seeds it with the entry's own
// key at its index, so a reference back to it sees only the definitions above.
func (f *File) expandFor(key, s string, seen *expand.Stack) (string, error) {
	eng := expand.Engine{
		Lookup:   f.source,
		Required: requiredError,
		MaxDepth: maxExpandDepth,
	}
	if f.opts.allowShell {
		eng.Run = f.opts.runCommand
	}
	if len(f.opts.resolvers) > 0 {
		eng.Resolve = f.resolveScheme
	}
	if len(f.opts.expandObs) > 0 {
		eng.Observe = func(name, resolved string) {
			for _, o := range f.opts.expandObs {
				o.ObserveExpand(key, name, resolved)
			}
		}
	}
	return eng.Expand(s, seen)
}

// requiredError builds dotenv's own error type for the engine, so callers'
// errors.As(&*dotenv.RequiredError) keeps working.
func requiredError(name, message string, empty bool) error {
	return &RequiredError{Key: name, Message: message, Empty: empty}
}

// resolveScheme hands ${scheme:ref} to the first installed Resolver that owns
// the scheme. No owner means "not mine": the engine then reads the token as an
// ordinary reference, exactly as before resolvers existed.
func (f *File) resolveScheme(scheme, ref string) (string, bool, error) {
	for i, r := range f.opts.resolvers {
		for _, s := range r.Schemes() {
			if !strings.EqualFold(s, scheme) {
				continue
			}
			v, err := r.Resolve(scheme, ref)
			if err != nil {
				return "", true, pluginError(f.opts.pluginNameFor(CapResolver, i), "resolve "+scheme+":"+ref, err)
			}
			return v, true, nil
		}
	}
	return "", false, nil
}

// source finds a variable's value: the file first, then any installed
// Lookuper.
//
// The file wins. Adding an environment fallback or a secret store must not
// silently override a value somebody wrote down. (Compose's own lookup runs the
// other way round, process environment first; this package never reads the
// environment, and a Lookuper is a fallback by contract.)
//
// Which file entry: the LAST definition of name, matching Get — except while
// name is already being expanded. Then the reference is a self-reference or a
// cycle, and it sees only the definitions ABOVE the entry on the stack, as in
// Compose's top-to-bottom read; with none above, it falls through to the
// Lookupers and then to unset. A name on the stack whose value a Lookuper
// supplied (expand.AtLookup) has nothing earlier to fall back to and is unset;
// asking the Lookuper again would loop. A single-quoted or backtick entry is
// literal: a reference to it inserts the text as written, as Compose does.
//
// A Lookuper error aborts expansion rather than being treated as a miss: a
// secret store that is unreachable must not look like a variable that is unset,
// or a deploy quietly proceeds with an empty password.
func (f *File) source(name string, seen *expand.Stack) (expand.Var, error) {
	below := len(f.entries)
	if at, onStack := seen.At(name); onStack {
		if at == expand.AtLookup {
			return expand.Var{}, nil
		}
		below = at
	}
	for i := below - 1; i >= 0; i-- {
		if e := f.entries[i]; e.Kind == KindPair && e.Key == name {
			return expand.Var{Value: e.Value, Set: true, At: i, Literal: !e.quote.interpolates()}, nil
		}
	}

	for i, l := range f.opts.lookupers {
		v, ok, err := l.Lookup(name)
		if err != nil {
			return expand.Var{}, pluginError(f.opts.pluginNameFor(CapLookup, i), "lookup "+name, err)
		}
		if ok {
			// First to answer wins.
			return expand.Var{Value: v, Set: true, At: expand.AtLookup}, nil
		}
	}
	return expand.Var{}, nil
}
