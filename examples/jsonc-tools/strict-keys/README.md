# strict-keys

`schema.FromStruct` on a config with maps of structs, a `MarshalText` ID type as a map key, an `enum:"plan,api"` field, pointers and `omitempty`; `schema.Validate` reports the bad enum value as an error and the typo'd key `biling` as a warning, and as an error with `schema.StrictKeys()`, each at its `line:col`.

Guide: [schema](../../../docs/guides/jsonc-tools/schema.md).

## Run

```sh
cd examples/jsonc-tools/strict-keys && go run .
go test .   # runs the program and checks its output
```

## Expected output

```text
default: ok=false
  error   agents.jsonc:4:25: profiles.work.billing: must be one of "plan", "api"; got "free"
  warning agents.jsonc:5:14: profiles.home.biling: unknown key "biling"
strict keys: ok=false
  error   agents.jsonc:4:25: profiles.work.billing: must be one of "plan", "api"; got "free"
  error   agents.jsonc:5:14: profiles.home.biling: unknown key "biling"
```
