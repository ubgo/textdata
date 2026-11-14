# env

Expands single values with `value.Expand`, no file format involved: the process environment (`WithEnv`), a map layered over it (`WithLookup`), defaults, a nested default, `$$`, and `${VAR:?msg}` with the `*expand.RequiredError` it returns. It sets and clears its own `DEMO_*` variables, so the output is the same everywhere.

Guide: [value](../../../docs/guides/base/value.md).

## Run

```sh
cd examples/value/env && go run .
go test .   # runs the program and checks its output
```

## Expected output

```text
with env:    postgres://ada@db.internal/app
without env: postgres://@/app
${DEMO_PORT:-5432}                   => 5432
${DEMO_REGION:-${DEMO_HOST}}         => db.internal
${DEMO_USER:+user is set}            => user is set
cost: $$5                            => cost: $5
${DEMO_USER:?DEMO_USER must be set}  => ada
layered:     postgres://ada@localhost/app
required:    key=DEMO_API_TOKEN
             expand: required variable DEMO_API_TOKEN is not set or is empty: set DEMO_API_TOKEN first
```
