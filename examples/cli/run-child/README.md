# run-child

Drives `dotenvctl run -- COMMAND` through `dotenvcmd.Execute`: the child sees the file's values, expanded and winning over the inherited environment, while this process's own environment is unchanged; the child's exit code (3) comes back verbatim; and a file whose `${MISSING:?…}` cannot be satisfied stops the run with exit 1 before the child starts. POSIX only (the children are `/bin/sh`).

Guide: [dotenvctl command reference § run](../../../docs/guides/cli/dotenvctl.md#run--execute-a-command-with-the-files-values).

## Run it

```sh
cd examples/cli/run-child && go run .
```

Expected output:

```text
child: from the file, postgres://db.internal/app
exit: 0
parent still has: from the shell
exit: 3
dotenvctl: envkit: expand: dotenv: required variable MISSING is not set or is empty: set MISSING first
exit: 1
```

`main_test.go` runs the program and checks this output.
