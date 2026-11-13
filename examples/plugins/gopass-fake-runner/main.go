// Command gopass-fake-runner installs the gopass value plugin in a dotenv file
// and a JSONC document at once, with a fake runner standing in for the gopass
// binary, so it runs anywhere without a password store or gpg.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"

	"github.com/ubgo/textdata/dotenv"
	"github.com/ubgo/textdata/jsonc"
	"github.com/ubgo/textdata/plugin"
	gopassplugin "github.com/ubgo/textdata/plugins/gopass"
)

// exitNotFound is the exit status real gopass uses for a missing entry; the
// fake returns it so the plugin classifies the miss exactly as in production.
const exitNotFound = 11

// envSource references the store twice: once through the gopass scheme and
// once through a plain name the file does not define (DB_USER).
const envSource = "DB_PASSWORD=${gopass:personal/app/db}\nDB_USER=${DB_USER}\nMISSING=${gopass:personal/app/nope}\n"

// jsoncSource is the same secret referenced from a JSONC document.
const jsoncSource = `{
  // the plugin reads the store; the file keeps the reference
  "db": {"password": "${gopass:personal/app/db}"}
}
`

// fakeStore is what the fake runner serves, keyed by store path.
var fakeStore = map[string]string{
	"personal/app/db":      "hush-from-gopass",
	"personal/app/db-user": "app",
}

// exitError carries an exit status the way *exec.ExitError does, which is all
// the plugin inspects (an ExitCode() int method).
type exitError struct{ code int }

func (e exitError) Error() string { return fmt.Sprintf("exit status %d", e.code) }
func (e exitError) ExitCode() int { return e.code }

// newFakeRunner returns a gopassplugin.Runner that answers `gopass show -o ref`
// from fakeStore and counts calls, so the cache can be observed. A test or a
// demo injects this instead of letting the plugin run a real binary.
//
//ds:def id=newfakerunner-x8sc24ut
func newFakeRunner(calls *int) gopassplugin.Runner {
	return func(_ context.Context, _ string, args []string) ([]byte, []byte, error) {
		*calls++
		if len(args) != 3 || args[0] != "show" || args[1] != "-o" {
			return nil, []byte("unexpected arguments"), exec.ErrNotFound
		}
		v, ok := fakeStore[args[2]]
		if !ok {
			return nil, []byte("entry is not in the password store"), exitError{code: exitNotFound}
		}
		// gopass ends its output with a newline; the plugin trims exactly one.
		return []byte(v + "\n"), nil, nil
	}
}

// newPlugin builds one gopass plugin: the Resolver for ${gopass:...} is always
// on, and WithMapping turns on the Lookuper for exactly the names listed.
//
//ds:def id=gopass-newplugin-q3jcvte8
func newPlugin(calls *int) *gopassplugin.Plugin {
	return gopassplugin.New(
		gopassplugin.WithRunner(newFakeRunner(calls)),
		gopassplugin.WithMapping(map[string]string{"DB_USER": "personal/app/db-user"}),
	)
}

// readDotenv installs the plugin in a dotenv file. Get keeps the reference;
// only GetExpanded asks the plugin.
//
//ds:def id=gopass-readdotenv-2xz5yprn
func readDotenv(w io.Writer, p *gopassplugin.Plugin) error {
	f := dotenv.Parse(envSource, dotenv.WithPlugin(p))
	raw, _ := f.Get("DB_PASSWORD")
	pass, _, err := f.GetExpanded("DB_PASSWORD")
	if err != nil {
		return err
	}
	user, _, err := f.GetExpanded("DB_USER")
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "dotenv raw:      %s\n", raw)
	fmt.Fprintf(w, "dotenv expanded: %s (user %s)\n", pass, user)
	// A missing entry is an error, never an empty value.
	_, _, err = f.GetExpanded("MISSING")
	fmt.Fprintf(w, "missing entry:   not found=%t\n", errors.Is(err, plugin.ErrNotFound))
	return nil
}

// readJSONC installs the same plugin instance in a JSONC document: the value
// was cached by the dotenv read, so the runner is not called again.
//
//ds:def id=gopass-readjsonc-egyent23
func readJSONC(w io.Writer, p *gopassplugin.Plugin) error {
	doc, err := jsonc.Parse([]byte(jsoncSource), jsonc.WithPlugin(p))
	if err != nil {
		return err
	}
	v, err := doc.GetExpanded("db.password")
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "jsonc expanded:  %v\n", v)
	fmt.Fprintf(w, "jsonc unchanged: %t\n", string(doc.Render()) == jsoncSource)
	return nil
}

// run reads both documents through one plugin and reports how many times the
// fake gopass ran.
func run(w io.Writer) error {
	calls := 0
	p := newPlugin(&calls)
	if err := readDotenv(w, p); err != nil {
		return err
	}
	if err := readJSONC(w, p); err != nil {
		return err
	}
	fmt.Fprintf(w, "gopass calls:    %d\n", calls)
	return nil
}

func main() {
	if err := run(os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
