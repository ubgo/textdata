package envkit

import (
	"slices"
	"strings"

	"github.com/ubgo/textdata/clikit/selection"
	"github.com/ubgo/textdata/dotenv"
)

// SelectOptions is the shared selection grammar — an alias of the
// format-neutral clikit/selection.Options, so every textdata CLI and every
// provider plugin means the same thing by --prefix, --include, --strip-prefix
// and the placeholder/empty guards. See selection.Options for the resolution
// order and the named-key invariant.
type SelectOptions = selection.Options

// SkipReason says why a selected key was held back by a guard; an alias of
// selection.SkipReason.
type SkipReason = selection.SkipReason

const (
	// SkipPlaceholder: the value matches the placeholder pattern — configured
	// but still a stand-in.
	SkipPlaceholder = selection.SkipPlaceholder
	// SkipEmpty: the value is the empty string.
	SkipEmpty = selection.SkipEmpty
)

// Skip is one guarded-out key, reported under its final (post-rename) name;
// an alias of selection.Skip.
type Skip = selection.Skip

// Selection is the resolved result: what to act on, and what was held back.
//
// Skipped is deliberately part of the result rather than swallowed — a caller
// that only sees Pairs cannot tell "nothing matched" from "everything was a
// placeholder", and those need different responses from a human.
type Selection struct {
	Pairs   []dotenv.Pair
	Skipped []Skip
}

// Names returns the selected pair names in order — the common projection for
// banners, confirmations, and set arithmetic.
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

// Flat returns the selection as the format-neutral selection.Selection that
// provider plugins (clikit/providerkit) consume.
func (s *Selection) Flat() *selection.Selection {
	out := &selection.Selection{Skipped: s.Skipped}
	for _, p := range s.Pairs {
		out.Pairs = append(out.Pairs, selection.Pair{Key: p.Key, Value: p.Value})
	}
	return out
}

// Map returns the selection as a plain map, last-wins — for callers handing
// values to something that wants a map (template data, an env builder).
func (s *Selection) Map() map[string]string {
	out := make(map[string]string, len(s.Pairs))
	for _, p := range s.Pairs {
		out[p.Key] = p.Value
	}
	return out
}

// SelectFile opens path and applies opts. The file must exist.
func SelectFile(path string, opts SelectOptions) (*Selection, error) {
	f, err := openExisting(path)
	if err != nil {
		return nil, err
	}
	return Select(f, opts)
}

// Select applies the selection grammar to an already-parsed file — the
// overload for callers holding a *dotenv.File who must not pay to re-parse.
func Select(f *dotenv.File, opts SelectOptions) (*Selection, error) {
	placeholderRe, err := placeholderMatcher(opts.PlaceholderPattern)
	if err != nil {
		return nil, err
	}
	values, err := effectiveValues(f, opts.Expand)
	if err != nil {
		return nil, err
	}
	keys, err := selectKeys(f, opts)
	if err != nil {
		return nil, err
	}

	sel := &Selection{}
	// source maps each final name to the key it came from; a second key with
	// the same final name is a NameCollisionError, never a duplicate pair.
	source := make(map[string]string, len(keys))
	for _, key := range keys {
		value := values[key]
		name := key
		if opts.StripPrefix && opts.Prefix != "" {
			name = strings.TrimPrefix(key, opts.Prefix)
		}
		if first, dup := source[name]; dup {
			return nil, &NameCollisionError{Name: name, First: first, Second: key}
		}
		source[name] = key
		switch {
		case !opts.IncludePlaceholders && placeholderRe.MatchString(value):
			sel.Skipped = append(sel.Skipped, Skip{Name: name, Reason: SkipPlaceholder})
		case !opts.IncludeEmpty && value == "":
			sel.Skipped = append(sel.Skipped, Skip{Name: name, Reason: SkipEmpty})
		default:
			sel.Pairs = append(sel.Pairs, dotenv.Pair{Key: name, Value: value})
		}
	}
	return sel, nil
}

// selectKeys resolves which ORIGINAL key names the selection covers, in file
// order, applying SelectOptions' documented precedence.
func selectKeys(f *dotenv.File, opts SelectOptions) ([]string, error) {
	var keys []string
	switch {
	case len(opts.Keys) > 0:
		for _, k := range opts.Keys {
			if !f.Has(k) {
				return nil, &SelectKeyError{Key: k}
			}
		}
		keys = slices.Clone(opts.Keys)
	default:
		for _, k := range f.Keys() {
			if opts.Prefix != "" && !strings.HasPrefix(k, opts.Prefix) {
				continue
			}
			if opts.ExcludePrefix != "" && strings.HasPrefix(k, opts.ExcludePrefix) {
				continue
			}
			keys = append(keys, k)
		}
	}

	for _, k := range opts.IncludeKeys {
		if !f.Has(k) {
			return nil, &SelectKeyError{Key: k, Include: true}
		}
		if !slices.Contains(keys, k) {
			keys = append(keys, k)
		}
	}

	if len(opts.ExcludeKeys) > 0 {
		kept := keys[:0]
		for _, k := range keys {
			if !slices.Contains(opts.ExcludeKeys, k) {
				kept = append(kept, k)
			}
		}
		keys = kept
	}
	return keys, nil
}
