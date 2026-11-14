# run

Uses package `shell` on its own, with no file format: `shell.Run` with the OS default shell, a `Runner` with `Env`, `Dir` and `Timeout`, `bash -c` in place of `sh -c`, and the output rule (only trailing CR and LF are trimmed). The commands are harmless (`echo`, `printf`, `basename`); it needs a Unix shell and bash.

Guide: [shell](../../../docs/guides/base/shell.md).

## Run

```sh
cd examples/shell/run && go run .
go test .   # runs the program and checks its output
```

## Expected output

```text
run:     "hello from the shell"
default: ["sh" "-c"]
runner:  "hi from work"
bash:    "bash it is"
printf 'secret\r\n'          => "secret"
printf '  padded  \n\n'      => "  padded  "
printf 'line 1\nline 2\n'    => "line 1\nline 2"
```
