// Command commands shows $(command) in value.Expand: off unless asked for,
// on with WithCommands (shell.Run, bounded by the context), a shell.Runner
// with a timeout, an allow-listed runner through WithRunner, and the
// *expand.CommandError a failure returns. The commands are harmless (echo,
// printf, sleep); it needs a Unix shell.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/ubgo/textdata/expand"
	"github.com/ubgo/textdata/shell"
	"github.com/ubgo/textdata/value"
)

// shortTimeout interrupts the demo's sleep long before it ends.
const shortTimeout = 100 * time.Millisecond

func main() {
	if err := run(os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// run executes every step of the guide in order and prints what each returns.
func run(w io.Writer) error {
	steps := []func(io.Writer) error{offByDefault, commandsOn, boundedCommands, allowListed}
	for _, step := range steps {
		if err := step(w); err != nil {
			return err
		}
	}
	return nil
}

// offByDefault: without WithCommands or WithRunner, $(…) is kept as written
// and nothing runs, whatever other options are given.
//
//ds:def id=value-offbydefault-w8d3h6zq
func offByDefault(w io.Writer) error {
	out, err := value.Expand(context.Background(), "token=$(echo never-runs)", value.WithEnv())
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "off:      %s\n", out)
	return nil
}

// commandsOn runs $(…) through the OS shell. The command's own references
// expand first, and the trailing newline is trimmed.
//
//ds:def id=value-commandson-q2v7m4xe
func commandsOn(w io.Writer) error {
	who := func(name string) (string, bool) { return "ada", name == "WHO" }
	out, err := value.Expand(context.Background(), "hello $(printf '%s\\n' $WHO)!", value.WithLookup(who), value.WithCommands())
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "on:       %s\n", out)

	_, err = value.Expand(context.Background(), "$(echo 'no such entry' >&2; exit 2)", value.WithCommands())
	var ce *expand.CommandError
	var ee *shell.ExitError
	if !errors.As(err, &ce) || !errors.As(err, &ee) {
		return fmt.Errorf("want *expand.CommandError wrapping *shell.ExitError, got %v", err)
	}
	fmt.Fprintf(w, "failed:   code=%d\n          %v\n", ee.Code, err)
	return nil
}

// boundedCommands: the context passed to Expand bounds the command, and a
// shell.Runner given to WithRunner adds its own Timeout (or another shell).
//
//ds:def id=value-bounded-f6k9p2ns
func boundedCommands(w io.Writer) error {
	ctx, cancel := context.WithTimeout(context.Background(), shortTimeout)
	defer cancel()
	_, err := value.Expand(ctx, "$(sleep 30)", value.WithCommands())
	fmt.Fprintf(w, "deadline: ErrTimeout=%t\n", errors.Is(err, shell.ErrTimeout))

	r := shell.Runner{Timeout: shortTimeout}
	_, err = value.Expand(context.Background(), "$(sleep 30)", value.WithRunner(r))
	fmt.Fprintf(w, "runner:   ErrTimeout=%t\n", errors.Is(err, shell.ErrTimeout))
	return nil
}

// errNotAllowed is what the allow-listed runner returns for any other command.
var errNotAllowed = errors.New("command not allowed")

// allowList is a CommandRunner that answers known commands from a table and
// refuses the rest: commands are on, but nothing outside the list runs.
type allowList map[string]string

// RunCommand implements plugin.CommandRunner.
func (a allowList) RunCommand(cmd string) (string, error) {
	if v, ok := a[cmd]; ok {
		return v, nil
	}
	return "", errNotAllowed
}

// allowListed hands $(…) to the allow-listed runner instead of a shell.
//
//ds:def id=value-allowlist-c4t8j1wr
func allowListed(w io.Writer) error {
	runner := allowList{"gopass show -o work/api": "fake-api-token"}
	out, err := value.Expand(context.Background(), "$(gopass show -o work/api)", value.WithRunner(runner))
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "allowed:  %s\n", out)
	_, err = value.Expand(context.Background(), "$(rm -rf ~)", value.WithRunner(runner))
	fmt.Fprintf(w, "refused:  %v\n", err)
	return nil
}
