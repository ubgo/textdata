# registry

Writes plugins against the base `plugin` package (a `Lookuper` + `Resolver` store, a `SaveGuard` that reads the format-neutral `Document`, and a plugin whose wrong method signature capability detection exposes), shows `Has`, `Registry`, `Filter`, `NameFor` and `plugin.Errorf`, then installs the same plugins in dotenv and jsonc. Uses a temp directory and an in-memory store.

Guide: [plugin](../../../docs/guides/base/plugin.md).

## Run

```sh
cd examples/base/registry && go run .
go test .   # runs the program and checks its output
```

## Expected output

```text
store:           [Lookuper Resolver]
no-private-keys: [SaveGuard]
upper:           []
info: store: Lookuper, Resolver
info: no-private-keys: SaveGuard
info: upper: (none)
save guards: 1, first resolver: store, second resolver: unknown
error: demo: plugin "store": lookup DOWN: dial store: plugin unavailable
unavailable=true plugin=true
dotenv: DSN=postgres://app@db/app?password=hush TOKEN=t-123
jsonc:  db.password=hush token=t-123
dotenv save: dotenv: plugin "no-private-keys": guard save $DIR/.env: dotenv .env: TLS_KEY holds a private key: access denied (denied=true)
.env on disk: "TLS_KEY=\n"
jsonc save:  jsonc: plugin "no-private-keys": guard save $DIR/config.jsonc: jsonc config.jsonc: tls.key holds a private key: access denied (denied=true)
plugins seen by jsonc: [no-private-keys: SaveGuard]
```
