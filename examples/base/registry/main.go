// Command registry writes plugins against the base plugin package and installs
// the same values in two formats. It shows capability detection (and how a
// wrong method signature is caught), the Registry, plugin errors, and a
// SaveGuard reading the format-neutral Document.
package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/ubgo/textdata/dotenv"
	"github.com/ubgo/textdata/jsonc"
	"github.com/ubgo/textdata/plugin"
)

// Plugin names and the one scheme the store owns, named once so errors and
// reports print the same strings the code compares against.
const (
	storeName    = "store"
	guardName    = "no-private-keys"
	brokenName   = "upper"
	storeScheme  = "store"
	pemMarker    = "-----BEGIN"
	capCloser    = plugin.Capability("Closer")
	errorPrefix  = "demo"
	downKey      = "DOWN"
	sampleSecret = "hush"
)

func main() {
	dir, err := os.MkdirTemp("", "registry-example-")
	if err == nil {
		err = run(os.Stdout, dir)
		// Cleanup is best-effort: the run's result matters more than a
		// leftover temp directory.
		_ = os.RemoveAll(dir)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// run executes every step of the guide in order.
func run(w io.Writer, dir string) error {
	capabilities(w)
	registry(w)
	pluginErrors(w)
	if err := lookupInBothFormats(w); err != nil {
		return err
	}
	return guardInBothFormats(w, dir)
}

// store is one plugin with two abilities: Lookuper for names the file leaves
// undefined, Resolver for ${store:ref}. Sharing one value lets both abilities
// use one client and one cache in a real secret store.
//
//ds:def id=store-wgesjcub
type store struct{ values map[string]string }

func (s store) Name() string { return storeName }

//ds:def id=store-lookup-2fkdgvx4
func (s store) Lookup(name string) (string, bool, error) {
	if name == downKey {
		// Down is an error, never "not found": an unreachable store must not
		// look like an unset variable.
		return "", false, fmt.Errorf("dial store: %w", plugin.ErrUnavailable)
	}
	v, ok := s.values[name]
	return v, ok, nil
}

func (s store) Schemes() []string { return []string{storeScheme} }

//ds:def id=store-resolve-b55eetqv
func (s store) Resolve(scheme, ref string) (string, error) {
	if scheme != storeScheme {
		return "", plugin.ErrUnsupportedScheme
	}
	v, ok := s.values[ref]
	if !ok {
		return "", fmt.Errorf("%s: %w", ref, plugin.ErrNotFound)
	}
	return v, nil
}

// noPrivateKeys vetoes a save that would write a PEM private key in clear.
// It reads only the base Document, so the one type guards every format.
//
//ds:def id=noprivatekeys-cx6t7r74
type noPrivateKeys struct{}

func (noPrivateKeys) Name() string { return guardName }

//ds:def id=noprivatekeys-guardsave-786rpfc6
func (noPrivateKeys) GuardSave(doc plugin.Document) error {
	for _, p := range doc.ValuePaths() {
		if v, _ := doc.ValueAt(p); strings.Contains(v, pemMarker) {
			return fmt.Errorf("%s %s: %s holds a private key: %w", doc.FormatName(), filepath.Base(doc.Location()), p, plugin.ErrDenied)
		}
	}
	return nil
}

// upper has a TransformValue with the wrong parameter type. It compiles and
// installs, and would never run; capability detection is how you find out.
//
//ds:def id=upper-eumvk3jg
type upper struct{}

func (upper) Name() string                                    { return brokenName }
func (upper) TransformValue(path string, value []byte) string { return strings.ToUpper(string(value)) }

// capabilities checks each plugin against every known capability, in the
// fixed reporting order plugin.Capabilities gives.
//
//ds:def id=capabilities-c6u7z394
func capabilities(w io.Writer) {
	for _, p := range []plugin.Plugin{store{}, noPrivateKeys{}, upper{}} {
		var got []string
		for _, c := range plugin.Capabilities {
			if plugin.Has(p, c) {
				got = append(got, string(c))
			}
		}
		fmt.Fprintf(w, "%-16s %v\n", p.Name()+":", got)
	}
}

// registry keeps plugins in install order, adds a capability this package does
// not define (a Check), and filters by interface with Filter.
//
//ds:def id=registry-v5n983ak
func registry(w io.Writer) {
	r := plugin.NewRegistry(plugin.Check{Cap: capCloser, Has: func(p plugin.Plugin) bool {
		_, ok := p.(io.Closer)
		return ok
	}})
	r.Add(store{}, noPrivateKeys{}, upper{}, nil) // a nil plugin is ignored
	for _, info := range r.Info() {
		fmt.Fprintln(w, "info:", info)
	}
	guards := plugin.Filter[plugin.SaveGuard](r)
	fmt.Fprintf(w, "save guards: %d, first resolver: %s, second resolver: %s\n",
		len(guards), r.NameFor(plugin.CapResolver, 0), r.NameFor(plugin.CapResolver, 1))
}

// pluginErrors wraps a plugin failure so the message names the plugin and the
// named error stays reachable through errors.Is.
//
//ds:def id=pluginerrors-874f9ryb
func pluginErrors(w io.Writer) {
	_, _, cause := store{}.Lookup(downKey)
	err := plugin.Errorf(errorPrefix, storeName, "lookup "+downKey, cause)
	var pe *plugin.Error
	fmt.Fprintf(w, "error: %v\nunavailable=%v plugin=%v\n",
		err, errors.Is(err, plugin.ErrUnavailable), errors.As(err, &pe) && pe.Plugin == storeName)
}

// lookupInBothFormats installs one store in dotenv and in jsonc. Each format
// asks it for the names and schemes its own file leaves unresolved.
//
//ds:def id=lookupinbothformats-cyx5gztt
func lookupInBothFormats(w io.Writer) error {
	s := store{values: map[string]string{"DB_PASSWORD": sampleSecret, "api/token": "t-123"}}

	env := dotenv.Parse("DSN=postgres://app@db/app?password=${DB_PASSWORD}\nTOKEN=${store:api/token}\n", dotenv.WithPlugin(s))
	dsn, _, err := env.GetExpanded("DSN")
	if err != nil {
		return err
	}
	token, _, err := env.GetExpanded("TOKEN")
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "dotenv: DSN=%s TOKEN=%s\n", dsn, token)

	doc, err := jsonc.Parse([]byte(`{"db": {"password": "${DB_PASSWORD}"}, "token": "${store:api/token}"}`), jsonc.WithPlugin(s))
	if err != nil {
		return err
	}
	pw, err := doc.GetExpanded("db.password")
	if err != nil {
		return err
	}
	tok, err := doc.GetExpanded("token")
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "jsonc:  db.password=%v token=%v\n", pw, tok)
	return nil
}

// privateKey is a value the guard must keep off disk.
const privateKey = pemMarker + " PRIVATE KEY-----"

// guardInBothFormats opens a .env and a .jsonc with the same SaveGuard, puts a
// private key into each and tries to save. Both saves are refused and the
// files on disk keep their old bytes.
//
//ds:def id=guardinbothformats-jr8cm64t
func guardInBothFormats(w io.Writer, dir string) error {
	envPath := filepath.Join(dir, ".env")
	if err := os.WriteFile(envPath, []byte("TLS_KEY=\n"), 0o600); err != nil {
		return err
	}
	env, err := dotenv.Open(envPath, dotenv.WithPlugin(noPrivateKeys{}))
	if err != nil {
		return err
	}
	env.Set("TLS_KEY", privateKey)
	envErr := env.Save()
	fmt.Fprintf(w, "dotenv save: %s (denied=%v)\n", hideDir(envErr, dir), errors.Is(envErr, plugin.ErrDenied))
	onDisk, err := os.ReadFile(envPath)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, ".env on disk: %q\n", onDisk)
	return guardJSONC(w, dir)
}

// guardJSONC is the same guard on a JSONC file: one plugin, two formats.
//
//ds:def id=guardjsonc-hzz8h899
func guardJSONC(w io.Writer, dir string) error {
	jsonPath := filepath.Join(dir, "config.jsonc")
	if err := os.WriteFile(jsonPath, []byte(`{"tls": {"key": ""}}`+"\n"), 0o600); err != nil {
		return err
	}
	doc, err := jsonc.Open(jsonPath, jsonc.WithPlugin(noPrivateKeys{}))
	if err != nil {
		return err
	}
	if err := doc.Set("tls.key", privateKey); err != nil {
		return err
	}
	jsonErr := doc.Save()
	fmt.Fprintf(w, "jsonc save:  %s (denied=%v)\n", hideDir(jsonErr, dir), errors.Is(jsonErr, plugin.ErrDenied))
	fmt.Fprintf(w, "plugins seen by jsonc: %v\n", doc.Plugins())
	return nil
}

// hideDir replaces the temp directory in an error so the output is the same on
// every run and every machine.
func hideDir(err error, dir string) string {
	if err == nil {
		return "<nil>"
	}
	return strings.ReplaceAll(err.Error(), dir, "$DIR")
}
