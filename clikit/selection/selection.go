// Package selection is the selection grammar every textdata CLI shares: which
// keys a command acts on, under which names, and which values are held back
// as placeholders or empty.
//
// It knows no file format. A format's operations package (dotenv/cli/envkit)
// resolves a file into flat key/value pairs with these options and reports the
// result as a Selection, which provider plugins (clikit/providerkit) consume,
// so a provider works the same whichever format the values came from.
package selection

import (
	"fmt"
	"regexp"
)

// Options is the shared selection grammar: which keys, under which names,
// with which values.
//
// Resolution order, deterministic and pinned by the format's tests:
//
//	explicit Keys (as given; prefix filters do not second-guess them)
//	→ else: all active keys → Prefix keeps → ExcludePrefix drops
//	→ IncludeKeys force-add (bypassing the prefix filters)
//	→ ExcludeKeys drop
//	→ StripPrefix renames
//	→ the placeholder/empty guards classify what is left
//
// Invariant every caller depends on: a key the user NAMED (Keys or
// IncludeKeys) that does not exist is an error, never a silent skip —
// reporting success for work that never happened is the failure mode this
// grammar exists to prevent. Excluding an absent key is fine: "make sure X
// never ships" is valid even when X is already gone.
type Options struct {
	// Keys are explicit key names; empty means every active key.
	Keys []string
	// Prefix keeps only keys with this prefix ("" keeps all).
	Prefix string
	// ExcludePrefix drops keys with this prefix — the inverse selector, for
	// "everything EXCEPT the deploy-routing keys".
	ExcludePrefix string
	// IncludeKeys force-include specific keys even when the prefix filters
	// would drop them.
	IncludeKeys []string
	// ExcludeKeys drop specific keys. Matched against the key's ORIGINAL
	// name, before StripPrefix renaming.
	ExcludeKeys []string
	// StripPrefix removes Prefix from the emitted names. Keys that do not
	// carry the prefix (explicit or force-included) keep their names. Two
	// selected keys that end with the same name are an error naming both
	// (envkit.NameCollisionError for dotenv), never two pairs under one name.
	StripPrefix bool
	// Expand resolves ${references} against the file. Callers pushing values
	// outward normally want this ON — a literal ${DB_HOST} stored in a remote
	// secret is nearly always wrong. Readers normally want it OFF.
	Expand bool
	// IncludePlaceholders lifts the guard that classifies __LIKE_THIS__
	// values as skipped.
	IncludePlaceholders bool
	// IncludeEmpty lifts the guard that classifies empty values as skipped.
	IncludeEmpty bool
	// PlaceholderPattern overrides DefaultPlaceholderPattern. Empty means the
	// default.
	PlaceholderPattern string
}

// SkipReason says why a selected key was held back by a guard. Closed set;
// callers switch on it to report or to override.
type SkipReason string

const (
	// SkipPlaceholder: the value matches the placeholder pattern — configured
	// but still a stand-in.
	SkipPlaceholder SkipReason = "placeholder"
	// SkipEmpty: the value is the empty string.
	SkipEmpty SkipReason = "empty"
)

// Skip is one guarded-out key, reported under its final (post-rename) name so
// callers name it the way the destination would.
type Skip struct {
	Name   string     `json:"name"`
	Reason SkipReason `json:"reason"`
}

// Pair is one selected key and its value, in the destination's naming.
type Pair struct {
	Key   string
	Value string
}

// Selection is the resolved result: what to act on, and what was held back.
//
// Skipped is deliberately part of the result rather than swallowed — a caller
// that only sees Pairs cannot tell "nothing matched" from "everything was a
// placeholder", and those need different responses from a human.
type Selection struct {
	Pairs   []Pair
	Skipped []Skip
}

// Names returns the selected pair names in order.
func (s *Selection) Names() []string {
	out := make([]string, 0, len(s.Pairs))
	for _, p := range s.Pairs {
		out = append(out, p.Key)
	}
	return out
}

// SkippedNames returns the held-back names in order.
func (s *Selection) SkippedNames() []string {
	out := make([]string, 0, len(s.Skipped))
	for _, sk := range s.Skipped {
		out = append(out, sk.Name)
	}
	return out
}

// Map returns the selection as a plain map, last-wins.
func (s *Selection) Map() map[string]string {
	out := make(map[string]string, len(s.Pairs))
	for _, p := range s.Pairs {
		out[p.Key] = p.Value
	}
	return out
}

// DefaultPlaceholderPattern marks values that are stand-ins rather than real
// configuration — the `__YOU__` / `__CONFIRM__` convention. Exported because
// callers need the same default the CLI uses, and a caller with a different
// convention needs to know what it is overriding.
const DefaultPlaceholderPattern = `^__[A-Z0-9_]+__$`

// defaultPlaceholderRe is the compiled default, compiled once per process.
var defaultPlaceholderRe = regexp.MustCompile(DefaultPlaceholderPattern)

// PlaceholderMatcher resolves an optional caller pattern to a compiled regexp,
// falling back to the shared default. A bad pattern is an error, never a
// panic: the pattern is user input in every caller.
func PlaceholderMatcher(pattern string) (*regexp.Regexp, error) {
	if pattern == "" || pattern == DefaultPlaceholderPattern {
		return defaultPlaceholderRe, nil
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, fmt.Errorf("placeholder pattern %q: %w", pattern, err)
	}
	return re, nil
}
