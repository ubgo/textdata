// Command run uses package shell on its own, with no file format involved:
// shell.Run with the OS default shell, a Runner with Env, Dir and Timeout, a
// different shell (bash), and the output rule (only trailing CR/LF trimmed).
// It runs harmless commands only (echo, printf, pwd) and needs a Unix shell.
package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/ubgo/textdata/shell"
)

// workDirName is the directory the Runner example runs in, created inside a
// temp dir so the printed name is the same on every run.
const workDirName = "work"

// commandTimeout bounds every command the Runner example starts.
const commandTimeout = 5 * time.Second

func main() {
	if err := run(os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// run executes every step of the guide in order and prints what each returns.
func run(w io.Writer) error {
	steps := []func(io.Writer) error{runDefault, runWithOptions, runOtherShell, outputRule}
	for _, step := range steps {
		if err := step(w); err != nil {
			return err
		}
	}
	return nil
}

// runDefault runs one command with the OS default shell: sh -c here,
// PowerShell on Windows. Run returns stdout without the trailing newline.
//
//ds:def id=shell-rundefault-k4m8q2xd
func runDefault(w io.Writer) error {
	out, err := shell.Run(context.Background(), "echo hello from the shell")
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "run:     %q\n", out)
	fmt.Fprintf(w, "default: %q\n", shell.DefaultShell()) // [sh -c] here; PowerShell on Windows
	return nil
}

// runWithOptions sets a Runner's environment, working directory and
// timeout. Env replaces the whole environment (nil inherits it); Dir ""
// would be the current directory.
//
//ds:def id=shell-runwithoptions-p7w3n6vc
func runWithOptions(w io.Writer) error {
	tmp, err := os.MkdirTemp("", "shell-run")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	dir := filepath.Join(tmp, workDirName)
	if err := os.Mkdir(dir, 0o755); err != nil {
		return err
	}

	r := shell.Runner{
		Env:     []string{"GREETING=hi", "PATH=" + os.Getenv("PATH")},
		Dir:     dir,
		Timeout: commandTimeout,
	}
	out, err := r.Run(context.Background(), `printf '%s from %s' "$GREETING" "$(basename "$PWD")"`)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "runner:  %q\n", out)
	return nil
}

// runOtherShell swaps sh -c for bash -c. On Windows the same field selects
// cmd /C or a POSIX shell such as Git's bash.
//
//ds:def id=shell-runothershell-h2c9t5rb
func runOtherShell(w io.Writer) error {
	bash := shell.Runner{Shell: []string{"bash", "-c"}}
	out, err := bash.Run(context.Background(), `[[ -n $BASH_VERSION ]] && echo "bash it is"`)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "bash:    %q\n", out)
	return nil
}

// outputRule shows what is trimmed: every trailing CR and LF, nothing else.
// Leading and trailing spaces and inner newlines are part of the value.
//
//ds:def id=shell-outputrule-z6v1j8sy
func outputRule(w io.Writer) error {
	for _, cmd := range []string{
		`printf 'secret\r\n'`,
		`printf '  padded  \n\n'`,
		`printf 'line 1\nline 2\n'`,
	} {
		out, err := shell.Run(context.Background(), cmd)
		if err != nil {
			return err
		}
		fmt.Fprintf(w, "%-28s => %q\n", cmd, out)
	}
	return nil
}
