# jsonc: expansion and value plugins

Expands `${VAR}`, `${VAR:-default}`, `${VAR:?message}`, `${scheme:ref}`, `$(command)` and `$$` in a JSONC config while the document keeps the references. Every source is a fake the program owns: a map lookup, a `Resolver` for `${vault:...}`, a `ValueTransformer` and a `CommandRunner` (`plugins.go`). It also turns on `WithCommandSubstitution(true)` for a harmless `echo` through `sh -c`, and uses `WithExpand` with the one-call `jsonc.Read`.

## Run it

```sh
cd examples/jsonc/expand && go run .
```

Expected output (`main_test.go` checks it on every test run):

```text
expanded: postgres://db.internal:5432/app
raw:      postgres://${HOST}:${PORT:-5432}/app
build (commands off): $(git rev-parse --short HEAD) | price: $5
required: app.jsonc:2:12: expand "token": expand: required variable API_TOKEN is not set or is empty: set API_TOKEN
missing key: API_TOKEN
password: "hush" | build: a1b2c3d
plugin: lookup: Lookuper
plugin: fake-vault: Resolver
plugin: trim-space: ValueTransformer
plugin: fake-git: CommandRunner
sh -c: hello
Read: postgres://db.internal:5432/app | ExpandRequested: true
```

## Learn more

- Guide: [expansion and plugins](../../../docs/guides/jsonc/expansion-and-plugins.md)
- Module README: [jsonc § Expansion and plugins](../../../jsonc/README.md#expansion-and-plugins)
