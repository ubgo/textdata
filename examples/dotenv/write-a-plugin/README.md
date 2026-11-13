# dotenv/write-a-plugin

A plugin type with a Lookuper and a SaveGuard sharing one store, compile-time assertions, a cross-format `plugin.SaveGuard`, Plugins() diagnostics, a refused save that leaves the file untouched, and a store outage that fails the read.

Guide: [docs/guides/dotenv/plugins.md#step-3--write-a-plugin-type](../../../docs/guides/dotenv/plugins.md#step-3--write-a-plugin-type).

## Run it

```sh
cd examples/dotenv/write-a-plugin && go run .
```

Nothing touches the network, the process environment or a real secret store; programs that write files use a fresh temp directory and remove it. `go test ./dotenv/write-a-plugin/` (from `examples/`) runs the program and compares its output with the text below.

## Expected output

```text
== install and read
plugin vault: Lookuper, SaveGuard
plugin require-keys: SaveGuard
GetExpanded: postgres://app@db/app?password=hush
Get:         postgres://${DB_USER}@db/app?password=${DB_PASSWORD}
== guard
leak refused: true
dotenv: plugin "vault": guard save .env: plain-text secret in file: DB_PASSWORD holds DB_PASSWORD
dotenv: plugin "require-keys": guard save .env: dotenv: DB_URL is required
file unchanged: true files in dir: 1
== store down
dotenv: plugin "vault": lookup DB_PASSWORD: plugin unavailable: connection refused
unavailable: true
```
