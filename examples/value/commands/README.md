# commands

`$(command)` in `value.Expand`: kept as written unless asked for, run through the OS shell with `WithCommands`, bounded by the context or by a `shell.Runner` timeout, refused or answered by an allow-listed runner through `WithRunner`, and the `*expand.CommandError` a failure returns. The commands are harmless (`printf`, `echo`, `sleep`); it needs a Unix shell.

Guide: [value](../../../docs/guides/base/value.md).

## Run

```sh
cd examples/value/commands && go run .
go test .   # runs the program and checks its output
```

## Expected output

```text
off:      token=$(echo never-runs)
on:       hello ada!
failed:   code=2
          expand: command "echo 'no such entry' >&2; exit 2" failed: shell: command "echo 'no such entry' >&2; exit 2" exited with code 2: no such entry
deadline: ErrTimeout=true
runner:   ErrTimeout=true
allowed:  fake-api-token
refused:  expand: command "rm -rf ~" failed: command not allowed
```
