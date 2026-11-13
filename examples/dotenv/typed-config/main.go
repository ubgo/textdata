// Command typed-config turns the string values of a .env file into a typed Go
// struct (int, bool, time.Duration, a slice) with the standard library alone,
// reporting every bad value at once. It also shows the small adapter that lets
// a struct-tag binder such as go-envconfig resolve keys from a *dotenv.File,
// without this program depending on that binder.
//
// Run it with `go run .`; it works in a fresh temp directory.
package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/ubgo/textdata/dotenv"
)

// Keys the binder reads. Named once so a typo is a compile error, not a
// silently missing setting.
const (
	keyPort  = "PORT"
	keyDebug = "DEBUG"
	keyHosts = "HOSTS"
	keyWait  = "WAIT"
	keyToken = "TOKEN"
)

// Defaults apply only when a key is absent, never when it is malformed.
const (
	defaultPort = 8080
	defaultWait = 30 * time.Second
	hostsSep    = ";"
)

// good and bad are the two files the run binds.
const (
	good = "DOMAIN=acme.io\nPORT=8443\nDEBUG=true\nHOSTS=api.${DOMAIN};www.${DOMAIN}\nTOKEN=t-123\n"
	bad  = "PORT=eighty\nWAIT=soon\n"
)

// Config is what the application actually wants.
//
//ds:def id=configtype-j7tq2m9w
type Config struct {
	Port  int
	Debug bool
	Hosts []string
	Wait  time.Duration
	Token string
}

func main() {
	dir, err := os.MkdirTemp("", "dotenv-typed-config")
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

// run binds the good file, then the bad one, then reads through the adapter.
func run(w io.Writer, dir string) error {
	for _, c := range []struct{ name, content string }{{".env", good}, {".env.bad", bad}} {
		path := filepath.Join(dir, c.name)
		if err := os.WriteFile(path, []byte(c.content), 0o600); err != nil {
			return err
		}
		fmt.Fprintln(w, "==", c.name)
		cfg, err := load(path)
		if err != nil {
			fmt.Fprintln(w, err)
			continue
		}
		fmt.Fprintf(w, "%+v\n", cfg)
	}
	fmt.Fprintln(w, "== adapter")
	return adapter(w)
}

// load reads the expanded values (references, defaults and plugins already
// applied) and binds them. Read fails first on an unsatisfied ${VAR:?msg}.
//
//ds:def id=load-c5hx8n3v
func load(path string) (Config, error) {
	m, err := dotenv.Read(path)
	if err != nil {
		return Config{}, err
	}
	return bind(m)
}

// bind converts each value with strconv and time, collecting every problem
// with errors.Join instead of stopping at the first one.
//
//ds:def id=bind-r2wk6f4p
func bind(m map[string]string) (Config, error) {
	cfg := Config{Port: defaultPort, Wait: defaultWait}
	var errs []error
	if v, ok := m[keyPort]; ok {
		n, err := strconv.Atoi(v)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %q is not an integer", keyPort, v))
		}
		cfg.Port = n
	}
	if v, ok := m[keyDebug]; ok {
		b, err := strconv.ParseBool(v)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %q is not a boolean", keyDebug, v))
		}
		cfg.Debug = b
	}
	if v, ok := m[keyHosts]; ok && v != "" {
		cfg.Hosts = strings.Split(v, hostsSep)
	}
	if v, ok := m[keyWait]; ok {
		d, err := time.ParseDuration(v)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %q is not a duration", keyWait, v))
		}
		cfg.Wait = d
	}
	cfg.Token = m[keyToken]
	if cfg.Token == "" {
		errs = append(errs, fmt.Errorf("%s is required", keyToken))
	}
	return cfg, errors.Join(errs...)
}

// fileLookuper adapts a *dotenv.File to the one-method lookuper interface
// struct-tag binders accept (go-envconfig's Lookup(key) (string, bool)). The
// interface has no error return, so an expansion error has to be dropped:
// call ExpandedMap first if a ${VAR:?msg} must fail loudly.
//
//ds:def id=filelookuper-m9dv3k7s
type fileLookuper struct{ f *dotenv.File }

//ds:def id=filelookuperlookup-q6mb3r8z
func (l fileLookuper) Lookup(key string) (string, bool) {
	v, ok, err := l.f.GetExpanded(key)
	if err != nil {
		return "", false
	}
	return v, ok
}

// adapter shows the trap in the per-key adapter: a required reference that
// fails reads as "not set" instead of as an error.
func adapter(w io.Writer) error {
	f := dotenv.Parse(good + "AUTH=${MISSING:?set MISSING}\n")
	l := fileLookuper{f}
	hosts, ok := l.Lookup(keyHosts)
	fmt.Fprintln(w, "HOSTS:", hosts, ok)
	auth, ok := l.Lookup("AUTH")
	fmt.Fprintf(w, "AUTH: %q %v\n", auth, ok)
	_, err := f.ExpandedMap()
	fmt.Fprintln(w, "ExpandedMap:", err)
	return nil
}
