// Command errors shows every way package shell reports a failure, on its
// own with no file format: a non-zero exit (*shell.ExitError), a timeout and
// a context deadline (shell.ErrTimeout), a cancelled context, a missing shell
// (shell.ErrNoShell), stdin closed so a prompt cannot hang, and stdout kept
// out of every error. It needs a Unix shell.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/ubgo/textdata/shell"
)

// shortTimeout is long enough for the shell to start and much shorter than
// the sleep it interrupts.
const shortTimeout = 100 * time.Millisecond

// fakeSecret is what a failing command prints on stdout; the program checks
// that no error text contains it.
const fakeSecret = "not-a-real-password"

func main() {
	if err := run(os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// run executes every step of the guide in order and prints what each returns.
func run(w io.Writer) error {
	steps := []func(io.Writer) error{exitCode, timeouts, noShell, stdinClosed, noStdoutInErrors}
	for _, step := range steps {
		if err := step(w); err != nil {
			return err
		}
	}
	return nil
}

// exitCode: a non-zero exit is an *shell.ExitError carrying the code and the
// head of stderr (at most shell.MaxStderr bytes), where tools explain
// themselves.
//
//ds:def id=shell-exitcode-r5g2m9kw
func exitCode(w io.Writer) error {
	_, err := shell.Run(context.Background(), "echo 'entry not found' >&2; exit 11")
	var ee *shell.ExitError
	if !errors.As(err, &ee) {
		return fmt.Errorf("want *shell.ExitError, got %v", err)
	}
	fmt.Fprintf(w, "exit:     code=%d stderr=%q\n          %v\n", ee.Code, ee.Stderr, err)
	return nil
}

// timeouts: Runner.Timeout and a context deadline both end the command with
// shell.ErrTimeout; cancelling the context is context.Canceled instead.
//
//ds:def id=shell-timeouts-c8x4f1pn
func timeouts(w io.Writer) error {
	_, err := shell.Runner{Timeout: shortTimeout}.Run(context.Background(), "sleep 30")
	fmt.Fprintf(w, "timeout:  ErrTimeout=%t\n          %v\n", errors.Is(err, shell.ErrTimeout), err)

	ctx, cancel := context.WithTimeout(context.Background(), shortTimeout)
	defer cancel()
	_, err = shell.Run(ctx, "sleep 30")
	fmt.Fprintf(w, "deadline: ErrTimeout=%t DeadlineExceeded=%t\n", errors.Is(err, shell.ErrTimeout), errors.Is(err, context.DeadlineExceeded))

	ctx, stop := context.WithCancel(context.Background())
	time.AfterFunc(shortTimeout, stop)
	_, err = shell.Run(ctx, "sleep 30")
	fmt.Fprintf(w, "cancel:   Canceled=%t ErrTimeout=%t\n", errors.Is(err, context.Canceled), errors.Is(err, shell.ErrTimeout))
	return nil
}

// noShell: a shell that is not installed is shell.ErrNoShell, and nothing
// runs. The default shell is checked the same way.
//
//ds:def id=shell-noshell-v3k7d2qa
func noShell(w io.Writer) error {
	_, err := shell.Runner{Shell: []string{"no-such-shell", "-c"}}.Run(context.Background(), "echo hi")
	fmt.Fprintf(w, "no shell: ErrNoShell=%t\n          %v\n", errors.Is(err, shell.ErrNoShell), err)
	return nil
}

// stdinClosed: stdin is the null device, so a command that asks a question
// reads end-of-file at once instead of waiting for an answer nobody types.
//
//ds:def id=shell-stdinclosed-n9b6w4ty
func stdinClosed(w io.Writer) error {
	out, err := shell.Run(context.Background(), `if read -r answer; then echo "read: $answer"; else echo "stdin is empty"; fi`)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "stdin:    %s\n", out)
	return nil
}

// noStdoutInErrors: a command that prints a secret and then fails reports
// the command and the exit code, never what it printed.
//
//ds:def id=shell-nostdout-e2h8s5uj
func noStdoutInErrors(w io.Writer) error {
	r := shell.Runner{Env: []string{"FAKE_SECRET=" + fakeSecret}}
	_, err := r.Run(context.Background(), `printf '%s' "$FAKE_SECRET"; exit 1`)
	leaked := err != nil && strings.Contains(err.Error(), fakeSecret)
	fmt.Fprintf(w, "no leak:  %v (stdout in error: %t)\n", err, leaked)
	return nil
}
