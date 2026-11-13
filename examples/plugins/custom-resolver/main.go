// Command custom-resolver writes a value plugin from scratch against the
// shared plugin package and installs it in dotenv and JSONC. The plugin is a
// Resolver for ${mem:ref} and a Lookuper for plain ${NAME}, backed by a map
// standing in for a real secret store.
package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/ubgo/textdata/dotenv"
	"github.com/ubgo/textdata/jsonc"
	"github.com/ubgo/textdata/plugin"
)

// memScheme is the scheme the plugin owns: ${mem:ref}. Lower case, no colon.
const memScheme = "mem"

// refDown is a reference the fake store treats as an outage, to show that a
// store failure is an error and never an empty value.
const refDown = "down"

// Store is a value plugin over an in-memory map. A real one would hold a
// client and a cache; the shape is the same.
//
//ds:def id=custom-store-decl-bkfm37jz span=+20
type Store struct {
	name   string
	values map[string]string
	// lookups maps plain ${NAME} references to store refs; a name not in it
	// is "not mine", so the next Lookuper is asked.
	lookups map[string]string
}

// Compile-time proof of the abilities Store claims. Without these lines a
// typo in a method signature compiles, installs, and is silently never called.
var (
	_ plugin.Plugin   = (*Store)(nil)
	_ plugin.Resolver = (*Store)(nil)
	_ plugin.Lookuper = (*Store)(nil)
)

// Name implements plugin.Plugin; errors name the plugin with it.
func (s *Store) Name() string { return s.name }

// Schemes implements plugin.Resolver.
func (s *Store) Schemes() []string { return []string{memScheme} }

// Resolve implements plugin.Resolver. It refuses a scheme it does not own and
// wraps the base's named errors so callers can branch with errors.Is.
//
//ds:def id=custom-resolve-d3rb33qv
func (s *Store) Resolve(scheme, ref string) (string, error) {
	if !strings.EqualFold(scheme, memScheme) {
		return "", fmt.Errorf("%s: scheme %q: %w", s.name, scheme, plugin.ErrUnsupportedScheme)
	}
	if ref == refDown {
		return "", fmt.Errorf("%s: %s: %w", s.name, ref, plugin.ErrUnavailable)
	}
	v, ok := s.values[ref]
	if !ok {
		return "", fmt.Errorf("%s: %s: %w", s.name, ref, plugin.ErrNotFound)
	}
	return v, nil
}

// Lookup implements plugin.Lookuper. A Lookuper says "not mine" with
// ok=false (never ErrNotFound) so the next one is tried; a store failure is
// still an error.
//
//ds:def id=custom-lookup-pdggg4g9
func (s *Store) Lookup(name string) (string, bool, error) {
	ref, ok := s.lookups[name]
	if !ok {
		return "", false, nil
	}
	v, err := s.Resolve(memScheme, ref)
	if errors.Is(err, plugin.ErrNotFound) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return v, true, nil
}

// envSource mixes a reference to the file's own value (the file wins over
// every Lookuper, even one mapping REGION), plugin lookups, a scheme reference,
// a scheme nobody owns, and an outage.
const envSource = `REGION=eu-west-1
ZONE=${REGION}-a
DB_HOST=${DB_HOST}
CACHE_HOST=${CACHE_HOST}
DB_PASSWORD=${mem:prod/db}
TYPO=${mme:prod/db}
BROKEN=${mem:down}
`

// install builds two stores and installs them in order: primary first. A
// Lookuper's first non-miss wins, so primary answers DB_HOST; fallback is
// asked only for names primary does not map.
//
//ds:def id=custom-install-3yhpjcb5
func install() []dotenv.Option {
	primary := &Store{
		name:    "primary",
		values:  map[string]string{"prod/db": "pw-primary", "hosts/db": "db.primary"},
		lookups: map[string]string{"DB_HOST": "hosts/db"},
	}
	fallback := &Store{
		name:    "fallback",
		values:  map[string]string{"prod/db": "pw-fallback", "hosts/db": "db.fallback", "hosts/cache": "cache.fallback"},
		lookups: map[string]string{"DB_HOST": "hosts/db", "CACHE_HOST": "hosts/cache", "REGION": "hosts/db"},
	}
	return []dotenv.Option{dotenv.WithPlugin(primary), dotenv.WithPlugin(fallback)}
}

// readDotenv shows what the plugins report and how each value reads.
//
//ds:def id=custom-readdotenv-ryprryaz
func readDotenv(w io.Writer) {
	f := dotenv.Parse(envSource, install()...)
	for _, info := range f.Plugins() {
		fmt.Fprintf(w, "installed: %s\n", info)
	}
	for _, key := range f.Keys() {
		v, _, err := f.GetExpanded(key)
		var perr *plugin.Error
		if errors.As(err, &perr) {
			fmt.Fprintf(w, "%-12s error from plugin %q, unavailable=%t\n", key+":", perr.Plugin, errors.Is(err, plugin.ErrUnavailable))
			continue
		}
		fmt.Fprintf(w, "%-12s %q\n", key+":", v)
	}
}

// readJSONC installs the same type in JSONC: one plugin, every format.
//
//ds:def id=custom-readjsonc-5ykgkc66
func readJSONC(w io.Writer) error {
	s := &Store{name: "primary", values: map[string]string{"prod/db": "pw-primary"}}
	doc, err := jsonc.Parse([]byte(`{"db": {"password": "${mem:prod/db}"}}`), jsonc.WithPlugin(s))
	if err != nil {
		return err
	}
	v, err := doc.GetExpanded("db.password")
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "%-12s %q\n", "jsonc:", v)
	return nil
}

// run reads both documents.
func run(w io.Writer) error {
	readDotenv(w)
	return readJSONC(w)
}

func main() {
	if err := run(os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
