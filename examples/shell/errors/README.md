# errors

Every way package `shell` reports a failure, with no file format: a non-zero exit (`*shell.ExitError` with the code and the head of stderr), `Runner.Timeout` and a context deadline (`shell.ErrTimeout`), a cancelled context (`context.Canceled`), a missing shell (`shell.ErrNoShell`), stdin closed so a prompt cannot hang, and stdout kept out of every error. The "secret" is a fake string; it needs a Unix shell.

Guide: [shell](../../../docs/guides/base/shell.md).

## Run

```sh
cd examples/shell/errors && go run .
go test .   # runs the program and checks its output
```

## Expected output

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
