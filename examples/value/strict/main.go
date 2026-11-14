// Command strict expands values with value.WithStrict, the mode for a caller
// that must never receive a silently empty or unexpanded value: an unset
// name, a ${scheme:ref} no plugin claims, and a $(command) with commands off
// are each a typed error naming the reference and its offset, never a value.
// The forms that resolve, or whose operator says what unset means, read as
// they do without it.
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

// config is the variable source: what a config section or a map would hold.
// The token is made up.
var config = map[string]string{
	"API_TOKEN": "fake-token",
	"EMPTY":     "",
}

func main() {
	if err := run(os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// run shows the forms strict mode keeps, then the ones it refuses.
func run(w io.Writer) error {
	if err := strictResolves(w); err != nil {
		return err
	}
	return strictRefuses(w)
}

// lookup reads config.
func lookup(name string) (string, bool) {
	v, ok := config[name]
	return v, ok
}

// strictResolves: references that resolve, a name set to "", and the default
// operators all read exactly as without WithStrict.
//
//ds:def id=value-strictresolves-k4w9p2td
func strictResolves(w io.Writer) error {
	for _, in := range []string{
		"Bearer ${API_TOKEN}",
		"[${EMPTY}]",
		"${REGION:-eu-west-1}",
	} {
		out, err := value.Expand(context.Background(), in, value.WithStrict(), value.WithLookup(lookup))
		if err != nil {
			return err
		}
		fmt.Fprintf(w, "%-24s => %s\n", in, out)
	}
	return nil
}

// strictRefuses: each unresolved reference is an error that matches
// expand.ErrStrict and is one of three types; ${VAR:?msg} keeps its own
// *expand.RequiredError.
//
//ds:def id=value-strictrefuses-r7n3c5hm
func strictRefuses(w io.Writer) error {
	for _, in := range []string{
		"Bearer ${API_TOKN}",         // a typo: unset
		"${gopas:work/api}",          // a scheme no plugin claims
		"$(gopass show -o work/api)", // commands are off
		"${API_SECRET:?set it}",      // required: its own error
	} {
		_, err := value.Expand(context.Background(), in, value.WithStrict(), value.WithLookup(lookup))
		var (
			unset   *expand.UnsetError
			scheme  *expand.UnknownSchemeError
			cmdOff  *expand.CommandOffError
			require *expand.RequiredError
		)
		kind := "none"
		switch {
		case errors.As(err, &unset):
			kind = "unset " + unset.Name
		case errors.As(err, &scheme):
			kind = "unknown scheme " + scheme.Scheme
		case errors.As(err, &cmdOff):
			kind = "command off"
		case errors.As(err, &require):
			kind = "required " + require.Key
		}
		fmt.Fprintf(w, "%-28s strict=%-5t %s\n  %v\n", in, errors.Is(err, expand.ErrStrict), kind, err)
	}
	return nil
}
