// Command env expands single values with value.Expand, no file format
// involved: the process environment, a map as a second source, defaults,
// required values, nesting, $$, and the errors a caller branches on. It sets
// its own demo variables, so the output is the same on every machine.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/ubgo/textdata/expand"
	"github.com/ubgo/textdata/value"
)

// The demo variables run sets in its own process environment, and the ones it
// clears so an unset name reads as unset on every machine.
const (
	envUser     = "DEMO_USER"
	envHost     = "DEMO_HOST"
	envPort     = "DEMO_PORT"
	envRegion   = "DEMO_REGION"
	envAPIToken = "DEMO_API_TOKEN"
)

func main() {
	if err := run(os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// run sets the demo environment, then executes every step of the guide in
// order and prints what each returns.
func run(w io.Writer) error {
	for k, v := range map[string]string{envUser: "ada", envHost: "db.internal"} {
		if err := os.Setenv(k, v); err != nil {
			return err
		}
	}
	for _, k := range []string{envPort, envRegion, envAPIToken} {
		if err := os.Unsetenv(k); err != nil {
			return err
		}
	}
	steps := []func(io.Writer) error{fromEnv, defaultsAndRequired, layeredSources, requiredError}
	for _, step := range steps {
		if err := step(w); err != nil {
			return err
		}
	}
	return nil
}

// fromEnv expands a value from the process environment. Without WithEnv the
// environment is never read and every name is unset.
//
//ds:def id=value-fromenv-t3q8w5kd
func fromEnv(w io.Writer) error {
	ctx := context.Background()
	in := "postgres://${DEMO_USER}@$DEMO_HOST/app"
	with, err := value.Expand(ctx, in, value.WithEnv())
	if err != nil {
		return err
	}
	without, err := value.Expand(ctx, in)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "with env:    %s\nwithout env: %s\n", with, without)
	return nil
}

// defaultsAndRequired uses the Compose operators: a default when unset or
// empty, a nested default, $$ for a literal dollar.
//
//ds:def id=value-defaults-m6x2c9vb
func defaultsAndRequired(w io.Writer) error {
	for _, in := range []string{
		"${DEMO_PORT:-5432}",
		"${DEMO_REGION:-${DEMO_HOST}}",
		"${DEMO_USER:+user is set}",
		"cost: $$5",
		"${DEMO_USER:?DEMO_USER must be set}",
	} {
		out, err := value.Expand(context.Background(), in, value.WithEnv())
		if err != nil {
			return err
		}
		fmt.Fprintf(w, "%-36s => %s\n", in, out)
	}
	return nil
}

// layeredSources asks a map first and the environment second; the first
// source that has a name wins. A map value is expanded in turn.
//
//ds:def id=value-layered-j9n4r7fs
func layeredSources(w io.Writer) error {
	overrides := map[string]string{
		"DEMO_HOST": "localhost",
		"DSN":       "postgres://${DEMO_USER}@${DEMO_HOST}/app",
	}
	lookup := func(name string) (string, bool) {
		v, ok := overrides[name]
		return v, ok
	}
	out, err := value.Expand(context.Background(), "$DSN", value.WithLookup(lookup), value.WithEnv())
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "layered:     %s\n", out)
	return nil
}

// requiredError: ${VAR:?msg} on an unset name is an *expand.RequiredError,
// the same type dotenv's engine and jsonc return.
//
//ds:def id=value-requirederror-b5y1k8hp
func requiredError(w io.Writer) error {
	_, err := value.Expand(context.Background(), "${DEMO_API_TOKEN:?set DEMO_API_TOKEN first}", value.WithEnv())
	var re *expand.RequiredError
	if !errors.As(err, &re) {
		return fmt.Errorf("want *expand.RequiredError, got %v", err)
	}
	fmt.Fprintf(w, "required:    key=%s\n             %v\n", re.Key, err)
	return nil
}
