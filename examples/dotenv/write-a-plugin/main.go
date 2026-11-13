// Command write-a-plugin writes a dotenv plugin type with two capabilities
// sharing one store: a Lookuper that fills ${DB_PASSWORD} from a secret store,
// and a SaveGuard that refuses to save a file holding one of those secrets in
// plain text. It also writes a cross-format guard against the shared plugin
// package, checks what was detected with Plugins, and shows what happens when
// the store is down.
//
// Run it with `go run .`; it works in a fresh temp directory and the store is
// an in-memory map.
package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/ubgo/textdata/dotenv"
	"github.com/ubgo/textdata/plugin"
)

// envFile is the file the run writes inside its own directory.
const envFile = ".env"

// source references the secret instead of holding it.
const source = "DB_USER=app\nDB_URL=postgres://${DB_USER}@db/app?password=${DB_PASSWORD}\n"

// pluginName is what Plugins and every error message call the vault plugin.
const pluginName = "vault"

// errLeak is the guard's refusal; callers match it with errors.Is.
var errLeak = errors.New("plain-text secret in file")

// vault answers lookups from an in-memory store standing in for a secret
// manager. down simulates an unreachable store.
//
//ds:def id=vaulttype-c4nm8r2t
type vault struct {
	secrets map[string]string
	down    bool
}

// Compile-time assertions: a method with a wrong name or signature would
// otherwise compile, install, and silently never run.
//
//ds:def id=vaultasserts-f8kw2n5d
var (
	_ dotenv.Plugin    = (*vault)(nil)
	_ dotenv.Lookuper  = (*vault)(nil)
	_ dotenv.SaveGuard = (*vault)(nil)
)

// Name labels the plugin in Plugins and in every error it causes.
func (v *vault) Name() string { return pluginName }

// Lookup runs only for a ${VAR} the file does not define. ok=false means "not
// mine" and the next Lookuper is asked; an error means "could not check" and
// aborts the read, so an outage never looks like an empty password.
//
//ds:def id=vaultlookup-h6xq3w9k
func (v *vault) Lookup(name string) (string, bool, error) {
	if v.down {
		return "", false, fmt.Errorf("%w: connection refused", plugin.ErrUnavailable)
	}
	s, ok := v.secrets[name]
	return s, ok, nil
}

// GuardSave may only veto. It receives a copy of the file, reads the raw
// values (Get, never expanded), and refuses if any of them is a secret.
//
//ds:def id=vaultguard-m2dz7f5p
func (v *vault) GuardSave(f *dotenv.File) error {
	for _, key := range f.Keys() {
		raw, _ := f.Get(key)
		for name, secret := range v.secrets {
			if strings.Contains(raw, secret) {
				return fmt.Errorf("%w: %s holds %s", errLeak, key, name)
			}
		}
	}
	return nil
}

// requireKeys is a cross-format guard: it is written against plugin.Document,
// so the same type works in dotenv and in jsonc.
type requireKeys []string

var _ plugin.SaveGuard = requireKeys(nil)

func (requireKeys) Name() string { return "require-keys" }

//ds:def id=requirekeysguard-t3pc7v9m
func (r requireKeys) GuardSave(doc plugin.Document) error {
	for _, key := range r {
		if _, ok := doc.ValueAt(key); !ok {
			return fmt.Errorf("%s: %s is required", doc.FormatName(), key)
		}
	}
	return nil
}

func main() {
	dir, err := os.MkdirTemp("", "dotenv-write-a-plugin")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	err = run(os.Stdout, dir)
	// Cleanup is best-effort: the run's result matters more than a leftover
	// temp directory.
	_ = os.RemoveAll(dir)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// run installs the plugins and prints each step under a heading so the test
// can pin the whole output.
func run(w io.Writer, dir string) error {
	path := filepath.Join(dir, envFile)
	if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
		return err
	}
	v := &vault{secrets: map[string]string{"DB_PASSWORD": "hush"}}

	fmt.Fprintln(w, "== install and read")
	f, err := install(w, path, v)
	if err != nil {
		return err
	}
	fmt.Fprintln(w, "== guard")
	if err := guard(w, f); err != nil {
		return err
	}
	fmt.Fprintln(w, "== store down")
	storeDown(w, v)
	return nil
}

// install opens the file with both plugins and prints what Plugins detected:
// a missing capability there means a method does not match its interface.
// The secret reaches GetExpanded only; Get and Render keep the reference.
//
//ds:def id=install-w3tj8c5n
func install(w io.Writer, path string, v *vault) (*dotenv.File, error) {
	f, err := dotenv.Open(path, dotenv.WithPlugin(v, requireKeys{"DB_URL"}))
	if err != nil {
		return nil, err
	}
	for _, info := range f.Plugins() {
		fmt.Fprintln(w, "plugin", info)
	}
	url, _, err := f.GetExpanded("DB_URL")
	if err != nil {
		return nil, err
	}
	raw, _ := f.Get("DB_URL")
	fmt.Fprintln(w, "GetExpanded:", url)
	fmt.Fprintln(w, "Get:        ", raw)
	return f, nil
}

// guard shows both vetoes. A refused save writes nothing: the file on disk
// keeps its old bytes and no temp file is left. errors.Is reaches the
// plugin's own error through the wrapping.
//
//ds:def id=guard-k7fs2x4b
func guard(w io.Writer, f *dotenv.File) error {
	before, err := os.ReadFile(f.Path)
	if err != nil {
		return err
	}
	f.Set("DB_PASSWORD", "hush") // someone pastes the secret in
	err = f.Save()
	fmt.Fprintln(w, "leak refused:", errors.Is(err, errLeak))
	fmt.Fprintln(w, strings.Replace(err.Error(), f.Path, envFile, 1))

	f.Unset("DB_PASSWORD", true)
	f.Unset("DB_URL", true)
	err = f.Save()
	fmt.Fprintln(w, strings.Replace(err.Error(), f.Path, envFile, 1))

	after, err := os.ReadFile(f.Path)
	if err != nil {
		return err
	}
	entries, err := os.ReadDir(filepath.Dir(f.Path))
	if err != nil {
		return err
	}
	fmt.Fprintln(w, "file unchanged:", string(before) == string(after), "files in dir:", len(entries))
	return nil
}

// storeDown shows that a Lookuper error aborts the read even when the
// reference has a default, and names the plugin and the variable.
//
//ds:def id=storedown-p8gw3m6d
func storeDown(w io.Writer, v *vault) {
	v.down = true
	f := dotenv.Parse("PASS=${DB_PASSWORD:-}\n", dotenv.WithPlugin(v))
	_, _, err := f.GetExpanded("PASS")
	fmt.Fprintln(w, err)
	fmt.Fprintln(w, "unavailable:", errors.Is(err, plugin.ErrUnavailable))
}
