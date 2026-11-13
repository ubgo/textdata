// Command run-child drives `dotenvctl run`: a child process gets the file's
// values (expanded, and winning over the inherited environment) while the
// program's own environment stays untouched; the child's exit code passes
// through; a missing required variable stops the run before the child starts.
package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"

	"github.com/ubgo/textdata/dotenv/cli/dotenvcmd"
)

// greetingKey is set both in this process's environment and in the file, to
// show which one the child sees.
const greetingKey = "GREETING"

// shellValue is what this process's environment says before and after.
const shellValue = "from the shell"

// Names and contents of the two files the demo creates.
const (
	appFile      = "app.env"
	appEnv       = "GREETING=from the file\nHOST=db.internal\nURL=postgres://${HOST}/app\n"
	requiredFile = "required.env"
	requiredEnv  = "NEED=${MISSING:?set MISSING first}\n"
)

func main() {
	dir, err := os.MkdirTemp("", "run-child-")
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

// run executes every step of the guide in dir.
// The caller owns dir (main makes a temp one, the test passes t.TempDir()),
// so nothing is written outside it.
func run(w io.Writer, dir string) error {
	if runtime.GOOS == "windows" {
		return errors.New("run-child: the child commands use /bin/sh")
	}
	app := filepath.Join(dir, appFile)
	required := filepath.Join(dir, requiredFile)
	for path, body := range map[string]string{app: appEnv, required: requiredEnv} {
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			return err
		}
	}

	restore, err := setShellValue()
	if err != nil {
		return err
	}
	defer restore()

	fmt.Fprintln(w, "exit:", runChild(w, app, "sh", "-c", `echo "child: $GREETING, $URL"`))
	fmt.Fprintf(w, "parent still has: %s\n", os.Getenv(greetingKey))
	fmt.Fprintln(w, "exit:", runChild(w, app, "sh", "-c", "exit 3"))
	fmt.Fprintln(w, "exit:", runChild(w, required, "sh", "-c", "echo never printed"))
	return nil
}

// setShellValue puts GREETING in this process's environment, as a shell
// would, and returns the undo. The undo drops its errors: putting back a
// variable that was just set under the same name cannot fail.
func setShellValue() (restore func(), err error) {
	old, had := os.LookupEnv(greetingKey)
	if err := os.Setenv(greetingKey, shellValue); err != nil {
		return nil, err
	}
	return func() {
		if had {
			_ = os.Setenv(greetingKey, old)
			return
		}
		_ = os.Unsetenv(greetingKey)
	}, nil
}

// runChild is `dotenvctl -f FILE run -- COMMAND...`. The "--" ends dotenvctl's
// own flags, so flags in COMMAND reach the child. The child's stdout and
// stderr go to w; its exit code becomes Execute's return value verbatim.
//
//ds:def id=runchild-7dpqrucc
func runChild(w io.Writer, envFile string, command ...string) int {
	args := append([]string{"-f", envFile, "run", "--"}, command...)
	return dotenvcmd.Execute(args, w, w)
}
