# exec-plugin

Installs a third-party dotenvctl plugin, a shell script named `dotenvctl-hello`, in a temp directory that is the whole PATH; lists it with `dotenvctl plugins --json` next to a `dotenvctl-github` that a built-in shadows; and runs it as `dotenvctl hello`. The script prints the `DOTENVCTL_*` context it receives and calls the host back through `DOTENVCTL_BIN` for a value. The program is the host too: it embeds `dotenvcmd`, so the callback runs this binary again, which serves it (`serveAsHost`). POSIX only (the plugin is `/bin/sh`).

Guide: [Exec plugins](../../../docs/guides/cli/exec-plugins.md).

## Run it

```sh
cd examples/cli/exec-plugin && go run .
```

Expected output:

```text
plugin github: shadowed by built-in (dotenvctl-github)
plugin hello: active (dotenvctl-hello)
hello, args: [world]
file: .env
json: 0
placeholder: ^__[A-Z0-9_]+__$
version set: yes
callback: hi from the file
exit: 0
hello, args: []
file: .env
json: 1
placeholder: ^__[A-Z0-9_]+__$
version set: yes
callback: hi from the file
exit: 0
```

`main_test.go` runs the program and checks this output; its `TestMain` serves the callback, because under `go test` the test binary is the host.
