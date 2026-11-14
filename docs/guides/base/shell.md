# Run a command: `shell`

Package `github.com/ubgo/textdata/shell` runs one command line through the operating system's shell and returns what it printed on stdout. It is the runner behind `$(command)` in dotenv and jsonc, and it works on its own: no file format, no plugin, no setup. Reach for it when your program has to run a short command and use its output, for example a `token_cmd` setting such as `gopass show -o work/api`, and you want a timeout, no hanging prompt, Windows support and errors that never print the secret, without writing that runner yourself.

## Before you start

```sh
go get github.com/ubgo/textdata@latest
```

`shell` is in the base module, which has no dependencies outside the standard library. The runnable programs are [`examples/shell/run`](../../../examples/shell/run) and [`examples/shell/errors`](../../../examples/shell/errors); both need a Unix shell. To expand a whole value such as `Bearer $(gopass show -o work/api)` rather than run one command, use [`value.Expand`](value.md), which calls this package.

## Step 1 — Run a command

`shell.Run(ctx, cmd)` runs `cmd` with the default shell for this OS and returns stdout with the trailing newline removed. [`shell.DefaultShell()`](ds:block?id=defaultshell-rz6bhgx2) reports which shell that is.

| OS | Shell `Run` uses |
|---|---|
| Linux, macOS, the BSDs, every other Unix-like | `sh -c <cmd>` |
| Windows, with `pwsh` (PowerShell 7+) on `PATH` | `pwsh -NoProfile -NonInteractive -Command <cmd>` |
| Windows, without `pwsh` | `powershell.exe -NoProfile -NonInteractive -Command <cmd>` |

<!-- ds:block id=shell-rundefault-k4m8q2xd -->
**runDefault** · [`examples/shell/run/main.go:47-55`](../../../examples/shell/run/main.go#L47-L55)

```go
func runDefault(w io.Writer) error {
	out, err := shell.Run(context.Background(), "echo hello from the shell")
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "run:     %q\n", out)
	fmt.Fprintf(w, "default: %q\n", shell.DefaultShell()) // [sh -c] here; PowerShell on Windows
	return nil
}
```
<!-- /ds:block hash=0a4cc9 -->

## Step 2 — Set the environment, directory and timeout with a `Runner`

[`shell.Runner`](ds:block?id=runner-vvzpt8w5) holds the options; its zero value is what `Run` uses.

| Field | Zero value | Set it to |
|---|---|---|
| `Shell []string` | the OS default above | another shell's argv prefix: `[]string{"bash", "-c"}`, `[]string{"cmd", "/C"}` |
| `Timeout time.Duration` | no limit beyond the context | a limit per command; running out is `ErrTimeout` |
| `Env []string` | the process environment | `KEY=value` pairs; they replace the whole environment, so add `PATH` if the command needs it |
| `Dir string` | the current directory | the directory the command runs in |

<!-- ds:block id=shell-runwithoptions-p7w3n6vc -->
**runWithOptions** · [`examples/shell/run/main.go:62-84`](../../../examples/shell/run/main.go#L62-L84)

```go
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
```
<!-- /ds:block hash=50deb6 -->

A `Runner` also has `RunCommand(cmd)`, which runs with `context.Background()` bounded by `Timeout`. That method makes it a `plugin.CommandRunner`, so you can pass a configured `Runner` wherever one is accepted, such as `value.WithRunner(shell.Runner{Timeout: 5 * time.Second})` or `dotenv.WithCommandRunner(r.RunCommand)`.

## Step 3 — Use another shell

`Runner.Shell` replaces the default. Use it when your commands are written for one shell: `bash -c` for bash syntax on Unix, and on Windows `cmd /C` for batch syntax, or a POSIX shell such as Git for Windows' `bash -c` for commands written for `sh`. The first element is looked up on `PATH`.

<!-- ds:block id=shell-runothershell-h2c9t5rb -->
**runOtherShell** · [`examples/shell/run/main.go:90-98`](../../../examples/shell/run/main.go#L90-L98)

```go
func runOtherShell(w io.Writer) error {
	bash := shell.Runner{Shell: []string{"bash", "-c"}}
	out, err := bash.Run(context.Background(), `[[ -n $BASH_VERSION ]] && echo "bash it is"`)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "bash:    %q\n", out)
	return nil
}
```
<!-- /ds:block hash=e1a992 -->

## Step 4 — Know what is trimmed

Only trailing CR and LF are removed, however many there are. Leading and trailing spaces, tabs and inner newlines stay, because a value may legitimately start or end with a space. A Windows tool that ends its output with `\r\n` gives the same value as a Unix one ending with `\n`.

<!-- ds:block id=shell-outputrule-z6v1j8sy -->
**outputRule** · [`examples/shell/run/main.go:104-117`](../../../examples/shell/run/main.go#L104-L117)

```go
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
```
<!-- /ds:block hash=cb50b9 -->

## Step 5 — Handle a failing command

A command that exits non-zero returns an `*shell.ExitError` with `Cmd`, `Code` (or `-1` when a signal ended it) and `Stderr`: the head of what the command printed on stderr, at most `shell.MaxStderr` (512) bytes, with `…` when cut. Tools explain themselves on stderr, so that is what a person needs to see. The error text never contains stdout, which may be the secret you were reading.

<!-- ds:block id=shell-exitcode-r5g2m9kw -->
**exitCode** · [`examples/shell/errors/main.go:51-59`](../../../examples/shell/errors/main.go#L51-L59)

```go
func exitCode(w io.Writer) error {
	_, err := shell.Run(context.Background(), "echo 'entry not found' >&2; exit 11")
	var ee *shell.ExitError
	if !errors.As(err, &ee) {
		return fmt.Errorf("want *shell.ExitError, got %v", err)
	}
	fmt.Fprintf(w, "exit:     code=%d stderr=%q\n          %v\n", ee.Code, ee.Stderr, err)
	return nil
}
```
<!-- /ds:block hash=bc110a -->

<!-- ds:block id=shell-nostdout-e2h8s5uj -->
**noStdoutInErrors** · [`examples/shell/errors/main.go:108-114`](../../../examples/shell/errors/main.go#L108-L114)

```go
func noStdoutInErrors(w io.Writer) error {
	r := shell.Runner{Env: []string{"FAKE_SECRET=" + fakeSecret}}
	_, err := r.Run(context.Background(), `printf '%s' "$FAKE_SECRET"; exit 1`)
	leaked := err != nil && strings.Contains(err.Error(), fakeSecret)
	fmt.Fprintf(w, "no leak:  %v (stdout in error: %t)\n", err, leaked)
	return nil
}
```
<!-- /ds:block hash=884e03 -->

## Step 6 — Bound it with a timeout or a context

`Runner.Timeout` and the context's deadline both kill the command and return an error matching `shell.ErrTimeout` (and `context.DeadlineExceeded`) with `errors.Is`. Cancelling the context returns an error matching `context.Canceled` instead, so you can tell "too slow" from "the caller gave up". The error names the command.

<!-- ds:block id=shell-timeouts-c8x4f1pn -->
**timeouts** · [`examples/shell/errors/main.go:65-79`](../../../examples/shell/errors/main.go#L65-L79)

```go
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
```
<!-- /ds:block hash=2d5036 -->

## Step 7 — Missing shell, closed stdin

When the shell program is not on `PATH` (the default for this OS, or `Runner.Shell[0]`), nothing runs and the error matches `shell.ErrNoShell`. Stdin is the null device: a command that asks a question reads end-of-file at once and fails or carries on, instead of waiting for an answer nobody can type.

<!-- ds:block id=shell-noshell-v3k7d2qa -->
**noShell** · [`examples/shell/errors/main.go:85-89`](../../../examples/shell/errors/main.go#L85-L89)

```go
func noShell(w io.Writer) error {
	_, err := shell.Runner{Shell: []string{"no-such-shell", "-c"}}.Run(context.Background(), "echo hi")
	fmt.Fprintf(w, "no shell: ErrNoShell=%t\n          %v\n", errors.Is(err, shell.ErrNoShell), err)
	return nil
}
```
<!-- /ds:block hash=24fe9f -->

<!-- ds:block id=shell-stdinclosed-n9b6w4ty -->
**stdinClosed** · [`examples/shell/errors/main.go:95-102`](../../../examples/shell/errors/main.go#L95-L102)

```go
func stdinClosed(w io.Writer) error {
	out, err := shell.Run(context.Background(), `if read -r answer; then echo "read: $answer"; else echo "stdin is empty"; fi`)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "stdin:    %s\n", out)
	return nil
}
```
<!-- /ds:block hash=07603b -->

## Try it

```sh
cd examples/shell/run && go run .
```

```text
run:     "hello from the shell"
default: ["sh" "-c"]
runner:  "hi from work"
bash:    "bash it is"
printf 'secret\r\n'          => "secret"
printf '  padded  \n\n'      => "  padded  "
printf 'line 1\nline 2\n'    => "line 1\nline 2"
```

```sh
cd examples/shell/errors && go run .
```

```text
exit:     code=11 stderr="entry not found"
          shell: command "echo 'entry not found' >&2; exit 11" exited with code 11: entry not found
timeout:  ErrTimeout=true
          shell: command "sleep 30" timed out
deadline: ErrTimeout=true DeadlineExceeded=true
cancel:   Canceled=true ErrTimeout=false
no shell: ErrNoShell=true
          shell: command "echo hi": no shell found: no-such-shell: exec: "no-such-shell": executable file not found in $PATH
stdin:    stdin is empty
no leak:  shell: command "printf '%s' \"$FAKE_SECRET\"; exit 1" exited with code 1 (stdout in error: false)
```

## Pitfalls

- **The command text is in the error.** Stdout never is, but the command line is, because that is what you need to find the failing setting. Do not put a secret in the command itself; let the command fetch it.
- **A command written for `sh` does not run in PowerShell.** `printf '%s' x`, `$(…)` inside the command, `&&` in Windows PowerShell 5 and `2>/dev/null` are `sh` syntax. On Windows, either write the command for PowerShell or set `Runner.Shell` to a POSIX shell (`[]string{"bash", "-c"}` with Git for Windows) or `cmd /C`.
- **`Env` replaces the whole environment.** `Env: []string{"A=1"}` gives the command only `A`, without `PATH`, `HOME` or `USERPROFILE`; many tools then fail to start or to find their config. Start from `os.Environ()` and append when you only want to add a variable.
- **A timeout kills the shell, not everything it started.** A process the command started in the background may keep running; `Run` still returns promptly, at most half a second after the shell ends. Commands that start daemons are not what this package is for.
- **Exit codes from PowerShell.** With `-Command`, PowerShell exits with the code given to `exit N`, or 1 when the last command failed. A native program's own code is passed on only when it is the last command or you `exit $LASTEXITCODE`.
- **Non-ASCII output on Windows.** PowerShell encodes redirected output with the console's code page; set `[Console]::OutputEncoding = [Text.Encoding]::UTF8;` at the start of the command when the output is not ASCII.
- **Windows is not tested on a real machine here.** The Windows shell choice is unit-tested by injecting the `PATH` lookup, and the package builds and vets for `GOOS=windows`; `shell_windows_test.go` holds the tests to run by hand on Windows.

## Related

- [value](value.md): expand a whole value, `${VAR}`, `${scheme:ref}` and `$(cmd)`, with this runner.
- [expand](expand.md): the engine; `Engine.Run` is where a format plugs a runner in.
- [dotenv expansion § Step 7](../dotenv/expansion.md#step-7--run-commands-opt-in) and [jsonc expansion § Step 4](../jsonc/expansion-and-plugins.md#step-4--turn-on-command-substitution-deliberately): `$(cmd)` in files, bounded with `WithCommandContext`.
- Examples: [`examples/shell/run`](../../../examples/shell/run), [`examples/shell/errors`](../../../examples/shell/errors).
- API: [shell on pkg.go.dev](https://pkg.go.dev/github.com/ubgo/textdata/shell).
