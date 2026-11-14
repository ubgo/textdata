//go:build unix

package shell

import (
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// These tests run real commands through sh -c, so they are Unix-only; the
// Windows branch is covered by injection in shell_test.go and by
// shell_windows_test.go on a Windows machine.

func run(t *testing.T, r Runner, cmd string) (string, error) {
	t.Helper()
	return r.Run(context.Background(), cmd)
}

func TestEcho(t *testing.T) {
	out, err := Run(context.Background(), "echo hello")
	if err != nil || out != "hello" {
		t.Fatalf("Run = %q, %v; want %q", out, err, "hello")
	}
}

// TestTrimsOnlyTrailingNewlines: every trailing CR and LF goes; spaces at
// either end and inner newlines stay.
func TestTrimsOnlyTrailingNewlines(t *testing.T) {
	tests := map[string]string{
		`printf 'a\r\n'`:          "a",
		`printf 'a\n\n\r\n'`:      "a",
		`printf '  a b  \n'`:      "  a b  ",
		`printf 'line1\nline2\n'`: "line1\nline2",
		`printf '\tx\t'`:          "\tx\t",
		`true`:                    "",
	}
	for cmd, want := range tests {
		out, err := run(t, Runner{}, cmd)
		if err != nil || out != want {
			t.Errorf("%s = %q, %v; want %q", cmd, out, err, want)
		}
	}
}

// TestExitError: a non-zero exit is an *ExitError with the code and the head
// of stderr, naming the command.
func TestExitError(t *testing.T) {
	_, err := run(t, Runner{}, "echo 'entry not found' >&2; exit 3")
	var ee *ExitError
	if !errors.As(err, &ee) {
		t.Fatalf("err = %T %v, want *ExitError", err, err)
	}
	if ee.Code != 3 || ee.Stderr != "entry not found" {
		t.Errorf("ExitError = %+v", ee)
	}
	want := `shell: command "echo 'entry not found' >&2; exit 3" exited with code 3: entry not found`
	if err.Error() != want {
		t.Errorf("Error() = %q\nwant      %q", err.Error(), want)
	}
	_, err = run(t, Runner{}, "exit 1")
	if err == nil || err.Error() != `shell: command "exit 1" exited with code 1` {
		t.Errorf("empty stderr: Error() = %v", err)
	}
}

// TestErrorNeverContainsStdout: what a failing command printed on stdout (a
// secret, possibly) is in no error text. The command prints SECRET without
// spelling it, since the command text itself is named in the error.
func TestErrorNeverContainsStdout(t *testing.T) {
	for _, r := range []Runner{{}, {Timeout: 200 * time.Millisecond}} {
		_, err := run(t, r, "printf %s%s SEC RET; sleep 1; exit 4")
		if err == nil {
			t.Fatal("want an error")
		}
		if strings.Contains(err.Error(), "SECRET") {
			t.Errorf("error leaks stdout: %v", err)
		}
	}
}

// TestStderrTruncated: a flood on stderr is cut at MaxStderr bytes.
func TestStderrTruncated(t *testing.T) {
	_, err := run(t, Runner{}, `i=0; while [ $i -lt 300 ]; do printf xxxxxxxxxx >&2; i=$((i+1)); done; exit 1`)
	var ee *ExitError
	if !errors.As(err, &ee) {
		t.Fatalf("err = %v", err)
	}
	if want := strings.Repeat("x", MaxStderr) + truncMark; ee.Stderr != want {
		t.Errorf("Stderr has %d bytes, want %d", len(ee.Stderr), len(want))
	}
}

// TestTimeout: Runner.Timeout stops a sleeping command quickly, even when the
// shell forked a child that holds stdout open, and the error is ErrTimeout
// and context.DeadlineExceeded.
func TestTimeout(t *testing.T) {
	start := time.Now()
	_, err := run(t, Runner{Timeout: 100 * time.Millisecond}, "sleep 30; true")
	if !errors.Is(err, ErrTimeout) || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want ErrTimeout", err)
	}
	if d := time.Since(start); d > waitDelay+5*time.Second {
		t.Errorf("took %s", d)
	}
	if !strings.Contains(err.Error(), "sleep 30") {
		t.Errorf("error should name the command: %v", err)
	}
}

// TestContextDeadlineAndCancel: a context deadline is ErrTimeout too; a
// cancelled context is context.Canceled, not ErrTimeout.
func TestContextDeadlineAndCancel(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if _, err := Run(ctx, "sleep 30"); !errors.Is(err, ErrTimeout) {
		t.Errorf("deadline: err = %v, want ErrTimeout", err)
	}
	ctx, cancel = context.WithCancel(context.Background())
	time.AfterFunc(100*time.Millisecond, cancel)
	_, err := Run(ctx, "sleep 30")
	if !errors.Is(err, context.Canceled) || errors.Is(err, ErrTimeout) {
		t.Errorf("cancel: err = %v, want context.Canceled only", err)
	}
}

// TestStdinClosed: a command that reads stdin gets EOF at once instead of
// waiting for input.
func TestStdinClosed(t *testing.T) {
	out, err := run(t, Runner{Timeout: 10 * time.Second}, `if read line; then echo "got:$line"; else echo eof; fi`)
	if err != nil || out != "eof" {
		t.Fatalf("Run = %q, %v; want eof", out, err)
	}
}

// TestShellOverride: Runner.Shell replaces sh -c.
func TestShellOverride(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not installed")
	}
	out, err := run(t, Runner{Shell: []string{"bash", "-c"}}, `[[ -n $BASH_VERSION ]] && echo bash`)
	if err != nil || out != "bash" {
		t.Fatalf("Run = %q, %v", out, err)
	}
}

// TestNoShell: a shell that does not exist is ErrNoShell and runs nothing.
func TestNoShell(t *testing.T) {
	_, err := run(t, Runner{Shell: []string{"/nonexistent/textdata-sh", "-c"}}, "echo hi")
	if !errors.Is(err, ErrNoShell) {
		t.Fatalf("err = %v, want ErrNoShell", err)
	}
}

// TestEnvAndDir: Env replaces the environment and Dir sets the working
// directory.
func TestEnvAndDir(t *testing.T) {
	t.Setenv("TEXTDATA_SHELL_INHERITED", "yes")
	out, err := run(t, Runner{}, `printf %s "$TEXTDATA_SHELL_INHERITED"`)
	if err != nil || out != "yes" {
		t.Errorf("inherited env: %q, %v", out, err)
	}
	out, err = run(t, Runner{Env: []string{"ONLY=this"}}, `printf %s "$ONLY|$TEXTDATA_SHELL_INHERITED"`)
	if err != nil || out != "this|" {
		t.Errorf("Env: %q, %v", out, err)
	}
	dir := t.TempDir()
	out, err = run(t, Runner{Dir: dir}, "pwd -P")
	want, _ := filepath.EvalSymlinks(dir)
	if err != nil || out != want {
		t.Errorf("Dir: %q, %v; want %q", out, err, want)
	}
}

// TestRunCommand: the plugin.CommandRunner method uses the Runner's options.
func TestRunCommand(t *testing.T) {
	out, err := Runner{Env: []string{"X=1"}}.RunCommand(`printf %s "$X"`)
	if err != nil || out != "1" {
		t.Fatalf("RunCommand = %q, %v", out, err)
	}
	if _, err := (Runner{Timeout: 50 * time.Millisecond}).RunCommand("sleep 30"); !errors.Is(err, ErrTimeout) {
		t.Errorf("RunCommand timeout: %v", err)
	}
}
