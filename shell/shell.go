// Package shell runs one command line through the operating system's shell
// and returns what it printed on stdout. It is the runner behind $(command)
// substitution in every textdata format, and works on its own for any program
// that needs the same behaviour.
//
// Why it exists: dotenv and jsonc each carried a private `sh -c` copy with no
// timeout and no Windows support. One runner in the base module gives every
// format (and every caller) the same rules:
//
//   - The shell is chosen per OS: `sh -c` on Unix-likes; on Windows `pwsh`
//     when it is on PATH, else `powershell.exe`, both with
//     `-NoProfile -NonInteractive -Command`. Runner.Shell overrides it.
//   - Stdin is the null device, so a command can never stop to ask a question
//     nobody sees.
//   - The context, plus Runner.Timeout, bounds the command; running out of
//     time is ErrTimeout.
//   - A failure is an error that names the command and the reason. It never
//     contains stdout, which may be a secret; stderr is included only when
//     non-empty and only its first MaxStderr bytes.
//   - Only trailing CR and LF are removed from stdout. Leading and inner
//     spaces are kept: a value may legitimately start or end with spaces.
//
// Stdlib only, like the rest of the base module.
package shell

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// MaxStderr is how many bytes of a failing command's stderr an error carries.
//
// Tools print their own reason on stderr ("gopass: entry not found"), which is
// what a person needs to fix the problem, but a runaway command can print
// megabytes there; the error keeps the head and marks the cut.
const MaxStderr = 512

// waitDelay bounds how long Run waits for a command's output pipes to close
// after the shell exits or is killed.
//
// Why: killing the shell on timeout does not kill a grandchild it started
// (`sh -c "sleep 60"` may fork sleep), and that grandchild keeps stdout open.
// Without a bound, Wait would block on the pipe until the grandchild ends,
// which defeats the timeout. Process groups would kill it too, but a command
// in its own group loses the terminal (a pinentry prompt on /dev/tty would
// stop with SIGTTIN), which would change how `sh -c` behaves today.
const waitDelay = 500 * time.Millisecond

// truncMark is appended to stderr cut at MaxStderr.
const truncMark = "…"

// goosWindows is the one runtime.GOOS whose default shell is not `sh -c`.
const goosWindows = "windows"

// The default shells, as argv prefixes; the command line is appended as the
// last argument.
var (
	// unixShell is used on every OS except Windows.
	unixShell = []string{"sh", "-c"}
	// pwshShell is PowerShell 7+, preferred on Windows when on PATH.
	pwshShell = []string{"pwsh", "-NoProfile", "-NonInteractive", "-Command"}
	// windowsPowerShell ships with every Windows since 7; the fallback.
	windowsPowerShell = []string{"powershell.exe", "-NoProfile", "-NonInteractive", "-Command"}
)

// ErrTimeout reports a command stopped because Runner.Timeout elapsed or the
// context's deadline passed. Match it with errors.Is; the error Run returns
// also matches context.DeadlineExceeded.
var ErrTimeout = errors.New("shell: command timed out")

// ErrNoShell reports that the shell program (the default for this OS, or
// Runner.Shell[0]) could not be found, so no command ran. Match it with
// errors.Is; the error Run returns also wraps the PATH lookup error.
var ErrNoShell = errors.New("shell: no shell found")

// ExitError reports a command that ran and exited with a non-zero code.
//
// Its text names the command, the code and the head of stderr; never stdout.
type ExitError struct {
	// Cmd is the command line as passed to Run.
	Cmd string
	// Code is the exit code; -1 when the process was ended by a signal.
	Code int
	// Stderr is the command's stderr with trailing whitespace removed, cut at
	// MaxStderr bytes (with "…" appended when cut). Empty when it printed
	// nothing there.
	Stderr string
}

func (e *ExitError) Error() string {
	s := fmt.Sprintf("shell: command %q exited with code %d", e.Cmd, e.Code)
	if e.Stderr != "" {
		s += ": " + e.Stderr
	}
	return s
}

// ds:def id=runner-vvzpt8w5 owner=@maintainers stability=stable

// Runner holds the options for running commands. The zero Runner is ready to
// use and is what Run uses.
type Runner struct {
	// Shell is the argv prefix the command line is appended to, for example
	// []string{"bash", "-c"} or []string{"cmd", "/C"}. Empty selects the OS
	// default (see the package documentation).
	Shell []string
	// Timeout bounds each command; 0 leaves only the context to bound it.
	Timeout time.Duration
	// Env is the command's environment as KEY=value pairs; nil inherits the
	// process environment, an empty non-nil slice gives an empty one.
	Env []string
	// Dir is the working directory; "" is the current directory.
	Dir string

	// lookPath and goos are seams for tests: they let the Windows branch of
	// the shell choice run on any OS. nil and "" mean exec.LookPath and
	// runtime.GOOS.
	lookPath func(string) (string, error)
	goos     string
}

// Run executes cmd with the zero Runner: the OS default shell, the process
// environment and the current directory, bounded only by ctx.
func Run(ctx context.Context, cmd string) (string, error) {
	return Runner{}.Run(ctx, cmd)
}

// Run executes cmd through the shell and returns its stdout with trailing CR
// and LF removed.
//
// Errors: ErrNoShell (wrapped) when the shell is missing; ErrTimeout (wrapped,
// together with context.DeadlineExceeded) when Timeout or ctx's deadline ends
// it; the context's error (wrapped) when ctx is cancelled; *ExitError for a
// non-zero exit. No error text contains stdout.
func (r Runner) Run(ctx context.Context, cmd string) (string, error) {
	argv, err := r.argv()
	if err != nil {
		return "", &cmdError{msg: fmt.Sprintf("shell: command %q: %v", cmd, err), errs: []error{ErrNoShell, err}}
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if r.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, r.Timeout)
		defer cancel()
	}

	c := exec.CommandContext(ctx, argv[0], append(argv[1:len(argv):len(argv)], cmd)...)
	c.Env = r.Env
	c.Dir = r.Dir
	// A nil Stdin is os.DevNull (os/exec documents it): reads see EOF at
	// once, so a command that prompts fails instead of hanging. Set
	// explicitly so nobody "fixes" it to os.Stdin.
	c.Stdin = nil
	var stdout bytes.Buffer
	stderr := &headWriter{max: MaxStderr}
	c.Stdout = &stdout
	c.Stderr = stderr
	c.WaitDelay = waitDelay

	runErr := c.Run()
	if ctxErr := ctx.Err(); ctxErr != nil {
		if errors.Is(ctxErr, context.DeadlineExceeded) {
			return "", &cmdError{msg: fmt.Sprintf("shell: command %q timed out", cmd), errs: []error{ErrTimeout, ctxErr}}
		}
		return "", &cmdError{msg: fmt.Sprintf("shell: command %q: %v", cmd, ctxErr), errs: []error{ctxErr}}
	}
	if runErr != nil && !errors.Is(runErr, exec.ErrWaitDelay) {
		// ErrWaitDelay alone means the command exited 0 but something it
		// started kept stdout open; what it printed is complete, so it is a
		// success.
		var ee *exec.ExitError
		if errors.As(runErr, &ee) {
			return "", &ExitError{Cmd: cmd, Code: ee.ExitCode(), Stderr: stderr.String()}
		}
		return "", fmt.Errorf("shell: command %q: %w", cmd, runErr)
	}
	return strings.TrimRight(stdout.String(), "\r\n"), nil
}

// RunCommand runs cmd with context.Background(), bounded by r.Timeout. It
// makes a Runner satisfy plugin.CommandRunner, so it can be handed to any API
// that takes one.
func (r Runner) RunCommand(cmd string) (string, error) {
	return r.Run(context.Background(), cmd)
}

// argv returns the shell's argv prefix with argv[0] resolved on PATH, or the
// lookup error naming the shell (Run adds ErrNoShell).
//
// Resolving first, rather than letting exec fail at start, is what makes a
// missing shell a distinct error: a start failure can also be a bad Dir.
func (r Runner) argv() ([]string, error) {
	look := r.lookPath
	if look == nil {
		look = exec.LookPath
	}
	sh := r.Shell
	if len(sh) == 0 {
		goos := r.goos
		if goos == "" {
			goos = runtime.GOOS
		}
		sh = defaultShell(goos, look)
	}
	path, err := look(sh[0])
	if err != nil {
		return nil, fmt.Errorf("no shell found: %s: %w", sh[0], err)
	}
	out := make([]string, len(sh))
	copy(out, sh)
	out[0] = path
	return out, nil
}

// defaultShell picks the shell for goos: PowerShell on Windows (pwsh when
// look finds it, else Windows PowerShell), `sh -c` everywhere else.
func defaultShell(goos string, look func(string) (string, error)) []string {
	if goos != goosWindows {
		return unixShell
	}
	if _, err := look(pwshShell[0]); err == nil {
		return pwshShell
	}
	return windowsPowerShell
}

// ds:def id=defaultshell-rz6bhgx2 owner=@maintainers stability=stable

// DefaultShell returns the argv prefix Run uses on this OS, for example
// [sh -c] or [pwsh -NoProfile -NonInteractive -Command]. It reports the
// choice only; it does not check that the program exists.
func DefaultShell() []string {
	sh := defaultShell(runtime.GOOS, exec.LookPath)
	out := make([]string, len(sh))
	copy(out, sh)
	return out
}

// cmdError is an error with its own text that matches several targets with
// errors.Is: a timeout is both ErrTimeout and context.DeadlineExceeded, a
// missing shell both ErrNoShell and the PATH lookup error. Its own text keeps
// the message readable ("shell: command "x" timed out") instead of a chain of
// repeated prefixes.
type cmdError struct {
	msg  string
	errs []error
}

func (e *cmdError) Error() string   { return e.msg }
func (e *cmdError) Unwrap() []error { return e.errs }

// headWriter keeps the first max bytes written to it and counts the rest, so
// a command printing megabytes to stderr costs MaxStderr bytes of memory.
type headWriter struct {
	max  int
	buf  []byte
	more bool
}

func (w *headWriter) Write(p []byte) (int, error) {
	if room := w.max - len(w.buf); room > 0 {
		if len(p) > room {
			w.buf = append(w.buf, p[:room]...)
			w.more = true
		} else {
			w.buf = append(w.buf, p...)
		}
	} else if len(p) > 0 {
		w.more = true
	}
	return len(p), nil
}

// String is the kept head with trailing whitespace removed, and "…" when
// bytes were dropped.
func (w *headWriter) String() string {
	// The cut may split a multi-byte character; drop the broken tail.
	s := strings.ToValidUTF8(strings.TrimRight(string(w.buf), " \t\r\n"), "")
	if w.more {
		s += truncMark
	}
	return s
}
